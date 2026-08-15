package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TitleMeta é o pacote de metadados que o TMDB devolve para um título.
type TitleMeta struct {
	ID       int64
	Name     string
	Year     int
	Overview string
	Rating   float64
	Genres   string
	TMDBID   int
	Poster   string
	Backdrop string
	State    string // matched | unmatched | manual
}

// PendingTitles lista os títulos que ainda não passaram pelo enriquecimento.
// Com all=true, devolve todos (usado no "buscar metadados de novo").
func (d *DB) PendingTitles(ctx context.Context, all bool, limit int) ([]Title, error) {
	query := `
		SELECT id, library_id, kind, name, sort_name, IFNULL(year, 0), overview,
		       IFNULL(rating, 0), genres, artist, IFNULL(tmdb_id, 0), poster, backdrop, meta_state
		  FROM titles`
	if !all {
		query += ` WHERE meta_state = 'pending'`
	}
	query += ` ORDER BY id LIMIT ?`
	if limit <= 0 {
		limit = 5000
	}

	rows, err := d.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("listando títulos pendentes: %w", err)
	}
	defer rows.Close()

	var titles []Title
	for rows.Next() {
		var (
			t    Title
			kind string
		)
		if err := rows.Scan(&t.ID, &t.LibraryID, &kind, &t.Name, &t.SortName, &t.Year,
			&t.Overview, &t.Rating, &t.Genres, &t.Artist, &t.TMDBID, &t.Poster, &t.Backdrop,
			&t.MetaState); err != nil {
			return nil, err
		}
		t.Kind = TitleKind(kind)
		titles = append(titles, t)
	}
	return titles, rows.Err()
}

// UpdateTitleMeta grava o resultado do casamento com o TMDB.
func (d *DB) UpdateTitleMeta(ctx context.Context, m TitleMeta) error {
	var year any
	if m.Year > 0 {
		year = m.Year
	}
	_, err := d.ExecContext(ctx, `
		UPDATE titles
		   SET name       = CASE WHEN ? != '' THEN ? ELSE name END,
		       year       = COALESCE(?, year),
		       overview   = ?,
		       rating     = ?,
		       genres     = ?,
		       tmdb_id    = ?,
		       poster     = CASE WHEN ? != '' THEN ? ELSE poster END,
		       backdrop   = CASE WHEN ? != '' THEN ? ELSE backdrop END,
		       meta_state = ?,
		       updated_at = ?
		 WHERE id = ?`,
		m.Name, m.Name, year, m.Overview, m.Rating, m.Genres, m.TMDBID,
		m.Poster, m.Poster, m.Backdrop, m.Backdrop, m.State, time.Now().Unix(), m.ID)
	if err != nil {
		return fmt.Errorf("gravando metadados do título %d: %w", m.ID, err)
	}
	return nil
}

// SetTitleState marca o título como sem match confiável, por exemplo.
func (d *DB) SetTitleState(ctx context.Context, id int64, state string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE titles SET meta_state = ?, updated_at = ? WHERE id = ?`,
		state, time.Now().Unix(), id)
	return err
}

// SetTitlePoster grava uma capa gerada localmente (frame do vídeo, capa do MP3).
func (d *DB) SetTitlePoster(ctx context.Context, id int64, poster string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE titles SET poster = ?, updated_at = ? WHERE id = ?`,
		poster, time.Now().Unix(), id)
	return err
}

// TitleSeasons lista as temporadas com episódios indexados.
func (d *DB) TitleSeasons(ctx context.Context, titleID int64) ([]int, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT DISTINCT season FROM episodes WHERE title_id = ? AND season > 0 ORDER BY season`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seasons []int
	for rows.Next() {
		var s int
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		seasons = append(seasons, s)
	}
	return seasons, rows.Err()
}

// UpdateEpisodeMeta completa nome, sinopse e still de um episódio já indexado.
// Não cria episódio que não exista em disco.
func (d *DB) UpdateEpisodeMeta(ctx context.Context, titleID int64, season, episode int, name, overview, still, airDate string) error {
	_, err := d.ExecContext(ctx, `
		UPDATE episodes
		   SET name     = CASE WHEN ? != '' THEN ? ELSE name END,
		       overview = ?,
		       still    = CASE WHEN ? != '' THEN ? ELSE still END,
		       air_date = ?
		 WHERE title_id = ? AND season = ? AND episode = ?`,
		name, name, overview, still, still, airDate, titleID, season, episode)
	return err
}

// FirstFileOfTitle devolve um arquivo representativo, para gerar thumbnail.
func (d *DB) FirstFileOfTitle(ctx context.Context, titleID int64) (MediaFile, error) {
	var (
		f     MediaFile
		mtype string
	)
	err := d.QueryRowContext(ctx, `
		SELECT f.id, f.library_id, f.path, f.rel_path, f.ext, f.size, f.mtime, f.media_type,
		       IFNULL(f.duration, 0), f.thumb
		  FROM media_files f
		  LEFT JOIN episodes e ON e.media_file_id = f.id
		 WHERE f.title_id = ?
		 ORDER BY IFNULL(e.season, 0), IFNULL(e.episode, 0), IFNULL(f.track, 0), f.rel_path
		 LIMIT 1`, titleID).
		Scan(&f.ID, &f.LibraryID, &f.Path, &f.RelPath, &f.Ext, &f.Size, &f.MTime, &mtype,
			&f.Duration, &f.Thumb)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaFile{}, ErrNotFound
	}
	if err != nil {
		return MediaFile{}, err
	}
	f.Type = MediaType(mtype)
	return f, nil
}

func (d *DB) SetFileThumb(ctx context.Context, fileID int64, thumb string) error {
	_, err := d.ExecContext(ctx, `UPDATE media_files SET thumb = ? WHERE id = ?`, thumb, fileID)
	return err
}
