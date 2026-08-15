package db

import (
	"context"
	"path/filepath"
	"testing"
)

// openTest cria um banco novo em disco temporário, já migrado.
func openTest(t *testing.T) *DB {
	t.Helper()
	database, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrindo banco de teste: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// seedTitle cria um título com n arquivos de duração fixa.
func seedTitle(t *testing.T, d *DB, libID int64, name string, files int, duration float64) int64 {
	t.Helper()
	ctx := context.Background()

	titleID, err := d.UpsertTitle(ctx, Title{
		LibraryID: libID,
		Kind:      TitleMovie,
		Name:      name,
		SortName:  name,
	})
	if err != nil {
		t.Fatalf("criando título %q: %v", name, err)
	}

	for i := range files {
		fileID, err := d.UpsertFile(ctx, MediaFile{
			LibraryID: libID,
			Path:      filepath.Join("/fake", name, string(rune('a'+i))+".mp4"),
			RelPath:   name + "/" + string(rune('a'+i)) + ".mp4",
			Ext:       ".mp4",
			Size:      1000,
			MTime:     1,
			Type:      TypeVideo,
			Duration:  duration,
		}, true)
		if err != nil {
			t.Fatalf("criando arquivo de %q: %v", name, err)
		}
		if err := d.SetFileTitle(ctx, fileID, titleID); err != nil {
			t.Fatalf("vinculando arquivo: %v", err)
		}
	}
	return titleID
}

// A contagem e a soma passaram de LEFT JOIN + GROUP BY para subconsulta.
// Estes testes fixam o comportamento que não pode mudar com a troca.
func TestListTitlesAgregados(t *testing.T) {
	ctx := context.Background()
	d := openTest(t)

	lib, err := d.AddLibrary(ctx, "Filmes", "/fake", KindMovie)
	if err != nil {
		t.Fatal(err)
	}

	seedTitle(t, d, lib.ID, "Alfa", 3, 600)
	seedTitle(t, d, lib.ID, "Beta", 1, 120)
	seedTitle(t, d, lib.ID, "Gama", 0, 0) // título sem arquivo em disco

	cards, err := d.ListTitles(ctx, TitleFilter{LibraryID: lib.ID})
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("recebi %d títulos, quero 3 (título sem arquivo também aparece)", len(cards))
	}

	quero := map[string]struct {
		files    int
		duration float64
	}{
		"Alfa": {3, 1800},
		"Beta": {1, 120},
		"Gama": {0, 0},
	}
	for _, card := range cards {
		esperado, ok := quero[card.Name]
		if !ok {
			t.Errorf("título inesperado: %q", card.Name)
			continue
		}
		if card.Files != esperado.files {
			t.Errorf("%s: %d arquivos, quero %d", card.Name, card.Files, esperado.files)
		}
		if card.Duration != esperado.duration {
			t.Errorf("%s: duração %.0f, quero %.0f", card.Name, card.Duration, esperado.duration)
		}
	}
}

func TestListTitlesOrdemEFiltro(t *testing.T) {
	ctx := context.Background()
	d := openTest(t)

	lib, _ := d.AddLibrary(ctx, "Filmes", "/fake", KindMovie)
	outra, _ := d.AddLibrary(ctx, "Series", "/fake2", KindTV)

	seedTitle(t, d, lib.ID, "Zulu", 1, 60)
	seedTitle(t, d, lib.ID, "Alfa", 1, 60)
	seedTitle(t, d, outra.ID, "Meio", 1, 60)

	cards, err := d.ListTitles(ctx, TitleFilter{LibraryID: lib.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 || cards[0].Name != "Alfa" || cards[1].Name != "Zulu" {
		t.Errorf("ordem/filtro errados: %v", nomes(cards))
	}

	busca, err := d.ListTitles(ctx, TitleFilter{Query: "zul"})
	if err != nil {
		t.Fatal(err)
	}
	if len(busca) != 1 || busca[0].Name != "Zulu" {
		t.Errorf("busca devolveu %v", nomes(busca))
	}

	total, err := d.CountTitles(ctx, TitleFilter{LibraryID: lib.ID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("contagem = %d, quero 2", total)
	}
}

func TestRecentesEFavoritos(t *testing.T) {
	ctx := context.Background()
	d := openTest(t)

	lib, _ := d.AddLibrary(ctx, "Filmes", "/fake", KindMovie)
	primeiro := seedTitle(t, d, lib.ID, "Primeiro", 2, 300)
	ultimo := seedTitle(t, d, lib.ID, "Ultimo", 1, 100)

	recentes, err := d.RecentTitles(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recentes) != 2 {
		t.Fatalf("recebi %d recentes, quero 2", len(recentes))
	}
	// Mesmo created_at: o desempate por id mantém o mais novo na frente.
	if recentes[0].ID != ultimo {
		t.Errorf("primeiro recente é %q, queria o último inserido", recentes[0].Name)
	}
	if recentes[1].Files != 2 {
		t.Errorf("agregado errado nos recentes: %d arquivos", recentes[1].Files)
	}

	user, err := d.CreateUser(ctx, "teste", "hash", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetFavorite(ctx, user.ID, primeiro, true); err != nil {
		t.Fatal(err)
	}

	favoritos, err := d.FavoriteTitles(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(favoritos) != 1 || favoritos[0].ID != primeiro {
		t.Fatalf("favoritos = %v", nomes(favoritos))
	}
	if favoritos[0].Files != 2 || favoritos[0].Duration != 600 {
		t.Errorf("agregado errado no favorito: %d arquivos, %.0fs", favoritos[0].Files, favoritos[0].Duration)
	}
}

func nomes(cards []TitleCard) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Name
	}
	return out
}
