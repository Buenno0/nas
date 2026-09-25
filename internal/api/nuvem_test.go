package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"nas/internal/db"
	"nas/internal/scan"
)

// Um item só da nuvem, no modo local: existe no catálogo, não toca, e nada
// tenta falar com a AWS.
func TestItemDaNuvemNoModoLocal(t *testing.T) {
	srv, admin, comum := prepara(t)
	ctx := context.Background()

	lib, err := srv.db.AddLibrary(ctx, "Filmes", t.TempDir(), db.KindMovie)
	if err != nil {
		t.Fatal(err)
	}
	sc := scan.New(srv.db)
	id, err := sc.IndexarNuvem(ctx, lib, "bibliotecas/1/abc/Duna (2021).mkv", "Duna (2021).mkv", 1<<30, "")
	if err != nil {
		t.Fatal(err)
	}

	// O scan da pasta (vazia no disco) não pode apagar o que mora na nuvem.
	if _, err := sc.ScanLibrary(ctx, lib, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.FileByID(ctx, id); err != nil {
		t.Fatalf("o scan apagou o item da nuvem: %v", err)
	}

	rec := chama(t, srv, http.MethodGet, fmt.Sprintf("/api/files/%d/playback", id), comum, "")
	var pb respostaPlayback
	if err := json.NewDecoder(rec.Body).Decode(&pb); err != nil || !pb.Indisponivel {
		t.Fatalf("playback deveria dizer indisponível: %d %+v %v", rec.Code, pb, err)
	}
	if rec := chama(t, srv, http.MethodGet, fmt.Sprintf("/stream/%d", id), comum, ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("stream de item da nuvem no modo local = %d, quero 503", rec.Code)
	}

	rec = chama(t, srv, http.MethodGet, "/api/titles?library_id="+fmt.Sprint(lib.ID), comum, "")
	var cards []db.TitleCard
	if err := json.NewDecoder(rec.Body).Decode(&cards); err == nil && len(cards) > 0 && !cards[0].SoNaNuvem {
		t.Fatalf("card deveria vir marcado so_na_nuvem: %+v", cards[0])
	}

	// Alternar o modo é do admin; sem bucket configurado, o híbrido falha e
	// o modo continua local, com o erro visível.
	if rec := chama(t, srv, http.MethodPut, "/api/modo", comum, `{"modo":"hibrido"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("usuário comum alternou o modo: %d", rec.Code)
	}
	if rec := chama(t, srv, http.MethodPut, "/api/modo", admin, `{"modo":"hibrido"}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("híbrido sem configuração = %d, quero 502", rec.Code)
	}
	if srv.nuvem.Modo() != "local" {
		t.Fatalf("modo ficou %s", srv.nuvem.Modo())
	}
	if rec := chama(t, srv, http.MethodPost, "/api/uploads", admin,
		fmt.Sprintf(`{"library_id":%d,"nome":"x.mp4","tamanho":10}`, lib.ID)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("upload no modo local = %d, quero 503", rec.Code)
	}
}

func TestLocalSumidoComCopiaNaNuvemViraNuvem(t *testing.T) {
	srv, _, _ := prepara(t)
	ctx := context.Background()
	lib, _ := srv.db.AddLibrary(ctx, "Filmes", t.TempDir(), db.KindMovie)
	id, err := srv.db.UpsertFile(ctx, db.MediaFile{
		LibraryID: lib.ID, Path: "/nao/existe.mp4", RelPath: "existe.mp4", Ext: ".mp4",
		Size: 1, MTime: 1, Type: db.TypeVideo,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.db.MarcaNaNuvem(ctx, id, db.LocalAmbos, "k"); err != nil {
		t.Fatal(err)
	}
	if err := srv.db.DeleteFilesByID(ctx, []int64{id}); err != nil {
		t.Fatal(err)
	}
	loc, _, err := srv.db.Localizacao(ctx, id)
	if err != nil || loc != db.LocalNuvem {
		t.Fatalf("esperava nuvem, veio %q (%v)", loc, err)
	}
}

func TestTamanhoDaParteCabeNoLimite(t *testing.T) {
	for _, total := range []int64{1, 100 << 20, 200 << 30, 5 << 40} {
		p := TamanhoDaParte(total)
		if p < 5<<20 || (total+p-1)/p > 10000 {
			t.Errorf("total %d: parte %d dá %d partes", total, p, (total+p-1)/p)
		}
	}
	if _, err := relSeguro("../../etc/passwd"); err == nil {
		t.Error("relSeguro aceitou subir diretório")
	}
}
