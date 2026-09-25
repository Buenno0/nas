package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConectarEhRotaDoAplicativo(t *testing.T) {
	if !IsSPARoute("/conectar") {
		t.Fatal("/conectar precisa abrir o fluxo de autorização, não a ruína 404")
	}
	req := httptest.NewRequest(http.MethodGet, "/conectar?codigo=ABCD-EFGH", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /conectar = %d, quero 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatal("/conectar não entregou o aplicativo React")
	}
}
