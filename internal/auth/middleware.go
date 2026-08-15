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

const userCtxKey ctxKey = iota

// UserFrom recupera o usuário autenticado colocado pelo middleware.
func UserFrom(ctx context.Context) (db.User, bool) {
	u, ok := ctx.Value(userCtxKey).(db.User)
	return u, ok
}

// Require bloqueia quem não tem sessão válida. Responde 401 em JSON: o SPA
// intercepta e manda para a tela de login.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			unauthorized(w, r)
			return
		}
		u, err := s.UserFromToken(r.Context(), cookie.Value)
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
		web.ServeError(w, http.StatusUnauthorized)
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
