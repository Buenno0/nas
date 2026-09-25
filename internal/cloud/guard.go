package cloud

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
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

type discador func(ctx context.Context, rede, endereco string) (net.Conn, error)

// contaConexoes envolve o discador do transporte para saber quantas conexões
// estão abertas: soma ao conectar, subtrai ao fechar (uma vez só).
func contaConexoes(base discador, n *atomic.Int64) discador {
	if base == nil {
		base = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	return func(ctx context.Context, rede, endereco string) (net.Conn, error) {
		c, err := base(ctx, rede, endereco)
		if err != nil {
			return nil, err
		}
		n.Add(1)
		return &conexaoContada{Conn: c, n: n}, nil
	}
}

type conexaoContada struct {
	net.Conn
	n       *atomic.Int64
	fechada atomic.Bool
}

func (c *conexaoContada) Close() error {
	if c.fechada.CompareAndSwap(false, true) {
		c.n.Add(-1)
	}
	return c.Conn.Close()
}
