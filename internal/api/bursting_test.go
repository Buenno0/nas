package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/energia"
)

// filaFalsa é um bucket que só registra as mensagens enviadas.
type filaFalsa struct {
	cloud.Armazenamento
	cloud.Mensageria
	mu   sync.Mutex
	jobs []string
}

func (f *filaFalsa) Verificar(context.Context) error { return nil }
func (f *filaFalsa) EnviarMensagem(_ context.Context, _ string, corpo []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = append(f.jobs, string(corpo))
	return nil
}

func preparaHibrido(t *testing.T, e energia.Estado) (*Server, string, *filaFalsa, db.MediaFile) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	database, err := db.OpenAt(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	fila := &filaFalsa{}
	cloud.Registrar(func(context.Context, config.Nuvem, *http.Client) (cloud.Armazenamento, error) {
		return fila, nil
	})
	cfg := config.Default()
	cfg.Nuvem = config.Nuvem{Bucket: "b", Regiao: "r", FilaJobs: "jobs"}
	var srv *Server
	chave := cloud.Nova(func() config.Nuvem { return srv.ConfigNuvem() }, false)
	srv = New(cfg, database, Options{Nuvem: chave, Energia: energia.Fixo(e)})
	ctx := context.Background()
	if err := chave.Ativar(ctx); err != nil {
		t.Fatal(err)
	}
	srv.Auth().CreateUser(ctx, "chefe", "senha-do-chefe", true)
	token, _, _, _ := srv.Auth().Login(ctx, "127.0.0.1", "chefe", "senha-do-chefe", "t", true)

	lib, _ := database.AddLibrary(ctx, "Filmes", t.TempDir(), db.KindMovie)
	// MKV com H.264: o navegador não abre o container, precisa de remux.
	id, _ := database.UpsertFile(ctx, db.MediaFile{LibraryID: lib.ID, Path: "/x/Filme.mkv", RelPath: "Filme.mkv",
		Ext: ".mkv", Size: 1, MTime: 1, Type: db.TypeVideo, VCodec: "h264", ACodec: "aac",
		PixFmt: "yuv420p", VProfile: "High", Duration: 60}, true)
	database.MarcaNaNuvem(ctx, id, db.LocalAmbos, "bibliotecas/1/a/Filme.mkv")
	f, _ := database.FileByID(ctx, id)
	return srv, token, fila, f
}

func playback(t *testing.T, srv *Server, token string, id int64) respostaPlayback {
	t.Helper()
	rec := chama(t, srv, http.MethodGet, fmt.Sprintf("/api/files/%d/playback", id), token, "")
	var pb respostaPlayback
	if err := json.NewDecoder(rec.Body).Decode(&pb); err != nil {
		t.Fatal(err)
	}
	return pb
}

// Na bateria, um arquivo que também está no bucket é preparado pelo worker.
func TestBurstingNaBateria(t *testing.T) {
	srv, token, fila, f := preparaHibrido(t, energia.Estado{NaBateria: true, Motivo: "na bateria"})

	pb := playback(t, srv, token, f.ID)
	if pb.Preparo == nil || pb.Preparo.Receita != "nuvem" {
		t.Fatalf("na bateria deveria preparar na nuvem: %+v", pb)
	}
	if rec := chama(t, srv, http.MethodPost, fmt.Sprintf("/api/files/%d/prepare", f.ID), token, ""); rec.Code != http.StatusAccepted {
		t.Fatalf("prepare = %d %s", rec.Code, rec.Body)
	}
	if len(fila.jobs) != 1 {
		t.Fatalf("esperava 1 job na fila, veio %d", len(fila.jobs))
	}
	// Com o derivado pronto, o player toca o MP4 do worker.
	srv.db.GravaDerivados(context.Background(), f.ID, []db.Derivado{{Tipo: "compat", Key: "derivados/x/compat.mp4"}})
	if pb := playback(t, srv, token, f.ID); pb.URL != fmt.Sprintf("/stream/%d?derivado=compat", f.ID) {
		t.Fatalf("URL = %q", pb.URL)
	}
}

// Na tomada e frio, o Mac prepara sozinho: nada vai para a fila.
func TestSemBurstingNaTomada(t *testing.T) {
	srv, token, fila, f := preparaHibrido(t, energia.Estado{})
	pb := playback(t, srv, token, f.ID)
	if pb.Preparo != nil && pb.Preparo.Receita == "nuvem" {
		t.Fatalf("na tomada não deveria mandar para a nuvem: %+v", pb.Preparo)
	}
	if len(fila.jobs) != 0 {
		t.Fatal("job enviado com o Mac na tomada")
	}
}
