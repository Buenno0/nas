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

// Proxy diz quem está no loopback na frente do servidor, e portanto em que
// cabeçalho confiar para o IP do cliente (rate limit do login).
type Proxy int

const (
	SemProxy   Proxy = iota // conexão direta: vale o RemoteAddr
	Cloudflare              // tunnel: o cloudflared põe CF-Connecting-IP
	Tailscale               // instância cloud: o tailscale serve (e o Funnel)
)

// ClientIP resolve o IP para o rate limit. Só um proxy no loopback é
// confiável, e só o que ELE escreve: o cliente pode mandar qualquer
// cabeçalho. A Cloudflare sobrescreve o CF-Connecting-IP; o tailscale serve
// acrescenta o IP real no FIM do X-Forwarded-For, então é o último item que
// vale — o primeiro pode ter vindo do próprio atacante.
func ClientIP(r *http.Request, proxy Proxy) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !isLoopback(host) {
		return host
	}
	switch proxy {
	case Cloudflare:
		if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
			return cf
		}
	case Tailscale:
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			itens := strings.Split(xff[len(xff)-1], ",")
			if ip := strings.TrimSpace(itens[len(itens)-1]); ip != "" {
				return ip
			}
		}
	}
	return host
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
