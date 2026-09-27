package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoricoSoContaOQueFoiAssistido(t *testing.T) {
	ctx := context.Background()
	d, err := OpenAt(filepath.Join(t.TempDir(), "nas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	uid, _ := d.CreateUser(ctx, "ana", "$argon2id$x", false, false)
	lib, _ := d.AddLibrary(ctx, "Filmes", t.TempDir(), KindMovie)
	fid, _ := d.UpsertFile(ctx, MediaFile{LibraryID: lib.ID, Path: "/x/a.mkv", RelPath: "a.mkv", Ext: ".mkv", Size: 1, MTime: 1, Type: TypeVideo}, true)

	agora := time.Now()
	casos := []struct {
		nome     string
		de, para float64
		passou   time.Duration
		quero    float64
	}{
		{"assistiu 10 s", 100, 110, 10 * time.Second, 10},
		{"voltou (seek para trás)", 110, 50, 10 * time.Second, 0},
		{"pulou 20 min", 50, 1250, 10 * time.Second, 0},
		{"adiantou mais que o relógio", 1250, 1400, 10 * time.Second, 0},
		{"aba parada 4 min", 1400, 1640, 4 * time.Minute, 240},
	}
	total := 0.0
	for _, c := range casos {
		got := d.SomaHistorico(ctx, uid.ID, fid, c.de, c.para, agora.Add(-c.passou), agora)
		if got != c.quero {
			t.Errorf("%s: somou %.0f, quero %.0f", c.nome, got, c.quero)
		}
		total += got
	}
	var gravado float64
	d.QueryRow(`SELECT segundos FROM historico WHERE user_id = ? AND media_file_id = ?`, uid.ID, fid).Scan(&gravado)
	if gravado != total {
		t.Fatalf("histórico gravou %.0f, quero %.0f", gravado, total)
	}
}

func TestPrateleirasNovas(t *testing.T) {
	ctx := context.Background()
	d, err := OpenAt(filepath.Join(t.TempDir(), "nas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	u, _ := d.CreateUser(ctx, "ana", "$argon2id$x", false, false)
	filmes, _ := d.AddLibrary(ctx, "Filmes", t.TempDir(), KindMovie)
	filme := func(nome string, dur float64, tmdb int) int64 {
		tid, _ := d.UpsertTitle(ctx, Title{LibraryID: filmes.ID, Kind: "movie", Name: nome, SortName: nome})
		d.ExecContext(ctx, `UPDATE titles SET tmdb_id = ? WHERE id = ?`, tmdb, tid)
		f, _ := d.UpsertFile(ctx, MediaFile{LibraryID: filmes.ID, Path: "/x/" + nome, RelPath: nome, Ext: ".mkv", Size: 1, MTime: 1, Type: TypeVideo, Duration: dur}, true)
		d.SetFileTitle(ctx, f, tid)
		return tid
	}
	curto := filme("Curto", 85*60, 10)
	filme("Longo", 180*60, 20)
	visto := filme("Visto", 90*60, 30)
	var fVisto int64
	d.QueryRow(`SELECT id FROM media_files WHERE title_id = ?`, visto).Scan(&fVisto)
	d.SaveProgress(ctx, u.ID, fVisto, 90*60, 90*60)

	cabe, err := d.CabeEm(ctx, u.ID, 95*60, 10)
	if err != nil || len(cabe) != 1 || cabe[0].ID != curto {
		t.Fatalf("cabe em 1h30: %v %+v", err, cabe)
	}
	if id, _, _, tmdb, err := d.UltimoTerminado(ctx, u.ID); err != nil || id != visto || tmdb != 30 {
		t.Fatalf("último terminado: %d %d %v", id, tmdb, err)
	}
	// Recomendados na ordem do TMDB, só o que existe e não foi terminado.
	rec, err := d.TitulosPorTMDB(ctx, u.ID, []int{999, 20, 30, 10}, 10)
	if err != nil || len(rec) != 2 || rec[0].Name != "Longo" || rec[1].Name != "Curto" {
		t.Fatalf("recomendados: %v %+v", err, rec)
	}
}
