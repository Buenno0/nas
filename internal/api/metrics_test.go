package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// O painel expõe caminhos de disco e desenho interno do servidor: é do admin.
func TestMetricsSoAdmin(t *testing.T) {
	srv, admin, comum := prepara(t)

	if rec := chama(t, srv, http.MethodGet, "/api/metrics/status", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("sem sessão: status %d, quero 401", rec.Code)
	}
	if rec := chama(t, srv, http.MethodGet, "/api/metrics/status", comum, ""); rec.Code != http.StatusForbidden {
		t.Errorf("usuário comum: status %d, quero 403", rec.Code)
	}
	if rec := chama(t, srv, http.MethodGet, "/api/metrics/status", admin, ""); rec.Code != http.StatusOK {
		t.Errorf("admin: status %d, quero 200", rec.Code)
	}
}

// O middleware tem de agregar pelo PADRÃO da rota, não pelo caminho resolvido:
// senão cada ID viraria uma linha nova no painel e a memória cresceria sem fim.
func TestMetricsAgregaPorPadraoDeRota(t *testing.T) {
	srv, admin, _ := prepara(t)

	for _, id := range []string{"1", "2", "3", "4"} {
		chama(t, srv, http.MethodGet, "/api/titles/"+id, admin, "")
	}

	rec := chama(t, srv, http.MethodGet, "/api/metrics/status", admin, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	var snap metricsSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}

	var achou bool
	for _, r := range snap.Trafego.Rotas {
		if r.Rota == "GET /api/titles/{id}" {
			achou = true
			if r.Total != 4 {
				t.Errorf("total = %d, quero 4 (os quatro IDs numa linha só)", r.Total)
			}
		}
		if r.Rota == "GET /api/titles/1" {
			t.Error("agregou pelo caminho resolvido: cardinalidade infinita")
		}
	}
	if !achou {
		t.Errorf("padrão GET /api/titles/{id} não apareceu; rotas vistas: %+v", snap.Trafego.Rotas)
	}
}

func TestMetricsSnapshotTemDisco(t *testing.T) {
	srv, admin, _ := prepara(t)
	rec := chama(t, srv, http.MethodGet, "/api/metrics/status", admin, "")

	var snap metricsSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.DiscoTotal <= 0 || snap.DiscoLivre <= 0 {
		t.Errorf("disco não foi lido: livre=%d total=%d", snap.DiscoLivre, snap.DiscoTotal)
	}
	if snap.Modo != "local" {
		t.Errorf("modo = %q, quero local (Options{} sem TrustProxy)", snap.Modo)
	}
	// Sem Serve não há startedAt: uptime tem de ser 0, não 56 anos.
	if snap.UptimeSegundos != 0 {
		t.Errorf("uptime = %d fora de Serve, quero 0", snap.UptimeSegundos)
	}
}
