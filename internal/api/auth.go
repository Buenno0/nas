package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"nas/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Remember vem do "manter conectado": sessão de 30 dias com cookie
	// datado, contra 12 horas com cookie que morre ao fechar o navegador.
	Remember bool `json:"remember"`
	// TokenNaResposta faz o token da sessão voltar também no corpo, para o
	// cliente que não tem cookie jar e guarda a credencial ele mesmo — o app
	// nativo, no Keychain.
	//
	// É um pedido explícito e o SPA nunca o faz: deixar o token fora do
	// alcance do JavaScript é exatamente o que o HttpOnly compra, e ninguém
	// devolve isso de graça.
	TokenNaResposta bool `json:"token_na_resposta"`
}

type userResponse struct {
	Username           string `json:"username"`
	MustChangePassword bool   `json:"must_change_password"`
	IsAdmin            bool   `json:"is_admin"`
	// Preenchidos só quando o cliente pediu o token no corpo.
	Token    string `json:"token,omitempty"`
	ExpiraEm string `json:"expira_em,omitempty"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}

	ip := auth.ClientIP(r, s.opts.TrustProxy)
	token, user, ttl, err := s.auth.Login(r.Context(), ip, req.Username, req.Password, r.UserAgent(), req.Remember)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			status = http.StatusTooManyRequests
		}
		writeError(w, status, err.Error())
		return
	}

	cookie := &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}
	// Sem "manter conectado" o cookie não leva data: é de sessão, e o
	// navegador o descarta ao fechar. A sessão no banco ainda expira sozinha.
	if req.Remember {
		cookie.Expires = time.Now().Add(ttl)
	}
	http.SetCookie(w, cookie)

	resp := userResponse{Username: user.Username, MustChangePassword: user.MustChangePassword, IsAdmin: user.IsAdmin}
	if req.TokenNaResposta {
		resp.Token = token
		resp.ExpiraEm = time.Now().Add(ttl).UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Cookie ou Bearer: sair da conta precisa funcionar pelo mesmo caminho por
	// onde a conta entrou.
	if token := auth.TokenDaRequisicao(r); token != "" {
		if err := s.auth.Logout(r.Context(), token); err != nil {
			writeError(w, http.StatusInternalServerError, "não foi possível encerrar a sessão")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, userResponse{Username: user.Username, MustChangePassword: user.MustChangePassword, IsAdmin: user.IsAdmin})
}

type passwordRequest struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())

	var req passwordRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}
	if err := s.auth.ChangePassword(r.Context(), user.ID, req.Current, req.New); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Todas as sessões caíram, inclusive esta: o cliente precisa entrar de novo.
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// tokenDeMidiaResposta é a credencial curta que um player carrega na URL.
type tokenDeMidiaResposta struct {
	Token    string `json:"token"`
	ExpiraEm string `json:"expira_em"`
	// Segundos até vencer, para o cliente agendar a renovação sem depender de
	// o relógio dele bater com o do servidor.
	ValidoPor int `json:"valido_por"`
	// Param diz onde enfiar o token. Está aqui para o app não ter o nome do
	// parâmetro escrito à mão em quatro lugares.
	Param string `json:"param"`
}

// handleMediaToken entrega uma credencial de URL para as rotas de mídia.
//
// Existe porque o AVPlayer do iOS abre o vídeo num processo próprio, sem o
// cookie jar nem os cabeçalhos do app. Vale horas, morre com a sessão que a
// pediu e não abre nada da API — os detalhes estão em auth/midia.go.
func (s *Server) handleMediaToken(w http.ResponseWriter, r *http.Request) {
	sessao := auth.SessaoFrom(r.Context())
	if sessao == "" {
		// Só acontece se a requisição entrou por token de mídia, que não pode
		// gerar outro. O 401 é honesto: falta a credencial certa.
		writeError(w, http.StatusUnauthorized, "é preciso uma sessão para gerar um token de mídia")
		return
	}
	token, expira := s.auth.TokenDeMidia(sessao, auth.TokenDeMidiaTTL)
	writeJSON(w, http.StatusOK, tokenDeMidiaResposta{
		Token:     token,
		ExpiraEm:  expira.UTC().Format(time.RFC3339),
		ValidoPor: int(auth.TokenDeMidiaTTL.Seconds()),
		Param:     "t",
	})
}
