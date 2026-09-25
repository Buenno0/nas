package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Réplica: o que o Mac publica para a instância cloud e como ela aplica.
//
// Donos: o Mac é dono de bibliotecas, usuários, títulos e itens do disco. A
// nuvem só acrescenta o que recebeu por upload (e o Mac incorpora pelo
// evento). IDs não viajam como identidade: cada nó tem os seus; um arquivo é
// reconhecido pela chave no bucket ou pelo caminho no Mac (Ref).

// Ref identifica um arquivo entre nós.
type Ref struct {
	Key string `json:"k,omitempty"` // chave no bucket, quando existe
	Mac string `json:"m,omitempty"` // caminho no Mac (ou "nuvem:<key>")
}

// Prefixo dos caminhos de itens que, vistos da nuvem, só existem no Mac.
const PrefixoNoMac = "mac:"

type chaveSemEventos struct{}

// SemEventos marca um contexto de aplicação de réplica: o que muda aqui veio
// de outro nó e não pode voltar para o outbox, senão os dois ficam ecoando.
func SemEventos(ctx context.Context) context.Context {
	return context.WithValue(ctx, chaveSemEventos{}, true)
}

func semEventos(ctx context.Context) bool {
	v, _ := ctx.Value(chaveSemEventos{}).(bool)
	return v
}

// RefDoArquivo monta a Ref de um arquivo deste nó.
func (d *DB) RefDoArquivo(ctx context.Context, fileID int64) (Ref, error) {
	var path, key string
	err := d.QueryRowContext(ctx, `SELECT path, nuvem_key FROM media_files WHERE id = ?`, fileID).Scan(&path, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return Ref{}, ErrNotFound
	}
	return Ref{Key: key, Mac: strings.TrimPrefix(path, PrefixoNoMac)}, err
}

// ArquivoPorRef acha o arquivo local de uma Ref vinda de outro nó.
func (d *DB) ArquivoPorRef(ctx context.Context, r Ref) (int64, error) {
	var id int64
	var err error
	if r.Key != "" {
		err = d.QueryRowContext(ctx, `SELECT id FROM media_files WHERE nuvem_key = ?`, r.Key).Scan(&id)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return id, err
		}
	}
	if r.Mac != "" {
		err = d.QueryRowContext(ctx, `SELECT id FROM media_files WHERE path IN (?, ?)`,
			r.Mac, PrefixoNoMac+r.Mac).Scan(&id)
	}
	if err == nil && id == 0 {
		err = sql.ErrNoRows
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// RefDoTitulo é a Ref do primeiro arquivo do título: títulos também têm IDs
// diferentes em cada nó.
func (d *DB) RefDoTitulo(ctx context.Context, titleID int64) (Ref, error) {
	var fileID int64
	err := d.QueryRowContext(ctx,
		`SELECT id FROM media_files WHERE title_id = ? ORDER BY id LIMIT 1`, titleID).Scan(&fileID)
	if errors.Is(err, sql.ErrNoRows) {
		return Ref{}, ErrNotFound
	}
	if err != nil {
		return Ref{}, err
	}
	return d.RefDoArquivo(ctx, fileID)
}

// TituloPorRef resolve a Ref de título.
func (d *DB) TituloPorRef(ctx context.Context, r Ref) (int64, error) {
	fileID, err := d.ArquivoPorRef(ctx, r)
	if err != nil {
		return 0, err
	}
	var titleID sql.NullInt64
	if err := d.QueryRowContext(ctx, `SELECT title_id FROM media_files WHERE id = ?`, fileID).Scan(&titleID); err != nil {
		return 0, err
	}
	if !titleID.Valid {
		return 0, ErrNotFound
	}
	return titleID.Int64, nil
}

// JaProcessado registra o evento e diz se ele já tinha sido aplicado.
func (d *DB) JaProcessado(ctx context.Context, eventID string) (bool, error) {
	res, err := d.ExecContext(ctx,
		`INSERT INTO eventos_processados (event_id, em) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		eventID, time.Now().Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 0, nil
}

// ProgressoLWW grava o progresso só se ele for mais novo que o daqui: last
// writer wins por (usuário, arquivo), com o relógio de quem escreveu.
func (d *DB) ProgressoLWW(ctx context.Context, userID, fileID int64, posicao, duracao float64, atualizado int64) error {
	finished := 0
	if duracao > 0 && posicao/duracao >= 0.95 {
		finished = 1
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO progress (user_id, media_file_id, position_sec, duration_sec, finished, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, media_file_id) DO UPDATE SET
			position_sec = excluded.position_sec,
			duration_sec = excluded.duration_sec,
			finished     = excluded.finished,
			updated_at   = excluded.updated_at
		WHERE excluded.updated_at > progress.updated_at`,
		userID, fileID, posicao, duracao, finished, atualizado)
	return err
}

// --- Snapshot ----------------------------------------------------------------

type SnapBiblioteca struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}

type SnapUsuario struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Hash       string `json:"hash"` // argon2id; nunca a senha
	MustChange bool   `json:"must_change"`
	IsAdmin    bool   `json:"is_admin"`
	CreatedAt  int64  `json:"created_at"`
}

type SnapTitulo struct {
	ID        int64   `json:"id"` // ID no Mac, só para os itens apontarem
	LibraryID int64   `json:"library_id"`
	Kind      string  `json:"kind"`
	Name      string  `json:"name"`
	SortName  string  `json:"sort_name"`
	Year      int     `json:"year,omitempty"`
	Overview  string  `json:"overview,omitempty"`
	Rating    float64 `json:"rating,omitempty"`
	Genres    string  `json:"genres,omitempty"`
	Artist    string  `json:"artist,omitempty"`
	TMDBID    int     `json:"tmdb_id,omitempty"`
	Poster    string  `json:"poster,omitempty"`
	Backdrop  string  `json:"backdrop,omitempty"`
	MetaState string  `json:"meta_state"`
}

type SnapItem struct {
	Ref         Ref      `json:"ref"`
	LibraryID   int64    `json:"library_id"`
	TitleID     int64    `json:"title_id,omitempty"`
	RelPath     string   `json:"rel_path"`
	Ext         string   `json:"ext"`
	Size        int64    `json:"size"`
	MTime       int64    `json:"mtime"`
	Type        string   `json:"type"`
	Duration    float64  `json:"duration,omitempty"`
	Width       int      `json:"width,omitempty"`
	Height      int      `json:"height,omitempty"`
	VCodec      string   `json:"vcodec,omitempty"`
	ACodec      string   `json:"acodec,omitempty"`
	Track       int      `json:"track,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	PixFmt      string   `json:"pix_fmt,omitempty"`
	VProfile    string   `json:"vprofile,omitempty"`
	Channels    int      `json:"channels,omitempty"`
	VBitrate    int      `json:"vbitrate,omitempty"`
	TakenAt     int64    `json:"taken_at,omitempty"`
	Localizacao string   `json:"localizacao"`
	Season      int      `json:"season,omitempty"`
	Episode     int      `json:"episode,omitempty"`
	EpName      string   `json:"ep_name,omitempty"`
	Streams     []Stream `json:"streams,omitempty"`
}

type SnapProgresso struct {
	UserID    int64   `json:"user_id"`
	Ref       Ref     `json:"ref"`
	Posicao   float64 `json:"posicao"`
	Duracao   float64 `json:"duracao"`
	UpdatedAt int64   `json:"updated_at"`
}

type SnapFavorito struct {
	UserID    int64 `json:"user_id"`
	TitleID   int64 `json:"title_id"`
	CreatedAt int64 `json:"created_at"`
}

type Snapshot struct {
	SchemaVersion int              `json:"schema_version"`
	GeradoEm      time.Time        `json:"gerado_em"`
	Bibliotecas   []SnapBiblioteca `json:"bibliotecas"`
	Usuarios      []SnapUsuario    `json:"usuarios"`
	Titulos       []SnapTitulo     `json:"titulos"`
	Itens         []SnapItem       `json:"itens"`
	Progresso     []SnapProgresso  `json:"progresso"`
	Favoritos     []SnapFavorito   `json:"favoritos"`
}

// ExportarSnapshot lê o estado inteiro que o Mac replica.
func (d *DB) ExportarSnapshot(ctx context.Context) (Snapshot, error) {
	s := Snapshot{SchemaVersion: VersaoDosEventos, GeradoEm: time.Now().UTC()}

	err := d.linhas(ctx, `SELECT id, name, kind, enabled FROM libraries`, func(r *sql.Rows) error {
		var b SnapBiblioteca
		if err := r.Scan(&b.ID, &b.Name, &b.Kind, &b.Enabled); err != nil {
			return err
		}
		s.Bibliotecas = append(s.Bibliotecas, b)
		return nil
	})
	if err != nil {
		return s, err
	}
	err = d.linhas(ctx, `SELECT id, username, password_hash, must_change_password, is_admin, created_at FROM users`,
		func(r *sql.Rows) error {
			var u SnapUsuario
			if err := r.Scan(&u.ID, &u.Username, &u.Hash, &u.MustChange, &u.IsAdmin, &u.CreatedAt); err != nil {
				return err
			}
			s.Usuarios = append(s.Usuarios, u)
			return nil
		})
	if err != nil {
		return s, err
	}
	err = d.linhas(ctx, `SELECT id, library_id, kind, name, sort_name, IFNULL(year, 0), overview,
			IFNULL(rating, 0), genres, artist, IFNULL(tmdb_id, 0), poster, backdrop, meta_state FROM titles`,
		func(r *sql.Rows) error {
			var t SnapTitulo
			if err := r.Scan(&t.ID, &t.LibraryID, &t.Kind, &t.Name, &t.SortName, &t.Year, &t.Overview,
				&t.Rating, &t.Genres, &t.Artist, &t.TMDBID, &t.Poster, &t.Backdrop, &t.MetaState); err != nil {
				return err
			}
			s.Titulos = append(s.Titulos, t)
			return nil
		})
	if err != nil {
		return s, err
	}

	indice := map[int64]int{}
	err = d.linhas(ctx, `
		SELECT f.id, f.path, f.nuvem_key, f.library_id, IFNULL(f.title_id, 0), f.rel_path, f.ext, f.size,
		       f.mtime, f.media_type, IFNULL(f.duration, 0), IFNULL(f.width, 0), IFNULL(f.height, 0),
		       f.vcodec, f.acodec, IFNULL(f.track, 0), f.display_name, f.pix_fmt, f.vprofile,
		       f.channels, f.vbitrate, f.taken_at, f.localizacao,
		       IFNULL(e.season, 0), IFNULL(e.episode, 0), IFNULL(e.name, '')
		  FROM media_files f LEFT JOIN episodes e ON e.media_file_id = f.id`,
		func(r *sql.Rows) error {
			var (
				it   SnapItem
				id   int64
				path string
			)
			if err := r.Scan(&id, &path, &it.Ref.Key, &it.LibraryID, &it.TitleID, &it.RelPath, &it.Ext,
				&it.Size, &it.MTime, &it.Type, &it.Duration, &it.Width, &it.Height, &it.VCodec, &it.ACodec,
				&it.Track, &it.DisplayName, &it.PixFmt, &it.VProfile, &it.Channels, &it.VBitrate,
				&it.TakenAt, &it.Localizacao, &it.Season, &it.Episode, &it.EpName); err != nil {
				return err
			}
			it.Ref.Mac = path
			indice[id] = len(s.Itens)
			s.Itens = append(s.Itens, it)
			return nil
		})
	if err != nil {
		return s, err
	}
	err = d.linhas(ctx, `SELECT media_file_id, idx, kind, codec, lang, title, channels, is_default, forced
		FROM media_streams WHERE ext_path = ''`, func(r *sql.Rows) error {
		var (
			fileID int64
			st     Stream
			kind   string
		)
		if err := r.Scan(&fileID, &st.Index, &kind, &st.Codec, &st.Lang, &st.Title, &st.Channels,
			&st.Default, &st.Forced); err != nil {
			return err
		}
		st.Kind = StreamKind(kind)
		if i, ok := indice[fileID]; ok {
			s.Itens[i].Streams = append(s.Itens[i].Streams, st)
		}
		return nil
	})
	if err != nil {
		return s, err
	}

	refPorID := func(id int64) (Ref, bool) {
		i, ok := indice[id]
		if !ok {
			return Ref{}, false
		}
		return s.Itens[i].Ref, true
	}
	var progs []struct {
		p      SnapProgresso
		fileID int64
	}
	err = d.linhas(ctx, `SELECT user_id, media_file_id, position_sec, duration_sec, updated_at FROM progress`,
		func(r *sql.Rows) error {
			var x struct {
				p      SnapProgresso
				fileID int64
			}
			if err := r.Scan(&x.p.UserID, &x.fileID, &x.p.Posicao, &x.p.Duracao, &x.p.UpdatedAt); err != nil {
				return err
			}
			progs = append(progs, x)
			return nil
		})
	if err != nil {
		return s, err
	}
	for _, x := range progs {
		if ref, ok := refPorID(x.fileID); ok {
			x.p.Ref = ref
			s.Progresso = append(s.Progresso, x.p)
		}
	}
	err = d.linhas(ctx, `SELECT user_id, title_id, created_at FROM favorites`, func(r *sql.Rows) error {
		var f SnapFavorito
		if err := r.Scan(&f.UserID, &f.TitleID, &f.CreatedAt); err != nil {
			return err
		}
		s.Favoritos = append(s.Favoritos, f)
		return nil
	})
	return s, err
}

func (d *DB) linhas(ctx context.Context, q string, fn func(*sql.Rows) error) error {
	rows, err := d.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ResultadoDaImportacao resume o que um snapshot mudou.
type ResultadoDaImportacao struct {
	Itens, Removidos, Usuarios int
	Posters                    []string // pôsteres referenciados, para baixar os que faltam
}

// ImportarSnapshot aplica, na instância cloud, o estado publicado pelo Mac.
// Idempotente: aplicar o mesmo snapshot duas vezes não muda nada.
//
// Itens só do Mac entram com caminho "mac:<caminho>" e localização local,
// que aqui significa "no Mac, indisponível". Itens com chave no bucket
// entram como nuvem e tocam daqui.
func (d *DB) ImportarSnapshot(ctx context.Context, s Snapshot, margem time.Duration) (ResultadoDaImportacao, error) {
	var res ResultadoDaImportacao
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	agora := time.Now().Unix()

	// Bibliotecas e usuários: mesmos IDs do Mac, que é o dono.
	bibs := map[int64]bool{}
	for _, b := range s.Bibliotecas {
		bibs[b.ID] = true
	}
	if err := apagarAusentes(ctx, tx, "libraries", bibs); err != nil {
		return res, err
	}
	for _, b := range s.Bibliotecas {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO libraries (id, name, path, kind, enabled, created_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name, kind = excluded.kind, enabled = excluded.enabled`,
			b.ID, b.Name, fmt.Sprintf("%sbiblioteca/%d", PrefixoNoMac, b.ID), b.Kind, b.Enabled, agora); err != nil {
			return res, fmt.Errorf("biblioteca %d: %w", b.ID, err)
		}
	}
	users := map[int64]bool{}
	for _, u := range s.Usuarios {
		users[u.ID] = true
	}
	// Ausentes saem antes: um nome reaproveitado com ID novo colidiria.
	if err := apagarAusentes(ctx, tx, "users", users); err != nil {
		return res, err
	}
	for _, u := range s.Usuarios {
		var antigo string
		_ = tx.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, u.ID).Scan(&antigo)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, username, password_hash, must_change_password, is_admin, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET username = excluded.username, password_hash = excluded.password_hash,
				must_change_password = excluded.must_change_password, is_admin = excluded.is_admin`,
			u.ID, u.Username, u.Hash, u.MustChange, u.IsAdmin, u.CreatedAt); err != nil {
			return res, fmt.Errorf("usuário %s: %w", u.Username, err)
		}
		// Senha trocada no Mac derruba as sessões daqui também.
		if antigo != "" && antigo != u.Hash {
			if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, u.ID); err != nil {
				return res, err
			}
		}
	}
	res.Usuarios = len(users)

	// Títulos: a identidade (biblioteca, tipo, nome normalizado, ano) é a mesma
	// nos dois nós; os metadados do Mac valem.
	titulos := map[int64]int64{} // id no Mac → id aqui
	for _, t := range s.Titulos {
		if !bibs[t.LibraryID] {
			continue
		}
		var year any
		if t.Year > 0 {
			year = t.Year
		}
		var rating, tmdb any
		if t.Rating > 0 {
			rating = t.Rating
		}
		if t.TMDBID > 0 {
			tmdb = t.TMDBID
		}
		var id int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO titles (library_id, kind, name, sort_name, year, overview, rating, genres, artist,
			                    tmdb_id, poster, backdrop, meta_state, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (library_id, kind, sort_name, IFNULL(year, 0)) DO UPDATE SET
				name = excluded.name, overview = excluded.overview, rating = excluded.rating,
				genres = excluded.genres, artist = excluded.artist, tmdb_id = excluded.tmdb_id,
				poster = excluded.poster, backdrop = excluded.backdrop, meta_state = excluded.meta_state,
				updated_at = excluded.updated_at
			RETURNING id`,
			t.LibraryID, t.Kind, t.Name, t.SortName, year, t.Overview, rating, t.Genres, t.Artist,
			tmdb, t.Poster, t.Backdrop, t.MetaState, agora, agora).Scan(&id)
		if err != nil {
			return res, fmt.Errorf("título %q: %w", t.Name, err)
		}
		titulos[t.ID] = id
		for _, img := range []string{t.Poster, t.Backdrop} {
			if img != "" {
				res.Posters = append(res.Posters, img)
			}
		}
	}

	// Itens.
	vistos := map[int64]bool{}
	for _, it := range s.Itens {
		if !bibs[it.LibraryID] {
			continue
		}
		caminho, loc := PrefixoNoMac+it.Ref.Mac, LocalLocal
		if it.Ref.Key != "" {
			caminho, loc = "nuvem:"+it.Ref.Key, LocalNuvem
		}
		// A linha pode existir com outro caminho: o item ganhou (ou perdeu)
		// cópia na nuvem desde o último snapshot.
		var id int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM media_files
			 WHERE (? != '' AND nuvem_key = ?) OR path IN (?, ?, ?) LIMIT 1`,
			it.Ref.Key, it.Ref.Key, caminho, PrefixoNoMac+it.Ref.Mac, "nuvem:"+it.Ref.Key).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return res, err
		}
		var titleID any
		if tid, ok := titulos[it.TitleID]; ok {
			titleID = tid
		}
		if id == 0 {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO media_files (library_id, title_id, path, rel_path, ext, size, mtime, media_type,
					duration, width, height, vcodec, acodec, track, display_name, pix_fmt, vprofile,
					channels, vbitrate, taken_at, localizacao, nuvem_key, probed_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				RETURNING id`,
				it.LibraryID, titleID, caminho, it.RelPath, it.Ext, it.Size, it.MTime, it.Type,
				it.Duration, it.Width, it.Height, it.VCodec, it.ACodec, it.Track, it.DisplayName,
				it.PixFmt, it.VProfile, it.Channels, it.VBitrate, it.TakenAt, loc, it.Ref.Key, agora, agora).Scan(&id)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE media_files SET library_id = ?, title_id = ?, path = ?, rel_path = ?, ext = ?, size = ?,
					mtime = ?, media_type = ?, duration = ?, width = ?, height = ?, vcodec = ?, acodec = ?,
					track = ?, display_name = ?, pix_fmt = ?, vprofile = ?, channels = ?, vbitrate = ?,
					taken_at = ?, localizacao = ?, nuvem_key = ?
				 WHERE id = ?`,
				it.LibraryID, titleID, caminho, it.RelPath, it.Ext, it.Size, it.MTime, it.Type,
				it.Duration, it.Width, it.Height, it.VCodec, it.ACodec, it.Track, it.DisplayName,
				it.PixFmt, it.VProfile, it.Channels, it.VBitrate, it.TakenAt, loc, it.Ref.Key, id)
		}
		if err != nil {
			return res, fmt.Errorf("item %s: %w", it.RelPath, err)
		}
		vistos[id] = true
		res.Itens++

		if tid, ok := titulos[it.TitleID]; ok && (it.Season > 0 || it.Episode > 0) {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO episodes (title_id, media_file_id, season, episode, name) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT (title_id, season, episode) DO UPDATE SET
					media_file_id = excluded.media_file_id, name = excluded.name`,
				tid, id, it.Season, it.Episode, it.EpName); err != nil {
				return res, err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_streams WHERE media_file_id = ?`, id); err != nil {
			return res, err
		}
		for _, st := range it.Streams {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO media_streams (media_file_id, idx, kind, codec, lang, title, channels, is_default, forced)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, st.Index, string(st.Kind), st.Codec, st.Lang, st.Title, st.Channels, st.Default, st.Forced); err != nil {
				return res, err
			}
		}
	}

	// Remoções: o que é do Mac e sumiu do snapshot sai. Itens da nuvem
	// recentes ficam: podem ser uploads feitos aqui que o Mac ainda não viu.
	limite := s.GeradoEm.Add(-margem).Unix()
	rows, err := tx.QueryContext(ctx, `SELECT id, path, created_at FROM media_files`)
	if err != nil {
		return res, err
	}
	var apagar []int64
	for rows.Next() {
		var (
			id, criado int64
			path       string
		)
		if err := rows.Scan(&id, &path, &criado); err != nil {
			rows.Close()
			return res, err
		}
		if vistos[id] {
			continue
		}
		if strings.HasPrefix(path, PrefixoNoMac) || criado < limite {
			apagar = append(apagar, id)
		}
	}
	rows.Close()
	for _, id := range apagar {
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_files WHERE id = ?`, id); err != nil {
			return res, err
		}
	}
	res.Removidos = len(apagar)
	if _, err := tx.ExecContext(ctx, `DELETE FROM titles WHERE NOT EXISTS
		(SELECT 1 FROM media_files f WHERE f.title_id = titles.id)`); err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}

	// Progresso (LWW) e favoritos depois do commit: dependem dos IDs finais.
	for _, p := range s.Progresso {
		if !users[p.UserID] {
			continue
		}
		if fileID, err := d.ArquivoPorRef(ctx, p.Ref); err == nil {
			if err := d.ProgressoLWW(ctx, p.UserID, fileID, p.Posicao, p.Duracao, p.UpdatedAt); err != nil {
				return res, err
			}
		}
	}
	for _, f := range s.Favoritos {
		if tid, ok := titulos[f.TitleID]; ok && users[f.UserID] {
			if _, err := d.ExecContext(ctx, `INSERT INTO favorites (user_id, title_id, created_at)
				VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, f.UserID, tid, f.CreatedAt); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

func apagarAusentes(ctx context.Context, tx *sql.Tx, tabela string, manter map[int64]bool) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM `+tabela)
	if err != nil {
		return err
	}
	var fora []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !manter[id] {
			fora = append(fora, id)
		}
	}
	rows.Close()
	for _, id := range fora {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+tabela+` WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}
