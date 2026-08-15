package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MediaType classifica o arquivo em disco.
type MediaType string

const (
	TypeVideo MediaType = "video"
	TypeAudio MediaType = "audio"
	TypePhoto MediaType = "photo"
)

// MediaFile é uma linha de media_files. Campos numéricos em zero significam
// "desconhecido" (arquivo ainda não passou pelo ffprobe).
type MediaFile struct {
	ID        int64     `json:"id"`
	LibraryID int64     `json:"library_id"`
	TitleID   *int64    `json:"title_id,omitempty"`
	Path      string    `json:"-"` // caminho absoluto nunca vai para o cliente
	RelPath   string    `json:"rel_path"`
	Ext       string    `json:"ext"`
	Size      int64     `json:"size"`
	MTime     int64     `json:"mtime"`
	Type      MediaType `json:"media_type"`
	Duration  float64   `json:"duration"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	VCodec    string    `json:"vcodec"`
	ACodec    string    `json:"acodec"`
	Track     int       `json:"track"`
	Thumb     string    `json:"thumb"`
	// DisplayName vem das tags do arquivo (título da faixa em MP3/FLAC).
	DisplayName string `json:"-"`
}

// Stamp é a assinatura usada para detectar mudanças sem reler o arquivo.
type Stamp struct {
	ID       int64
	Size     int64
	MTime    int64
	Probed   bool
	HasTitle bool
}

// FileStamps devolve, por caminho absoluto, a assinatura dos arquivos já
// indexados na biblioteca.
func (d *DB) FileStamps(ctx context.Context, libraryID int64) (map[string]Stamp, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, path, size, mtime, probed_at, title_id FROM media_files WHERE library_id = ?`, libraryID)
	if err != nil {
		return nil, fmt.Errorf("lendo índice da biblioteca: %w", err)
	}
	defer rows.Close()

	stamps := make(map[string]Stamp)
	for rows.Next() {
		var (
			path    string
			st      Stamp
			probed  sql.NullInt64
			titleID sql.NullInt64
		)
		if err := rows.Scan(&st.ID, &path, &st.Size, &st.MTime, &probed, &titleID); err != nil {
			return nil, err
		}
		st.Probed = probed.Valid && probed.Int64 > 0
		st.HasTitle = titleID.Valid
		stamps[path] = st
	}
	return stamps, rows.Err()
}

// UpsertFile insere ou atualiza um arquivo pelo caminho e devolve o id.
func (d *DB) UpsertFile(ctx context.Context, f MediaFile, probed bool) (int64, error) {
	var probedAt any
	if probed {
		probedAt = time.Now().Unix()
	}

	var id int64
	err := d.QueryRowContext(ctx, `
		INSERT INTO media_files
			(library_id, path, rel_path, ext, size, mtime, media_type,
			 duration, width, height, vcodec, acodec, track, display_name,
			 probed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET
			rel_path     = excluded.rel_path,
			size         = excluded.size,
			mtime        = excluded.mtime,
			media_type   = excluded.media_type,
			duration     = excluded.duration,
			width        = excluded.width,
			height       = excluded.height,
			vcodec       = excluded.vcodec,
			acodec       = excluded.acodec,
			track        = excluded.track,
			display_name = excluded.display_name,
			probed_at    = COALESCE(excluded.probed_at, media_files.probed_at)
		RETURNING id`,
		f.LibraryID, f.Path, f.RelPath, f.Ext, f.Size, f.MTime, string(f.Type),
		f.Duration, f.Width, f.Height, f.VCodec, f.ACodec, f.Track, f.DisplayName,
		probedAt, time.Now().Unix(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("gravando %s: %w", f.RelPath, err)
	}
	return id, nil
}

// DeleteFilesByID remove do índice arquivos que sumiram do disco.
func (d *DB) DeleteFilesByID(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `DELETE FROM media_files WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, id); err != nil {
			return fmt.Errorf("removendo arquivo %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// SetFileTitle associa um arquivo ao título ao qual ele pertence.
func (d *DB) SetFileTitle(ctx context.Context, fileID, titleID int64) error {
	_, err := d.ExecContext(ctx, `UPDATE media_files SET title_id = ? WHERE id = ?`, titleID, fileID)
	return err
}

// TitleKind é o tipo de um título na interface.
type TitleKind string

const (
	TitleMovie  TitleKind = "movie"
	TitleTV     TitleKind = "tv"
	TitleAlbum  TitleKind = "album"
	TitlePhotos TitleKind = "photos"
)

type Title struct {
	ID        int64     `json:"id"`
	LibraryID int64     `json:"library_id"`
	Kind      TitleKind `json:"kind"`
	Name      string    `json:"name"`
	SortName  string    `json:"-"`
	Year      int       `json:"year,omitempty"`
	Overview  string    `json:"overview,omitempty"`
	Rating    float64   `json:"rating,omitempty"`
	Genres    string    `json:"genres,omitempty"`
	Artist    string    `json:"artist,omitempty"`
	TMDBID    int       `json:"-"`
	Poster    string    `json:"poster,omitempty"`
	Backdrop  string    `json:"backdrop,omitempty"`
	MetaState string    `json:"meta_state"`
}

// UpsertTitle cria o título ou devolve o existente com a mesma identidade
// (biblioteca + tipo + nome normalizado + ano).
func (d *DB) UpsertTitle(ctx context.Context, t Title) (int64, error) {
	var year any
	if t.Year > 0 {
		year = t.Year
	}
	now := time.Now().Unix()

	var id int64
	err := d.QueryRowContext(ctx, `
		INSERT INTO titles (library_id, kind, name, sort_name, year, artist, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, kind, sort_name, IFNULL(year, 0)) DO UPDATE SET
			name       = excluded.name,
			artist     = CASE WHEN excluded.artist != '' THEN excluded.artist ELSE titles.artist END,
			updated_at = excluded.updated_at
		RETURNING id`,
		t.LibraryID, string(t.Kind), t.Name, t.SortName, year, t.Artist, now, now,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("gravando título %q: %w", t.Name, err)
	}
	return id, nil
}

type Episode struct {
	TitleID     int64
	MediaFileID int64
	Season      int
	Episode     int
	Name        string
}

func (d *DB) UpsertEpisode(ctx context.Context, ep Episode) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO episodes (title_id, media_file_id, season, episode, name)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (title_id, season, episode) DO UPDATE SET
			media_file_id = excluded.media_file_id,
			name = CASE WHEN episodes.name = '' THEN excluded.name ELSE episodes.name END`,
		ep.TitleID, ep.MediaFileID, ep.Season, ep.Episode, ep.Name)
	if err != nil {
		return fmt.Errorf("gravando episódio S%02dE%02d: %w", ep.Season, ep.Episode, err)
	}
	return nil
}

// PruneEmptyTitles remove títulos da biblioteca que ficaram sem nenhum arquivo
// (aconteceu de os arquivos serem apagados do disco).
func (d *DB) PruneEmptyTitles(ctx context.Context, libraryID int64) (int64, error) {
	res, err := d.ExecContext(ctx, `
		DELETE FROM titles
		 WHERE library_id = ?
		   AND id NOT IN (SELECT title_id FROM media_files WHERE title_id IS NOT NULL)`,
		libraryID)
	if err != nil {
		return 0, fmt.Errorf("limpando títulos órfãos: %w", err)
	}
	return res.RowsAffected()
}
