package cloud

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nas/internal/config"
)

// Modo é o estado da chave. CONECTANDO só existe durante a transição: ou vira
// HIBRIDO, ou volta a LOCAL com erro. Nunca fica meio-conectado.
type Modo string

const (
	ModoLocal      Modo = "local"
	ModoConectando Modo = "conectando"
	ModoHibrido    Modo = "hibrido"
)

// Estado é o que a interface e o /api/modo enxergam.
type Estado struct {
	Modo        Modo      `json:"modo"`
	Erro        string    `json:"erro,omitempty"`
	Desde       time.Time `json:"desde"`
	Travado     bool      `json:"travado"`
	Configurada bool      `json:"configurada"`
	Suporte     bool      `json:"suporte"`
	Bloqueadas  int64     `json:"nuvem_bloqueadas_total"`
}

type estado struct {
	modo   Modo
	arm    Armazenamento
	ctx    context.Context
	cancel context.CancelFunc
	erro   string
	desde  time.Time
}

// Chave é o kill switch. A leitura do modo é um load atômico, então todo
// caminho quente pode consultá-la sem custo; desligar não pega lock nenhum, o
// que mantém o corte instantâneo mesmo no meio de uma conexão lenta.
type Chave struct {
	atual      atomic.Pointer[estado]
	transicao  sync.Mutex // serializa Ativar; Desligar nunca espera por ele
	bloqueadas atomic.Int64
	travado    bool
	cfg        func() config.Nuvem
	cliente    *http.Client

	ouvMu    sync.Mutex
	ouvintes map[chan Estado]struct{}
}

// Nova cria a chave em LOCAL. travado é o --sem-nuvem: Ativar sempre falha.
func Nova(cfg func() config.Nuvem, travado bool) *Chave {
	c := &Chave{cfg: cfg, travado: travado, ouvintes: map[chan Estado]struct{}{}}
	base := http.DefaultTransport.(*http.Transport).Clone()
	c.cliente = &http.Client{Transport: guarda{base: base, chave: c}}
	c.atual.Store(&estado{modo: ModoLocal, desde: time.Now()})
	return c
}

func (c *Chave) Modo() Modo { return c.atual.Load().modo }

// Cliente é o único cliente HTTP que código de nuvem deve usar.
func (c *Chave) Cliente() *http.Client { return c.cliente }

func (c *Chave) Estado() Estado {
	e := c.atual.Load()
	cfg := c.cfg()
	return Estado{
		Modo:        e.modo,
		Erro:        e.erro,
		Desde:       e.desde,
		Travado:     c.travado,
		Configurada: cfg.Configurada(),
		Suporte:     !semSuporteAtivo(),
		Bloqueadas:  c.bloqueadas.Load(),
	}
}

// Bloqueadas é o contador nuvem_bloqueadas_total.
func (c *Chave) Bloqueadas() int64 { return c.bloqueadas.Load() }

// Hibrido devolve o bucket e o contexto raiz do trabalho de nuvem. ok falso
// significa: não toque na nuvem.
func (c *Chave) Hibrido() (Armazenamento, context.Context, bool) {
	e := c.atual.Load()
	if e.modo != ModoHibrido || e.arm == nil {
		return nil, nil, false
	}
	return e.arm, e.ctx, true
}

// Vincular devolve um contexto que morre quando ctx morre OU quando o kill
// switch é acionado. É o que um ffmpeg lendo uma URL da nuvem deve usar.
func (c *Chave) Vincular(ctx context.Context) (Armazenamento, context.Context, context.CancelFunc, bool) {
	arm, raiz, ok := c.Hibrido()
	if !ok {
		return nil, nil, nil, false
	}
	filho, cancel := context.WithCancel(ctx)
	parar := context.AfterFunc(raiz, cancel)
	return arm, filho, func() { parar(); cancel() }, true
}

// Ativar faz LOCAL → CONECTANDO → HIBRIDO. Qualquer falha volta a LOCAL com
// o erro visível no Estado.
func (c *Chave) Ativar(ctx context.Context) error {
	if c.travado {
		return ErrTravado
	}
	c.transicao.Lock()
	defer c.transicao.Unlock()

	if c.Modo() == ModoHibrido {
		return nil
	}
	cfg := c.cfg()
	if !cfg.Configurada() {
		c.falha(ErrNaoConfig)
		return ErrNaoConfig
	}

	raiz, cancel := context.WithCancel(context.Background())
	conectando := &estado{modo: ModoConectando, ctx: raiz, cancel: cancel, desde: time.Now()}
	c.atual.Store(conectando)
	c.avisa()

	// A conexão obedece tanto a quem pediu quanto ao kill switch.
	tctx, tcancel := context.WithTimeout(raiz, 30*time.Second)
	defer tcancel()
	parar := context.AfterFunc(ctx, tcancel)
	defer parar()

	arm, err := conectorAtual()(tctx, cfg, c.cliente)
	if err == nil {
		err = arm.Verificar(tctx)
	}
	if err != nil {
		cancel()
		if c.atual.Load() != conectando {
			return ErrInterrompido
		}
		c.falha(fmt.Errorf("conectando à nuvem: %w", err))
		return err
	}

	hibrido := &estado{modo: ModoHibrido, arm: arm, ctx: raiz, cancel: cancel, desde: time.Now()}
	if !c.atual.CompareAndSwap(conectando, hibrido) {
		// O kill switch ganhou a corrida: o que foi conectado é descartado.
		cancel()
		return ErrInterrompido
	}
	c.avisa()
	return nil
}

// Desligar é o kill switch: troca o estado primeiro (a partir daqui o guard
// recusa tudo), depois cancela o contexto raiz, o que derruba long-polls,
// uploads e ffmpeg lendo URLs. Credenciais e clientes vão embora junto com o
// estado antigo.
func (c *Chave) Desligar() {
	novo := &estado{modo: ModoLocal, desde: time.Now()}
	velho := c.atual.Swap(novo)
	if velho.cancel != nil {
		velho.cancel()
	}
	if velho.modo != ModoLocal {
		c.avisa()
	}
}

func (c *Chave) falha(err error) {
	velho := c.atual.Swap(&estado{modo: ModoLocal, erro: err.Error(), desde: time.Now()})
	if velho.cancel != nil {
		velho.cancel()
	}
	c.avisa()
}

// Assinar entrega cada mudança de estado, para o SSE da interface.
func (c *Chave) Assinar() (<-chan Estado, func()) {
	ch := make(chan Estado, 4)
	c.ouvMu.Lock()
	c.ouvintes[ch] = struct{}{}
	c.ouvMu.Unlock()
	return ch, func() {
		c.ouvMu.Lock()
		delete(c.ouvintes, ch)
		c.ouvMu.Unlock()
	}
}

func (c *Chave) avisa() {
	e := c.Estado()
	c.ouvMu.Lock()
	defer c.ouvMu.Unlock()
	for ch := range c.ouvintes {
		select {
		case ch <- e:
		default: // ouvinte lento perde um estado intermediário, não o sistema
		}
	}
}

func semSuporteAtivo() bool {
	conectorMu.RLock()
	defer conectorMu.RUnlock()
	// Comparar funções não é permitido em Go; a flag registrada resolve.
	return !registrado
}
