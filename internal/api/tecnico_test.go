package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"nas/internal/db"
)

// No modo local a tela técnica responde só com o que é do Mac, sem uma
// chamada sequer à nuvem (o contador de bloqueadas não se mexe).
func TestTecnicoNoModoLocal(t *testing.T) {
	srv, admin, comum := prepara(t)

	if rec := chama(t, srv, http.MethodGet, "/api/tecnico", comum, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("usuário comum viu a tela técnica: %d", rec.Code)
	}
	rec := chama(t, srv, http.MethodGet, "/api/tecnico?atualizar=1", admin, "")
	var resp struct {
		Pulso pulso   `json:"pulso"`
		Nuvem daNuvem `json:"nuvem"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %v", rec.Code, err)
	}
	if !resp.Nuvem.Indisponivel || resp.Pulso.Modo != "local" || resp.Pulso.Conexoes != 0 {
		t.Fatalf("resposta no modo local: %+v", resp)
	}
	if srv.nuvem.Bloqueadas() != 0 {
		t.Fatalf("a tela técnica tentou falar com a nuvem: %d bloqueadas", srv.nuvem.Bloqueadas())
	}

	srv.db.Anota(t.Context(), "envio.inicio", 3, 0, "a.mkv", nil)
	rec = chama(t, srv, http.MethodGet, "/api/tecnico/diario?tipo=envio.", admin, "")
	var notas []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&notas); err != nil || len(notas) != 1 {
		t.Fatalf("diário: %v %v", err, notas)
	}

	rec = chama(t, srv, http.MethodGet, "/api/tecnico/custo?atualizar=1", admin, "")
	var custo respostaCusto
	if err := json.NewDecoder(rec.Body).Decode(&custo); err != nil || !custo.Indisponivel {
		t.Fatalf("custo no modo local: %v %+v", err, custo)
	}
	if srv.nuvem.Bloqueadas() != 0 {
		t.Fatal("o painel de custo tentou falar com a AWS no modo local")
	}
}

func TestArmazenamentoSomaPorLocalizacao(t *testing.T) {
	srv, admin, comum := prepara(t)
	ctx := t.Context()
	lib, _ := srv.db.AddLibrary(ctx, "Filmes", t.TempDir(), db.KindMovie)
	a, _ := srv.db.UpsertFile(ctx, db.MediaFile{LibraryID: lib.ID, Path: "/x/a.mkv", RelPath: "a.mkv", Ext: ".mkv", Size: 100, MTime: 1, Type: db.TypeVideo}, true)
	b, _ := srv.db.UpsertFile(ctx, db.MediaFile{LibraryID: lib.ID, Path: "/x/b.mkv", RelPath: "b.mkv", Ext: ".mkv", Size: 50, MTime: 1, Type: db.TypeVideo}, true)
	srv.db.MarcaNaNuvem(ctx, b, db.LocalAmbos, "bibliotecas/1/b/b.mkv")
	_ = a

	if rec := chama(t, srv, http.MethodGet, "/api/armazenamento", comum, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("usuário comum viu o armazenamento: %d", rec.Code)
	}
	rec := chama(t, srv, http.MethodGet, "/api/armazenamento", admin, "")
	var resp struct {
		PorBiblioteca []db.UsoDaBiblioteca `json:"por_biblioteca"`
		Disco         map[string]int64     `json:"disco"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil || len(resp.PorBiblioteca) != 1 {
		t.Fatalf("%v %+v", err, resp)
	}
	if u := resp.PorBiblioteca[0]; u.Bytes != 150 || u.BytesNuvem != 50 {
		t.Fatalf("uso da biblioteca: %+v", u)
	}
	if resp.Disco["total"] == 0 {
		t.Fatal("sem dados do disco")
	}
}
