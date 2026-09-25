package api

import (
	"encoding/json"
	"net/http"
	"testing"
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
