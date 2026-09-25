package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TitleCard é o que a interface precisa para desenhar um pôster na grade.
type TitleCard struct {
	ID        int64     `json:"id"`
	LibraryID int64     `json:"library_id"`
	Kind      TitleKind `json:"kind"`
	Name      string    `json:"name"`
	Year      int       `json:"year,omitempty"`
	Artist    string    `json:"artist,omitempty"`
	Rating    float64   `json:"rating,omitempty"`
	Poster    string    `json:"poster,omitempty"`
	Backdrop  string    `json:"backdrop,omitempty"`
	Genres    string    `json:"genres,omitempty"`
	Files     int       `json:"files"`
	Duration  float64   `json:"duration,omitempty"`
	MetaState string    `json:"meta_state"`
	// SoNaNuvem: nenhum arquivo do título tem cópia no Mac. No modo local o
	// card aparece como "na nuvem, indisponível".
	SoNaNuvem bool `json:"so_na_nuvem,omitempty"`
	// SoNoMac: nenhum arquivo tem cópia no bucket. Na instância cloud o card
	// aparece como "no Mac, indisponível".
	SoNoMac bool `json:"so_no_mac,omitempty"`
}

// TitleFilter monta a listagem da biblioteca.
type TitleFilter struct {
	LibraryID int64
	Kind      TitleKind
	Query     string
	Sort      string // name | year | recent
	Limit     int
	Offset    int
}

// A contagem e a soma vêm de subconsultas, não de LEFT JOIN + GROUP BY.
// Parece rodeio, mas muda a ordem de grandeza: com JOIN, o SQLite lê todos os
// arquivos do acervo para devolver uma página de 60 títulos; com subconsulta,
// ele resolve os 60 primeiro e só então conta os arquivos de cada um, pelo
// índice. Medido em 20 mil títulos: 13 ms → 0,08 ms.
const titleCardSelect = `
	SELECT t.id, t.library_id, t.kind, t.name, IFNULL(t.year, 0), t.artist,
	       IFNULL(t.rating, 0), t.poster, t.backdrop, t.genres, t.meta_state,
	       (SELECT COUNT(*) FROM media_files f WHERE f.title_id = t.id),
	       (SELECT IFNULL(SUM(f.duration), 0) FROM media_files f WHERE f.title_id = t.id),
	       NOT EXISTS (SELECT 1 FROM media_files f WHERE f.title_id = t.id
	                      AND f.localizacao NOT IN ('nuvem', 'baixando')),
	       NOT EXISTS (SELECT 1 FROM media_files f WHERE f.title_id = t.id
	                      AND f.localizacao NOT IN ('local', 'enviando'))
	  FROM titles t`

// ListTitles devolve os títulos que casam com o filtro.
func (d *DB) ListTitles(ctx context.Context, f TitleFilter) ([]TitleCard, error) {
	var (
		where []string
		args  []any
	)
	if f.LibraryID > 0 {
		where = append(where, "t.library_id = ?")
		args = append(args, f.LibraryID)
	}
	if f.Kind != "" {
		where = append(where, "t.kind = ?")
		args = append(args, string(f.Kind))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "(t.name LIKE ? OR t.artist LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}

	query := titleCardSelect
	if len(where) > 0 {
		query += "\n WHERE " + strings.Join(where, " AND ")
	}

	switch f.Sort {
	case "year":
		query += "\n ORDER BY IFNULL(t.year, 0) DESC, t.sort_name"
	case "recent":
		// created_at e id juntos: dois títulos criados no mesmo segundo
		// (acontece o tempo todo no primeiro scan) precisam de desempate
		// estável, senão a paginação repete ou pula itens.
		query += "\n ORDER BY t.created_at DESC, t.id DESC"
	default:
		query += "\n ORDER BY t.sort_name"
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query += "\n LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listando títulos: %w", err)
	}
	defer rows.Close()

	cards := []TitleCard{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// CountTitles conta o total do filtro, para a paginação da grade.
func (d *DB) CountTitles(ctx context.Context, f TitleFilter) (int, error) {
	var (
		where []string
		args  []any
	)
	if f.LibraryID > 0 {
		where = append(where, "library_id = ?")
		args = append(args, f.LibraryID)
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, string(f.Kind))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "(name LIKE ? OR artist LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	query := "SELECT COUNT(*) FROM titles"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	var n int
	err := d.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

// RecentTitles lista o que entrou por último no acervo.
func (d *DB) RecentTitles(ctx context.Context, limit int) ([]TitleCard, error) {
	rows, err := d.QueryContext(ctx, titleCardSelect+`
	 ORDER BY t.created_at DESC, t.id DESC
	 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listando recentes: %w", err)
	}
	defer rows.Close()

	cards := []TitleCard{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// coletarCards materializa um resultado de titleCardSelect. Existe porque cada
// prateleira nova repetiria o mesmo laço de scan linha a linha.
func coletarCards(rows *sql.Rows) ([]TitleCard, error) {
	cards := []TitleCard{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func scanCard(rows *sql.Rows) (TitleCard, error) {
	var c TitleCard
	var kind string
	err := rows.Scan(&c.ID, &c.LibraryID, &kind, &c.Name, &c.Year, &c.Artist,
		&c.Rating, &c.Poster, &c.Backdrop, &c.Genres, &c.MetaState, &c.Files, &c.Duration, &c.SoNaNuvem, &c.SoNoMac)
	c.Kind = TitleKind(kind)
	return c, err
}

// ContinueItem é uma linha de "continuar assistindo".
type ContinueItem struct {
	FileID    int64     `json:"file_id"`
	TitleID   int64     `json:"title_id"`
	TitleName string    `json:"title_name"`
	Kind      TitleKind `json:"kind"`
	Label     string    `json:"label,omitempty"` // "T1:E2"
	Poster    string    `json:"poster,omitempty"`
	Backdrop  string    `json:"backdrop,omitempty"`
	Position  float64   `json:"position"`
	Duration  float64   `json:"duration"`
}

// ContinueWatching traz o que foi deixado pela metade nos últimos 30 dias, do
// mais recente para o mais antigo. Ignora o que mal começou (menos de 30s) e o
// que já acabou.
//
// O corte de 30 dias existe para a lista não virar cemitério: o que foi
// abandonado há meses aparece em "Esquecidos", com esse nome, em vez de fingir
// que é a sessão de ontem. A união das duas prateleiras é tudo que foi começado
// e não terminado — nada deixou de ser exibido.
func (d *DB) ContinueWatching(ctx context.Context, userID int64, limit int) ([]ContinueItem, error) {
	corte := time.Now().AddDate(0, 0, -diasParaEsquecer).Unix()
	return d.continuarQuery(ctx, `p.updated_at >= ?`, `p.updated_at DESC`, userID, corte, limit)
}

// continuarQuery é o corpo compartilhado por "continuar assistindo" e
// "esquecidos": mesma junção, mesmos critérios, só muda a janela de tempo.
func (d *DB) continuarQuery(ctx context.Context, filtro, ordem string, userID, corte int64, limit int) ([]ContinueItem, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT p.media_file_id, t.id, t.name, t.kind, t.poster, t.backdrop,
		       p.position_sec, MAX(p.duration_sec, IFNULL(f.duration, 0)),
		       IFNULL(e.season, 0), IFNULL(e.episode, 0)
		  FROM progress p
		  JOIN media_files f ON f.id = p.media_file_id
		  JOIN titles t      ON t.id = f.title_id
		  LEFT JOIN episodes e ON e.media_file_id = f.id
		 WHERE p.user_id = ? AND p.finished = 0 AND p.position_sec > 30
		   AND `+filtro+`
		 ORDER BY `+ordem+`
		 LIMIT ?`, userID, corte, limit)
	if err != nil {
		return nil, fmt.Errorf("continuar assistindo: %w", err)
	}
	defer rows.Close()

	items := []ContinueItem{}
	for rows.Next() {
		var (
			it              ContinueItem
			kind            string
			season, episode int
		)
		if err := rows.Scan(&it.FileID, &it.TitleID, &it.TitleName, &kind, &it.Poster, &it.Backdrop,
			&it.Position, &it.Duration, &season, &episode); err != nil {
			return nil, err
		}
		it.Kind = TitleKind(kind)
		if season > 0 || episode > 0 {
			it.Label = fmt.Sprintf("T%d:E%d", season, episode)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// FileInfo é um arquivo como a interface o vê (sem caminho absoluto).
type FileInfo struct {
	ID       int64     `json:"id"`
	RelPath  string    `json:"rel_path"`
	Name     string    `json:"name"`
	Ext      string    `json:"ext"`
	Type     MediaType `json:"media_type"`
	Size     int64     `json:"size"`
	Duration float64   `json:"duration"`
	Width    int       `json:"width,omitempty"`
	Height   int       `json:"height,omitempty"`
	VCodec   string    `json:"vcodec,omitempty"`
	ACodec   string    `json:"acodec,omitempty"`
	Track    int       `json:"track,omitempty"`
	Thumb    string    `json:"thumb,omitempty"`
	Position float64   `json:"position,omitempty"`
	Finished bool      `json:"finished,omitempty"`
	Season   int       `json:"season,omitempty"`
	Episode  int       `json:"episode,omitempty"`
	EpName   string    `json:"episode_name,omitempty"`
	// Quando é a data de captura da foto, com o mtime do arquivo como reserva.
	// A galeria agrupa por ela.
	Quando int64 `json:"quando,omitempty"`
	// TagName é o título vindo das tags do arquivo (música).
	TagName string `json:"-"`
	// Localizacao: local, enviando, ambos, baixando ou nuvem.
	Localizacao string `json:"localizacao"`
}

// TitleFiles devolve os arquivos de um título já com o progresso do usuário e
// os dados de episódio, ordenados como devem aparecer na tela.
func (d *DB) TitleFiles(ctx context.Context, titleID, userID int64) ([]FileInfo, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT f.id, f.rel_path, f.ext, f.media_type, f.size, IFNULL(f.duration, 0),
		       IFNULL(f.width, 0), IFNULL(f.height, 0), f.vcodec, f.acodec,
		       IFNULL(f.track, 0), f.thumb, f.display_name,
		       IFNULL(p.position_sec, 0), IFNULL(p.finished, 0),
		       IFNULL(e.season, 0), IFNULL(e.episode, 0), IFNULL(e.name, ''),
		       IFNULL(NULLIF(f.taken_at, 0), f.mtime), f.localizacao
		  FROM media_files f
		  LEFT JOIN progress p ON p.media_file_id = f.id AND p.user_id = ?
		  LEFT JOIN episodes e ON e.media_file_id = f.id
		 WHERE f.title_id = ?
		 -- Série e álbum ordenam por episódio e faixa. Foto não tem nem um nem
		 -- outro (ambos zero), então cai no critério seguinte: a data, do mais
		 -- recente para o mais antigo, que é como se olha um álbum de fotos.
		 ORDER BY IFNULL(e.season, 0), IFNULL(e.episode, 0), IFNULL(f.track, 0),
		          CASE WHEN f.media_type = 'photo'
		               THEN -IFNULL(NULLIF(f.taken_at, 0), f.mtime) ELSE 0 END,
		          f.rel_path`,
		userID, titleID)
	if err != nil {
		return nil, fmt.Errorf("arquivos do título: %w", err)
	}
	defer rows.Close()

	files := []FileInfo{}
	for rows.Next() {
		var (
			fi       FileInfo
			mtype    string
			finished int
		)
		if err := rows.Scan(&fi.ID, &fi.RelPath, &fi.Ext, &mtype, &fi.Size, &fi.Duration,
			&fi.Width, &fi.Height, &fi.VCodec, &fi.ACodec, &fi.Track, &fi.Thumb, &fi.TagName,
			&fi.Position, &finished, &fi.Season, &fi.Episode, &fi.EpName, &fi.Quando, &fi.Localizacao); err != nil {
			return nil, err
		}
		fi.Type = MediaType(mtype)
		fi.Finished = finished != 0
		fi.Name = displayName(fi)
		files = append(files, fi)
	}
	return files, rows.Err()
}

func displayName(fi FileInfo) string {
	if fi.EpName != "" {
		return fi.EpName
	}
	if fi.TagName != "" {
		return fi.TagName
	}
	if fi.Season > 0 || fi.Episode > 0 {
		return fmt.Sprintf("Episódio %d", fi.Episode)
	}
	name := fi.RelPath
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, fi.Ext)
}

// TitleByID devolve o título completo.
func (d *DB) TitleByID(ctx context.Context, id int64) (Title, error) {
	var (
		t      Title
		kind   string
		year   sql.NullInt64
		rating sql.NullFloat64
		tmdbID sql.NullInt64
	)
	err := d.QueryRowContext(ctx, `
		SELECT id, library_id, kind, name, sort_name, year, overview, rating,
		       genres, artist, tmdb_id, poster, backdrop, meta_state
		  FROM titles WHERE id = ?`, id).
		Scan(&t.ID, &t.LibraryID, &kind, &t.Name, &t.SortName, &year, &t.Overview, &rating,
			&t.Genres, &t.Artist, &tmdbID, &t.Poster, &t.Backdrop, &t.MetaState)
	if errors.Is(err, sql.ErrNoRows) {
		return Title{}, ErrNotFound
	}
	if err != nil {
		return Title{}, err
	}
	t.Kind = TitleKind(kind)
	t.Year = int(year.Int64)
	t.Rating = rating.Float64
	t.TMDBID = int(tmdbID.Int64)
	return t, nil
}

// FileByID devolve o arquivo com o caminho absoluto, para o streaming.
func (d *DB) FileByID(ctx context.Context, id int64) (MediaFile, error) {
	var (
		f       MediaFile
		mtype   string
		titleID sql.NullInt64
	)
	err := d.QueryRowContext(ctx, `
		SELECT id, library_id, title_id, path, rel_path, ext, size, mtime, media_type,
		       IFNULL(duration, 0), IFNULL(width, 0), IFNULL(height, 0), vcodec, acodec,
		       IFNULL(track, 0), thumb, pix_fmt, vprofile, channels, vbitrate,
		       localizacao, nuvem_key
		  FROM media_files WHERE id = ?`, id).
		Scan(&f.ID, &f.LibraryID, &titleID, &f.Path, &f.RelPath, &f.Ext, &f.Size, &f.MTime,
			&mtype, &f.Duration, &f.Width, &f.Height, &f.VCodec, &f.ACodec, &f.Track, &f.Thumb,
			&f.PixFmt, &f.VProfile, &f.Channels, &f.VBitrate, &f.Localizacao, &f.NuvemKey)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaFile{}, ErrNotFound
	}
	if err != nil {
		return MediaFile{}, err
	}
	f.Type = MediaType(mtype)
	if titleID.Valid {
		id := titleID.Int64
		f.TitleID = &id
	}
	return f, nil
}

// PlaybackInfo é tudo que o player precisa saber sobre um arquivo.
type PlaybackInfo struct {
	FileInfo
	TitleID   int64     `json:"title_id"`
	TitleName string    `json:"title_name"`
	Kind      TitleKind `json:"kind"`
	Poster    string    `json:"poster,omitempty"`
}

// PlaybackByFile devolve o arquivo com o título e o progresso do usuário.
func (d *DB) PlaybackByFile(ctx context.Context, fileID, userID int64) (PlaybackInfo, error) {
	var (
		info     PlaybackInfo
		mtype    string
		kind     sql.NullString
		name     sql.NullString
		poster   sql.NullString
		titleID  sql.NullInt64
		finished int
	)
	err := d.QueryRowContext(ctx, `
		SELECT f.id, f.rel_path, f.ext, f.media_type, f.size, IFNULL(f.duration, 0),
		       IFNULL(f.width, 0), IFNULL(f.height, 0), f.vcodec, f.acodec, f.thumb,
		       f.display_name,
		       IFNULL(p.position_sec, 0), IFNULL(p.finished, 0),
		       IFNULL(e.season, 0), IFNULL(e.episode, 0), IFNULL(e.name, ''),
		       f.title_id, t.name, t.kind, t.poster
		  FROM media_files f
		  LEFT JOIN progress p ON p.media_file_id = f.id AND p.user_id = ?
		  LEFT JOIN episodes e ON e.media_file_id = f.id
		  LEFT JOIN titles   t ON t.id = f.title_id
		 WHERE f.id = ?`, userID, fileID).
		Scan(&info.ID, &info.RelPath, &info.Ext, &mtype, &info.Size, &info.Duration,
			&info.Width, &info.Height, &info.VCodec, &info.ACodec, &info.Thumb,
			&info.TagName,
			&info.Position, &finished, &info.Season, &info.Episode, &info.EpName,
			&titleID, &name, &kind, &poster)
	if errors.Is(err, sql.ErrNoRows) {
		return PlaybackInfo{}, ErrNotFound
	}
	if err != nil {
		return PlaybackInfo{}, err
	}

	info.Type = MediaType(mtype)
	info.Finished = finished != 0
	info.Name = displayName(info.FileInfo)
	info.TitleID = titleID.Int64
	info.TitleName = name.String
	info.Kind = TitleKind(kind.String)
	info.Poster = poster.String
	return info, nil
}

// NextEpisode acha o episódio seguinte ao arquivo dado, dentro da mesma série.
func (d *DB) NextEpisode(ctx context.Context, fileID int64) (int64, error) {
	var next int64
	err := d.QueryRowContext(ctx, `
		WITH atual AS (
			SELECT e.title_id, e.season, e.episode
			  FROM episodes e WHERE e.media_file_id = ?
		)
		SELECT e.media_file_id
		  FROM episodes e, atual a
		 WHERE e.title_id = a.title_id
		   AND e.media_file_id IS NOT NULL
		   AND (e.season > a.season OR (e.season = a.season AND e.episode > a.episode))
		 ORDER BY e.season, e.episode
		 LIMIT 1`, fileID).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return next, err
}

// SaveProgress grava a posição de reprodução. finished é decidido aqui: com
// 95% assistido o item sai de "continuar assistindo".
func (d *DB) SaveProgress(ctx context.Context, userID, fileID int64, position, duration float64) error {
	finished := 0
	if duration > 0 && position/duration >= 0.95 {
		finished = 1
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO progress (user_id, media_file_id, position_sec, duration_sec, finished, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, media_file_id) DO UPDATE SET
			position_sec = excluded.position_sec,
			duration_sec = excluded.duration_sec,
			finished     = excluded.finished,
			updated_at   = excluded.updated_at`,
		userID, fileID, position, duration, finished, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("salvando progresso: %w", err)
	}
	if ref, err := d.RefDoArquivo(ctx, fileID); err == nil {
		d.RegistraEvento(ctx, "progresso.atualizado", map[string]any{
			"user_id": userID, "ref": ref, "posicao": position, "duracao": duration,
			"updated_at": time.Now().Unix(),
		})
	}
	return nil
}

func (d *DB) SetFavorite(ctx context.Context, userID, titleID int64, on bool) error {
	if ref, err := d.RefDoTitulo(ctx, titleID); err == nil {
		d.RegistraEvento(ctx, "favorito.alterado", map[string]any{
			"user_id": userID, "ref": ref, "favorito": on,
		})
	}
	if !on {
		_, err := d.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND title_id = ?`, userID, titleID)
		return err
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO favorites (user_id, title_id, created_at) VALUES (?, ?, ?)
		ON CONFLICT DO NOTHING`, userID, titleID, time.Now().Unix())
	return err
}

func (d *DB) IsFavorite(ctx context.Context, userID, titleID int64) (bool, error) {
	var n int
	err := d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM favorites WHERE user_id = ? AND title_id = ?`, userID, titleID).Scan(&n)
	return n > 0, err
}

// FavoriteTitles lista os favoritos do usuário como cartões.
func (d *DB) FavoriteTitles(ctx context.Context, userID int64, limit int) ([]TitleCard, error) {
	rows, err := d.QueryContext(ctx, titleCardSelect+`
	  JOIN favorites fav ON fav.title_id = t.id AND fav.user_id = ?
	 ORDER BY fav.created_at DESC
	 LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("listando favoritos: %w", err)
	}
	defer rows.Close()

	cards := []TitleCard{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}
