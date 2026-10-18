package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEstante(t *testing.T) {
	srv, _, comum := prepara(t)
	// A TV configurada não atende: nenhum teste aqui fala com a rede de verdade.
	desligada := httptest.NewServer(http.NotFoundHandler())
	desligada.Close()
	srv.cfg.Estante.TV = desligada.URL + "/dmr.xml"

	if rec := chama(t, srv, http.MethodPost, "/api/estante/tv/volume", "", `{"valor":10}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sem sessão: %d", rec.Code)
	}
	for _, corpo := range []string{`{}`, `{"valor":1,"passo":1}`, `nada`} {
		if rec := chama(t, srv, http.MethodPost, "/api/estante/tv/volume", comum, corpo); rec.Code != http.StatusBadRequest {
			t.Fatalf("corpo %s: %d", corpo, rec.Code)
		}
	}
	if rec := chama(t, srv, http.MethodPost, "/api/estante/tv/mudo", comum, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("mudo sem campo: %d", rec.Code)
	}
	// Conta comum pode mexer no volume; com a TV desligada a resposta é 409.
	if rec := chama(t, srv, http.MethodPost, "/api/estante/tv/volume", comum, `{"passo":1}`); rec.Code != http.StatusConflict {
		t.Fatalf("TV desligada: %d %s", rec.Code, rec.Body)
	}
}

func TestEstanteNaNuvem(t *testing.T) {
	srv, _, comum := prepara(t)
	srv.opts.NaNuvem = true

	rec := chama(t, srv, http.MethodGet, "/api/estante", comum, "")
	var e estadoEstante
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Disponivel {
		t.Fatalf("na nuvem a Estante devia vir indisponível: %d %s", rec.Code, rec.Body)
	}
	if rec := chama(t, srv, http.MethodPost, "/api/estante/tv/volume", comum, `{"passo":1}`); rec.Code != http.StatusConflict {
		t.Fatalf("na nuvem o comando devia ser recusado: %d", rec.Code)
	}
}
