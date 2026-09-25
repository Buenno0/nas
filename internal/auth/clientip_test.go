package auth

import (
	"net/http/httptest"
	"testing"
)

// Um cliente pode mandar qualquer cabeçalho. Só o que o proxy do loopback
// escreveu vale; do contrário, trocar o "IP" a cada tentativa furaria o
// limite de senhas.
func TestClientIPNaoAceitaCabecalhoForjado(t *testing.T) {
	casos := []struct {
		nome   string
		remoto string
		proxy  Proxy
		cab    map[string]string
		quero  string
	}{
		{"direto ignora cabeçalhos", "203.0.113.9:5000", Tailscale, map[string]string{"X-Forwarded-For": "1.1.1.1"}, "203.0.113.9"},
		{"sem proxy ignora", "127.0.0.1:5000", SemProxy, map[string]string{"CF-Connecting-IP": "1.1.1.1"}, "127.0.0.1"},
		{"tunnel usa a Cloudflare", "127.0.0.1:5000", Cloudflare, map[string]string{"CF-Connecting-IP": "198.51.100.7"}, "198.51.100.7"},
		{"tailscale ignora CF forjado", "127.0.0.1:5000", Tailscale, map[string]string{"CF-Connecting-IP": "1.1.1.1", "X-Forwarded-For": "198.51.100.7"}, "198.51.100.7"},
		{"tailscale usa o último do XFF", "127.0.0.1:5000", Tailscale, map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.7"}, "198.51.100.7"},
		{"tailscale sem XFF cai no loopback", "127.0.0.1:5000", Tailscale, nil, "127.0.0.1"},
	}
	for _, c := range casos {
		r := httptest.NewRequest("POST", "/api/auth/login", nil)
		r.RemoteAddr = c.remoto
		for k, v := range c.cab {
			r.Header.Set(k, v)
		}
		if got := ClientIP(r, c.proxy); got != c.quero {
			t.Errorf("%s: %q, quero %q", c.nome, got, c.quero)
		}
	}
}
