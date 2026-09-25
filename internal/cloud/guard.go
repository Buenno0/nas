package cloud

import (
	"fmt"
	"net/http"
)

// guarda é o RoundTripper que envolve todo cliente HTTP de nuvem.
//
// É defesa em profundidade: mesmo que algum trecho esqueça de checar o modo, a
// requisição não sai do Mac. Cada recusa soma em nuvem_bloqueadas_total.
type guarda struct {
	base  http.RoundTripper
	chave *Chave
}

func (g guarda) RoundTrip(r *http.Request) (*http.Response, error) {
	if g.chave.Modo() == ModoLocal {
		g.chave.bloqueadas.Add(1)
		if r.Body != nil {
			r.Body.Close()
		}
		return nil, fmt.Errorf("%s %s: %w", r.Method, r.URL.Host, ErrModoLocal)
	}
	return g.base.RoundTrip(r)
}
