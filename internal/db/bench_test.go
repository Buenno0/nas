package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// serieSintetica monta uma série com n episódios, para medir o custo das
// listagens sem depender do acervo real de ninguém.
func serieSintetica(tb testing.TB, n int) (*DB, int64, int64) {
	tb.Helper()
	d, err := OpenAt(filepath.Join(tb.TempDir(), "bench.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { d.Close() })

	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `INSERT INTO libraries (name, path, kind, created_at)
		VALUES ('Séries', '/tmp/series', 'tv', 0)`); err != nil {
		tb.Fatal(err)
	}
	res, err := d.ExecContext(ctx, `INSERT INTO titles (library_id, kind, name, sort_name, created_at, updated_at)
		VALUES (1, 'tv', 'Série Longa', 'serie longa', 0, 0)`)
	if err != nil {
		tb.Fatal(err)
	}
	titleID, _ := res.LastInsertId()

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		tb.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		res, err := tx.ExecContext(ctx, `INSERT INTO media_files
			(library_id, title_id, path, rel_path, ext, size, mtime, media_type, duration, created_at)
			VALUES (1, ?, ?, ?, '.mkv', 1000000, 0, 'video', 1400, 0)`,
			titleID, fmt.Sprintf("/tmp/series/ep%04d.mkv", i), fmt.Sprintf("ep%04d.mkv", i))
		if err != nil {
			tb.Fatal(err)
		}
		fileID, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx, `INSERT INTO episodes (title_id, media_file_id, season, episode, name)
			VALUES (?, ?, ?, ?, ?)`,
			titleID, fileID, 1+i/25, i, fmt.Sprintf("Episódio %d", i)); err != nil {
			tb.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `ANALYZE`); err != nil {
		tb.Fatal(err)
	}

	// Um usuário, para o LEFT JOIN de progresso existir de verdade.
	if _, err := d.ExecContext(ctx, `INSERT INTO users (username, password_hash, created_at)
		VALUES ('bench', 'x', 0)`); err != nil {
		tb.Fatal(err)
	}
	return d, titleID, 1
}

// BenchmarkTitleFiles mede a página de uma série de 1000 episódios — o
// LEFT JOIN episodes ON e.media_file_id = f.id.
func BenchmarkTitleFiles(b *testing.B) {
	d, titleID, userID := serieSintetica(b, 1000)
	ctx := context.Background()

	for b.Loop() {
		if _, err := d.TitleFiles(ctx, titleID, userID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTitleFilesSemIndice é o mesmo com o índice de media_file_id
// derrubado: existe para provar, e não supor, que o índice paga por si.
func BenchmarkTitleFilesSemIndice(b *testing.B) {
	d, titleID, userID := serieSintetica(b, 1000)
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `DROP INDEX IF EXISTS idx_episodes_file`); err != nil {
		b.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `ANALYZE`); err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		if _, err := d.TitleFiles(ctx, titleID, userID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTitleFilesSoQuery executa a mesma query mas descarta as linhas sem
// materializar structs. A diferença contra BenchmarkTitleFiles diz quanto do
// custo é do SQLite e quanto é do driver Go copiando 1000 linhas.
func BenchmarkTitleFilesSoQuery(b *testing.B) {
	d, titleID, _ := serieSintetica(b, 1000)
	ctx := context.Background()

	for b.Loop() {
		rows, err := d.QueryContext(ctx, `
			SELECT f.id, f.rel_path, f.ext, f.media_type, f.size, IFNULL(f.duration, 0),
			       IFNULL(f.width, 0), IFNULL(f.height, 0), f.vcodec, f.acodec,
			       IFNULL(f.track, 0), f.thumb, f.display_name,
			       IFNULL(p.position_sec, 0), IFNULL(p.finished, 0),
			       IFNULL(e.season, 0), IFNULL(e.episode, 0), IFNULL(e.name, '')
			  FROM media_files f
			  LEFT JOIN progress p ON p.media_file_id = f.id AND p.user_id = 1
			  LEFT JOIN episodes e ON e.media_file_id = f.id
			 WHERE f.title_id = ?
			 ORDER BY IFNULL(e.season, 0), IFNULL(e.episode, 0), IFNULL(f.track, 0), f.rel_path`,
			titleID)
		if err != nil {
			b.Fatal(err)
		}
		n := 0
		for rows.Next() {
			n++
		}
		rows.Close()
		if n != 1000 {
			b.Fatalf("veio %d linhas", n)
		}
	}
}

// BenchmarkTitleFilesParalelo é o teste do pool de conexões: várias
// requisições ao mesmo tempo, como acontece quando dois celulares navegam
// junto e o scan roda no fundo.
func BenchmarkTitleFilesParalelo(b *testing.B) {
	d, titleID, userID := serieSintetica(b, 200)
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := d.TitleFiles(ctx, titleID, userID); err != nil {
				b.Fatal(err)
			}
		}
	})
}
