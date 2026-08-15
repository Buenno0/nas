package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nas/internal/auth"
	"nas/internal/config"
	"nas/internal/db"
)

// prepara um servidor isolado: HOME temporário para o config.json e o cache
// nunca tocarem no ~/.nas de verdade.
func prepara(t *testing.T) (*Server, string, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	database, err := db.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrindo banco: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	srv := New(config.Default(), database, Options{})
	ctx := context.Background()

	if err := srv.Auth().CreateUser(ctx, "chefe", "senha-do-chefe", true); err != nil {
		t.Fatalf("criando admin: %v", err)
	}
	if err := srv.Auth().CreateUser(ctx, "visita", "senha-da-visita", false); err != nil {
		t.Fatalf("criando usuário: %v", err)
	}

	entrar := func(usuario, senha string) string {
		token, _, err := srv.Auth().Login(ctx, "127.0.0.1", usuario, senha, "teste")
		if err != nil {
			t.Fatalf("login de %s: %v", usuario, err)
		}
		return token
	}
	return srv, entrar("chefe", "senha-do-chefe"), entrar("visita", "senha-da-visita")
}

func chama(t *testing.T, srv *Server, metodo, caminho, token, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if corpo == "" {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(corpo)
	}
	req := httptest.NewRequest(metodo, caminho, body)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// O frontend esconde os botões, mas quem protege é a rota. Este teste bate
// direto na API, ignorando a interface.
func TestRotasDeAdmin(t *testing.T) {
	srv, admin, comum := prepara(t)

	casos := []struct {
		nome    string
		metodo  string
		caminho string
		corpo   string
	}{
		{"trocar configuração", http.MethodPut, "/api/settings", `{"tmdb_key":"roubada"}`},
		{"disparar scan", http.MethodPost, "/api/scan", ""},
		{"rebuscar metadados", http.MethodPost, "/api/metadata", ""},
	}

	for _, c := range casos {
		t.Run(c.nome+"/sem sessão", func(t *testing.T) {
			if rec := chama(t, srv, c.metodo, c.caminho, "", c.corpo); rec.Code != http.StatusUnauthorized {
				t.Errorf("status %d, quero 401", rec.Code)
			}
		})
		t.Run(c.nome+"/usuário comum", func(t *testing.T) {
			rec := chama(t, srv, c.metodo, c.caminho, comum, c.corpo)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status %d, quero 403 — usuário comum não pode %s", rec.Code, c.nome)
			}
			// 403 na resposta não basta: o efeito colateral também não pode
			// ter acontecido.
			if c.caminho == "/api/settings" {
				cfg, err := config.Load()
				if err != nil {
					t.Fatal(err)
				}
				if cfg.TMDBKey == "roubada" {
					t.Error("a chave do TMDB foi gravada apesar do 403")
				}
			}
		})
		t.Run(c.nome+"/admin", func(t *testing.T) {
			corpo := c.corpo
			if c.caminho == "/api/settings" {
				corpo = `{"tmdb_key":"legitima"}`
			}
			rec := chama(t, srv, c.metodo, c.caminho, admin, corpo)
			if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
				t.Errorf("admin foi barrado com %d", rec.Code)
			}
		})
	}

	// E o admin, esse sim, gravou.
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TMDBKey != "legitima" {
		t.Errorf("chave gravada = %q, quero a do admin", cfg.TMDBKey)
	}
}

func TestRotasDeTodos(t *testing.T) {
	srv, _, comum := prepara(t)

	// Ler o andamento, ver o acervo e corrigir capa continuam liberados.
	for _, caminho := range []string{"/api/home", "/api/libraries", "/api/settings", "/api/scan/status"} {
		if rec := chama(t, srv, http.MethodGet, caminho, comum, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s devolveu %d para usuário comum, quero 200", caminho, rec.Code)
		}
	}

	// Sem chave do TMDB a busca falha — mas com 502, não com 403.
	if rec := chama(t, srv, http.MethodGet, "/api/titles/1/matches", comum, ""); rec.Code == http.StatusForbidden {
		t.Error("corrigir capa deveria ser liberado para qualquer conta")
	}
}

// O caminho absoluto das pastas é do dono da máquina, não de quem assiste.
func TestCaminhoDaBibliotecaEscondido(t *testing.T) {
	srv, admin, comum := prepara(t)
	ctx := context.Background()

	if _, err := srv.db.AddLibrary(ctx, "Filmes", "/Users/alguem/Media/Filmes", db.KindMovie); err != nil {
		t.Fatal(err)
	}

	recAdmin := chama(t, srv, http.MethodGet, "/api/libraries", admin, "")
	if !strings.Contains(recAdmin.Body.String(), "/Users/alguem/Media/Filmes") {
		t.Error("admin deveria ver o caminho da biblioteca")
	}

	recComum := chama(t, srv, http.MethodGet, "/api/libraries", comum, "")
	if strings.Contains(recComum.Body.String(), "/Users/alguem") {
		t.Errorf("caminho vazou para usuário comum: %s", recComum.Body.String())
	}
	if !strings.Contains(recComum.Body.String(), "Filmes") {
		t.Error("usuário comum precisa ver o nome da biblioteca")
	}
}
