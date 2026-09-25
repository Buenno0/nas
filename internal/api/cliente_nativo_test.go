package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nas/internal/media"
)

// chamaBearer é o chama() do admin_test, mas mandando a sessão no cabeçalho —
// que é como um app nativo a manda.
func chamaBearer(t *testing.T, srv *Server, metodo, caminho, token, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// O SPA manda a sessão no cookie porque o navegador a esconde dele. Um app
// nativo guarda a sessão no Keychain e a manda no cabeçalho. As duas portas
// levam ao mesmo lugar.
func TestSessaoPeloCabecalho(t *testing.T) {
	srv, _, comum := prepara(t)

	for _, caminho := range []string{"/api/home", "/api/auth/me", "/api/libraries"} {
		if rec := chamaBearer(t, srv, http.MethodGet, caminho, comum, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s com Bearer devolveu %d, quero 200", caminho, rec.Code)
		}
	}

	// E um Bearer inventado continua sendo ninguém.
	if rec := chamaBearer(t, srv, http.MethodGet, "/api/home", "token-de-mentira", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("Bearer forjado devolveu %d, quero 401", rec.Code)
	}
	// Cabeçalho sem o esquema não é credencial.
	req := httptest.NewRequest(http.MethodGet, "/api/home", nil)
	req.Header.Set("Authorization", comum)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Authorization sem \"Bearer \" devolveu %d, quero 401", rec.Code)
	}
}

func TestHealthAnunciaRecursosDaTVSemQuebrarCamposAntigos(t *testing.T) {
	srv, _, _ := prepara(t)
	rec := chama(t, srv, http.MethodGet, "/healthz", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
	var health struct {
		Status     string   `json:"status"`
		Time       string   `json:"time"`
		APIVersion int      `json:"api_version"`
		Features   []string `json:"features"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health.Status != "ok" || health.Time == "" || health.APIVersion < 2 {
		t.Fatalf("health incompleto: %+v", health)
	}
	joined := strings.Join(health.Features, ",")
	if !strings.Contains(joined, "device_pairing") || !strings.Contains(joined, "playback_caps_v2") {
		t.Fatalf("features da TV ausentes: %v", health.Features)
	}
}

// O token da sessão só volta no corpo para quem pediu. O SPA não pede, e é
// isso que mantém o HttpOnly valendo alguma coisa.
func TestTokenNoCorpoSoQuandoPedido(t *testing.T) {
	srv, _, _ := prepara(t)

	ler := func(corpo string) userResponse {
		t.Helper()
		rec := chama(t, srv, http.MethodPost, "/api/auth/login", "", corpo)
		if rec.Code != http.StatusOK {
			t.Fatalf("login devolveu %d: %s", rec.Code, rec.Body.String())
		}
		var resp userResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("resposta ilegível: %v", err)
		}
		return resp
	}

	semPedir := ler(`{"username":"visita","password":"senha-da-visita"}`)
	if semPedir.Token != "" {
		t.Error("o token voltou no corpo sem ninguém pedir")
	}

	pedindo := ler(`{"username":"visita","password":"senha-da-visita","token_na_resposta":true}`)
	if pedindo.Token == "" {
		t.Fatal("o token não voltou no corpo para quem pediu")
	}
	if pedindo.ExpiraEm == "" {
		t.Error("token sem data de validade: o app não saberia quando renovar")
	}
	// E o token que voltou tem de abrir a API de verdade.
	if rec := chamaBearer(t, srv, http.MethodGet, "/api/auth/me", pedindo.Token, ""); rec.Code != http.StatusOK {
		t.Errorf("o token do corpo não abriu a API: %d", rec.Code)
	}
}

// tokenDeMidia pede a credencial de URL para uma sessão.
func tokenDeMidia(t *testing.T, srv *Server, sessao string) string {
	t.Helper()
	rec := chamaBearer(t, srv, http.MethodPost, "/api/auth/media-token", sessao, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("pedindo token de mídia: %d — %s", rec.Code, rec.Body.String())
	}
	var resp tokenDeMidiaResposta
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta ilegível: %v", err)
	}
	if resp.Param != "t" {
		t.Errorf("param = %q, quero t", resp.Param)
	}
	if resp.ValidoPor <= 0 {
		t.Errorf("valido_por = %d, quero um prazo positivo", resp.ValidoPor)
	}
	return resp.Token
}

// O AVPlayer abre a URL do vídeo num processo que não tem o cookie jar nem os
// cabeçalhos do app. O token de mídia é a única credencial que ele carrega.
//
// O arquivo 999 não existe: 404 quer dizer que a autenticação passou e o
// handler rodou, que é exatamente o que estes casos medem.
func TestTokenDeMidiaAbreAMidia(t *testing.T) {
	srv, _, comum := prepara(t)
	token := tokenDeMidia(t, srv, comum)

	rotas := []string{"/stream/999", "/preparado/999", "/img/file/999"}

	for _, rota := range rotas {
		t.Run(rota+"/com token", func(t *testing.T) {
			rec := chamaBearer(t, srv, http.MethodGet, rota+"?t="+token, "", "")
			if rec.Code == http.StatusUnauthorized {
				t.Error("token de mídia foi barrado na rota de mídia")
			}
		})
		t.Run(rota+"/sem nada", func(t *testing.T) {
			if rec := chamaBearer(t, srv, http.MethodGet, rota, "", ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("status %d sem credencial nenhuma, quero 401", rec.Code)
			}
		})
		t.Run(rota+"/token forjado", func(t *testing.T) {
			rec := chamaBearer(t, srv, http.MethodGet, rota+"?t=a.b.c", "", "")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status %d com token forjado, quero 401", rec.Code)
			}
		})
	}
}

// O que a credencial de URL NÃO abre. Ela vive à vista, num log de proxy ou
// num histórico; o dia em que vazar, o estrago tem de parar na mídia.
func TestTokenDeMidiaNaoAbreAAPI(t *testing.T) {
	srv, admin, comum := prepara(t)
	token := tokenDeMidia(t, srv, comum)
	doAdmin := tokenDeMidia(t, srv, admin)

	fechadas := []struct {
		metodo  string
		caminho string
	}{
		{http.MethodGet, "/api/home"},
		{http.MethodGet, "/api/auth/me"},
		{http.MethodGet, "/api/libraries"},
		{http.MethodGet, "/api/settings"},
		{http.MethodPost, "/api/auth/password"},
	}

	for _, c := range fechadas {
		rec := chamaBearer(t, srv, c.metodo, c.caminho+"?t="+token, "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s com token de mídia devolveu %d, quero 401", c.metodo, c.caminho, rec.Code)
		}
	}

	// Nem as rotas de admin, nem com o token do próprio admin.
	if rec := chamaBearer(t, srv, http.MethodPost, "/api/scan?t="+doAdmin, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/scan com token de mídia do admin devolveu %d, quero 401", rec.Code)
	}

	// E uma credencial curta não pode gerar outra: seria uma corrente infinita
	// de renovações a partir de uma URL vazada.
	if rec := chamaBearer(t, srv, http.MethodPost, "/api/auth/media-token?t="+token, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("token de mídia renovou a si mesmo: %d", rec.Code)
	}
}

// Sair da conta precisa valer também para o link que estava aberto no player.
func TestLogoutDerrubaOTokenDeMidia(t *testing.T) {
	srv, _, comum := prepara(t)
	token := tokenDeMidia(t, srv, comum)

	if rec := chamaBearer(t, srv, http.MethodGet, "/stream/999?t="+token, "", ""); rec.Code == http.StatusUnauthorized {
		t.Fatal("token recusado antes mesmo do logout")
	}
	if rec := chamaBearer(t, srv, http.MethodPost, "/api/auth/logout", comum, ""); rec.Code != http.StatusOK {
		t.Fatalf("logout por Bearer devolveu %d", rec.Code)
	}
	if rec := chamaBearer(t, srv, http.MethodGet, "/stream/999?t="+token, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("o token de mídia sobreviveu ao logout: %d", rec.Code)
	}
}

// Os dois dialetos de negociação, lidos da query. O SPA fala um; o app fala o
// outro; nenhum dos dois pode contaminar o outro.
func TestCapsDaQuery(t *testing.T) {
	caps := func(q string) media.Caps {
		return capsDaQuery(httptest.NewRequest(http.MethodGet, "/api/files/1/playback?"+q, nil))
	}

	t.Run("navegador", func(t *testing.T) {
		c := caps("can=hevc,av1")
		if !c.HEVC || !c.AV1 || c.VP9 {
			t.Errorf("dúvidas do navegador lidas errado: %+v", c)
		}
		if c.Video != nil || c.Audio != nil || c.Containers != nil {
			t.Error("o SPA não declara conjuntos; deixá-los nil é o que preserva os padrões")
		}
	})

	t.Run("app", func(t *testing.T) {
		c := caps("vid=h264,hevc&aud=aac,ac3&cont=mp4,mov,m4a&maxw=1920&maxh=1080")
		if !c.Video["h264"] || !c.Video["hevc"] || c.Video["vp9"] {
			t.Errorf("conjunto de vídeo lido errado: %v", c.Video)
		}
		if !c.Audio["aac"] || !c.Audio["ac3"] || c.Audio["opus"] {
			t.Errorf("conjunto de áudio lido errado: %v", c.Audio)
		}
		if !c.Containers["mp4"] || c.Containers["webm"] {
			t.Errorf("conjunto de containers lido errado: %v", c.Containers)
		}
		if c.MaxWidth != 1920 || c.MaxHeight != 1080 {
			t.Errorf("limite de tela lido errado: %dx%d", c.MaxWidth, c.MaxHeight)
		}
	})

	t.Run("lista hostil é cortada", func(t *testing.T) {
		c := caps("aud=" + strings.TrimSuffix(strings.Repeat("aac,", 500), ","))
		if len(c.Audio) > maxItensDeCaps {
			t.Errorf("entraram %d itens, o teto é %d", len(c.Audio), maxItensDeCaps)
		}
	})
}

// Os parâmetros que escolhem a receita têm de chegar inteiros ao /preparado.
// Perder um faria o player esperar por um preparo e pedir outro — e ficar
// olhando uma barra de progresso que nunca é a dele.
func TestURLPreparadoRepassaOPlano(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/api/files/7/playback?vid=h264,hevc&aud=aac&cont=mp4&audio=3&can=hevc&maxw=1920&maxh=1080&t=segredo&ruido=1", nil)
	u := urlPreparado(7, r)

	if !strings.HasPrefix(u, "/preparado/7?") {
		t.Fatalf("url = %q", u)
	}
	for _, quero := range []string{"vid=h264%2Chevc", "aud=aac", "cont=mp4", "audio=3", "can=hevc", "maxw=1920", "maxh=1080"} {
		if !strings.Contains(u, quero) {
			t.Errorf("faltou %s em %q", quero, u)
		}
	}
	// O token de mídia não escolhe receita nenhuma e não tem por que ser
	// copiado adiante — muito menos parar num cache.
	if strings.Contains(u, "segredo") {
		t.Errorf("o token de mídia vazou para a URL do preparado: %q", u)
	}
	if strings.Contains(u, "ruido") {
		t.Errorf("parâmetro alheio ao plano foi repassado: %q", u)
	}
}
