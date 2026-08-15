package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind é o tipo de conteúdo de uma biblioteca.
type Kind string

const (
	KindMovie Kind = "movie"
	KindTV    Kind = "tv"
	KindMusic Kind = "music"
	KindPhoto Kind = "photo"
)

var ErrNotFound = errors.New("não encontrado")

// ParseKind valida um tipo vindo da CLI ou da API, aceitando os apelidos em
// português que são naturais de digitar.
func ParseKind(s string) (Kind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "movie", "movies", "filme", "filmes":
		return KindMovie, nil
	case "tv", "serie", "series", "séries", "show", "shows":
		return KindTV, nil
	case "music", "musica", "músicas", "musicas", "audio":
		return KindMusic, nil
	case "photo", "photos", "foto", "fotos", "imagem", "imagens":
		return KindPhoto, nil
	}
	return "", fmt.Errorf("tipo inválido %q: use movie, tv, music ou photo", s)
}

// GuessKind infere o tipo pelo nome da pasta; o segundo retorno é false quando
// o nome não diz nada e o usuário precisa informar --kind.
func GuessKind(folder string) (Kind, bool) {
	if k, err := ParseKind(folder); err == nil {
		return k, true
	}
	return "", false
}

type Library struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	Kind      Kind       `json:"kind"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
	ScannedAt *time.Time `json:"scanned_at,omitempty"`
}

func (d *DB) AddLibrary(ctx context.Context, name, path string, kind Kind) (Library, error) {
	now := time.Now()
	res, err := d.ExecContext(ctx,
		`INSERT INTO libraries (name, path, kind, enabled, created_at) VALUES (?, ?, ?, 1, ?)`,
		name, path, string(kind), now.Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Library{}, fmt.Errorf("a pasta %s já é uma biblioteca", path)
		}
		return Library{}, fmt.Errorf("inserindo biblioteca: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Library{}, err
	}
	return Library{ID: id, Name: name, Path: path, Kind: kind, Enabled: true, CreatedAt: now}, nil
}

func (d *DB) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, name, path, kind, enabled, created_at, scanned_at
		   FROM libraries ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listando bibliotecas: %w", err)
	}
	defer rows.Close()

	var libs []Library
	for rows.Next() {
		lib, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		libs = append(libs, lib)
	}
	return libs, rows.Err()
}

func (d *DB) Library(ctx context.Context, id int64) (Library, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, name, path, kind, enabled, created_at, scanned_at
		   FROM libraries WHERE id = ?`, id)
	lib, err := scanLibrary(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Library{}, ErrNotFound
	}
	return lib, err
}

func (d *DB) DeleteLibrary(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("removendo biblioteca: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) MarkLibraryScanned(ctx context.Context, id int64, at time.Time) error {
	_, err := d.ExecContext(ctx, `UPDATE libraries SET scanned_at = ? WHERE id = ?`, at.Unix(), id)
	return err
}

// scanner cobre *sql.Row e *sql.Rows, que expõem o mesmo Scan.
type scanner interface{ Scan(dest ...any) error }

func scanLibrary(s scanner) (Library, error) {
	var (
		lib       Library
		kind      string
		enabled   int
		createdAt int64
		scannedAt sql.NullInt64
	)
	if err := s.Scan(&lib.ID, &lib.Name, &lib.Path, &kind, &enabled, &createdAt, &scannedAt); err != nil {
		return Library{}, err
	}
	lib.Kind = Kind(kind)
	lib.Enabled = enabled != 0
	lib.CreatedAt = time.Unix(createdAt, 0)
	if scannedAt.Valid {
		t := time.Unix(scannedAt.Int64, 0)
		lib.ScannedAt = &t
	}
	return lib, nil
}
