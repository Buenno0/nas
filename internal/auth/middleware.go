package auth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"nas/internal/db"
	"nas/internal/web"
)

type ctxKey int

const (
	userCtxKey ctxKey = iota
	sessaoCtxKey
)

// UserFrom recupera o usuário autenticado colocado pelo middleware.
func UserFrom(ctx context.Context) (db.User, bool) {
	u, ok := ctx.Value(userCtxKey).(db.User)
	return u, ok
}

// SessaoFrom devolve o token de sessão em claro que autenticou a requisição.
//
// Vem vazio quando a requisição entrou por token de mídia — de propósito: uma
// credencial de URL não pode gerar outra, senão a validade curta viraria uma
// corrente infinita de renovações.
func SessaoFrom(ctx context.Context) string {
	t, _ := ctx.Value(sessaoCtxKey).(string)
	return t
}

// TokenDaRequisicao lê a sessão de onde o cliente souber mandá-la.
//
// O SPA nunca vê o token dele: fica no cookie HttpOnly, que é justamente o que
// impede um XSS de roubá-lo. Um app nativo não tem esse problema nem esse
// luxo — guarda o token no Keychain e o manda no cabeçalho, como manda em
// qualquer API.
func TokenDaRequisicao(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
	}
	if cookie, err := r.Cookie(CookieName); err == nil {
		return cookie.Value
	}
	return ""
}

// Require bloqueia quem não tem sessão válida. Responde 401 em JSON: o SPA
// intercepta e manda para a tela de login.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := TokenDaRequisicao(r)
		u, err := s.UserFromToken(r.Context(), token)
		if err != nil {
			unauthorized(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, u)
		ctx = context.WithValue(ctx, sessaoCtxKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireMidia protege o que um player abre por URL: o vídeo cru, o preparado,
// as capas e as legendas.
//
// Aceita a sessão normal — é por onde o SPA passa, com o cookie que o
// navegador anexa sozinho — ou um token de mídia em ?t=, que é a única
// credencial que o AVPlayer do iOS consegue carregar sem ajuda. A ordem
// importa pouco, mas o token vem primeiro: quem o mandou explicitamente quis
// usá-lo, e cair no cookie escondendo um token vencido daria um bug difícil.
func (s *Service) RequireMidia(next http.Handler) http.Handler {
	comSessao := s.Require(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("t")
		if token == "" {
			comSessao.ServeHTTP(w, r)
			return
		}
		u, err := s.UserFromMediaToken(r.Context(), token)
		if err != nil {
			unauthorized(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userCtxKey, u)))
	})
}

// RequireAdmin protege o que mexe no servidor inteiro: bibliotecas, scan,
// chave do TMDB. Esconder o botão no frontend não protege nada — é aqui que a
// recusa acontece.
func (s *Service) RequireAdmin(next http.Handler) http.Handler {
	return s.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFrom(r.Context())
		if !ok || !user.IsAdmin {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "só o administrador pode fazer isso",
			})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func unauthorized(w http.ResponseWriter, r *http.Request) {
	// Um link de /stream aberto direto no navegador merece a tela de 401, não
	// um JSON cru. O SPA, que pede com Accept: */*, continua recebendo JSON.
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		web.ServeError(w, r, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "não autenticado"})
}

// ClientIP resolve o IP para o rate limit. Atrás do tunnel, toda requisição
// chega do loopback (o cloudflared roda na própria máquina), então nesse caso
// — e só nesse — o cabeçalho da Cloudflare é a fonte confiável.
func ClientIP(r *http.Request, trustProxy bool) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if trustProxy && isLoopback(host) {
		if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
			return cf
		}
	}
	return host
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
