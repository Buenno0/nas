package db

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"
)

// Benchmark das listagens em um acervo grande, com o driver Go de verdade.
// Só roda com -bench, então não pesa no `go test ./...`.
//
//	go test ./internal/db -bench Listagens -benchtime 50x
func BenchmarkListagens(b *testing.B) {
	const (
		totalTitulos  = 20_000
		arquivosCada  = 3
		paginaTamanho = 60
	)

	ctx := context.Background()
	d, err := OpenAt(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	semear(b, d, totalTitulos, arquivosCada)

	// A consulta que existia antes da reescrita, para a comparação ser honesta.
	const antiga = `
		SELECT t.id, t.library_id, t.kind, t.name, IFNULL(t.year, 0), t.artist,
		       IFNULL(t.rating, 0), t.poster, t.backdrop, t.genres, t.meta_state,
		       COUNT(f.id), IFNULL(SUM(f.duration), 0)
		  FROM titles t
		  LEFT JOIN media_files f ON f.title_id = t.id
		 WHERE t.library_id = ?
		 GROUP BY t.id
		 ORDER BY t.sort_name
		 LIMIT ? OFFSET ?`

	b.Run("grade/antiga/offset0", func(b *testing.B) {
		for b.Loop() {
			consumir(b, d, antiga, 1, paginaTamanho, 0)
		}
	})
	b.Run("grade/nova/offset0", func(b *testing.B) {
		for b.Loop() {
			if _, err := d.ListTitles(ctx, TitleFilter{LibraryID: 1, Limit: paginaTamanho}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("grade/antiga/offset3000", func(b *testing.B) {
		for b.Loop() {
			consumir(b, d, antiga, 1, paginaTamanho, 3000)
		}
	})
	b.Run("grade/nova/offset3000", func(b *testing.B) {
		for b.Loop() {
			if _, err := d.ListTitles(ctx, TitleFilter{LibraryID: 1, Limit: paginaTamanho, Offset: 3000}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("home/recentes", func(b *testing.B) {
		for b.Loop() {
			if _, err := d.RecentTitles(ctx, 20); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("busca/like", func(b *testing.B) {
		for b.Loop() {
			if _, err := d.ListTitles(ctx, TitleFilter{Query: "duna", Limit: paginaTamanho}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func consumir(b *testing.B, d *DB, query string, args ...any) {
	b.Helper()
	rows, err := d.Query(query, args...)
	if err != nil {
		b.Fatal(err)
	}
	for rows.Next() {
		var c TitleCard
		var kind string
		if err := rows.Scan(&c.ID, &c.LibraryID, &kind, &c.Name, &c.Year, &c.Artist,
			&c.Rating, &c.Poster, &c.Backdrop, &c.Genres, &c.MetaState, &c.Files, &c.Duration); err != nil {
			rows.Close()
			b.Fatal(err)
		}
	}
	rows.Close()
}

// semear insere direto em SQL, numa transação só: o caminho normal de upsert
// levaria minutos e não é o que está sendo medido.
func semear(b *testing.B, d *DB, titulos, arquivosPorTitulo int) {
	b.Helper()

	palavras := []string{"odisseia", "duna", "matrix", "cidade", "noite", "alien",
		"senhor", "aneis", "tempo", "fogo", "gelo", "sombra", "luz", "viagem", "estrela"}
	rng := rand.New(rand.NewSource(42))
	agora := time.Now().Unix()

	if _, err := d.Exec(`INSERT INTO libraries (name, path, kind, enabled, created_at)
	                     VALUES ('Filmes', '/fake', 'movie', 1, ?)`, agora); err != nil {
		b.Fatal(err)
	}

	tx, err := d.Begin()
	if err != nil {
		b.Fatal(err)
	}
	insTitulo, err := tx.Prepare(`INSERT INTO titles
		(library_id, kind, name, sort_name, year, overview, rating, genres, artist,
		 poster, backdrop, meta_state, created_at, updated_at)
		VALUES (1, 'movie', ?, ?, ?, 'sinopse', 7.5, 'Ação', '', '', '', 'matched', ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	insArquivo, err := tx.Prepare(`INSERT INTO media_files
		(library_id, title_id, path, rel_path, ext, size, mtime, media_type,
		 duration, width, height, vcodec, acodec, track, display_name, probed_at, created_at)
		VALUES (1, ?, ?, ?, '.mp4', 1000000, ?, 'video', 5400, 1920, 1080, 'h264', 'aac', 0, '', ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}

	for i := range titulos {
		nome := fmt.Sprintf("%s %s %d", palavras[rng.Intn(len(palavras))], palavras[rng.Intn(len(palavras))], i)
		criado := agora - int64(rng.Intn(1_000_000))
		res, err := insTitulo.Exec(nome, nome, 1970+rng.Intn(57), criado, criado)
		if err != nil {
			b.Fatal(err)
		}
		titleID, err := res.LastInsertId()
		if err != nil {
			b.Fatal(err)
		}
		for k := range arquivosPorTitulo {
			caminho := fmt.Sprintf("/fake/%d/%d.mp4", titleID, k)
			if _, err := insArquivo.Exec(titleID, caminho, caminho, agora, agora, agora); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	if _, err := d.Exec("ANALYZE"); err != nil {
		b.Fatal(err)
	}
}
