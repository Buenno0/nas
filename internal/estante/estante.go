package estante

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Alvos é onde procurar cada aparelho. Vazio = procurar na rede.
type Alvos struct {
	PS5 string // IP
	TV  string // URL da descrição UPnP
}

// Estado é o que a página mostra. Nil é "não respondeu": o PS5 desligado (ou
// em descanso sem atender a rede) e a TV desligada somem do mesmo jeito.
type Estado struct {
	PS5 *PS5Visto `json:"ps5"`
	TV  *TVVista  `json:"tv"`
	Em  time.Time `json:"em"`
}

type PS5Visto struct {
	Nome     string    `json:"nome"`
	Endereco string    `json:"endereco"`
	Estado   EstadoPS5 `json:"estado"`
	Jogo     string    `json:"jogo,omitempty"`
}

type TVVista struct {
	Nome   string `json:"nome"`
	Volume int    `json:"volume"`
	Mudo   bool   `json:"mudo"`
	Max    int    `json:"max"`
}

// VolumeMax é o teto do RenderingControl da TV da sala (0–100, passo 1).
const VolumeMax = 100

// ErrTVDesligada: nenhum renderizador respondeu, então não há o que comandar.
var ErrTVDesligada = errors.New("a TV não está respondendo; ela só aparece na rede ligada")

// Central sonda os aparelhos e avisa quem está olhando. Ela só fala com a rede
// enquanto há ouvintes (a página aberta) ou quando alguém pede o estado: sem
// ninguém olhando, nenhum pacote sai.
type Central struct {
	alvos     func() Alvos
	espera    time.Duration // quanto esperar por respostas de DDP e SSDP
	intervalo time.Duration // entre sondagens, com ouvintes

	// procurarPS5 e procurarTVs são trocados nos testes, para não mandar
	// broadcast na rede de quem roda go test.
	procurarPS5 func(ctx context.Context, host string, espera time.Duration) ([]Console, error)
	procurarTVs func(ctx context.Context, espera time.Duration) ([]*TV, error)

	sondaMu sync.Mutex // uma sondagem por vez; quem chega junto espera e reaproveita

	mu     sync.Mutex
	estado Estado
	tv     *TV
	// ultimaTV é a descrição da última TV achada por SSDP. Ela fica mesmo
	// quando a TV some: na volta, um GET direto acha a TV sem depender do
	// multicast, que o Wi-Fi perde.
	ultimaTV string
	ouvintes map[chan Estado]struct{}
	chegou   chan struct{}
}

func NovaCentral(alvos func() Alvos) *Central {
	return &Central{
		alvos:       alvos,
		espera:      1500 * time.Millisecond,
		intervalo:   5 * time.Second,
		procurarPS5: Descobrir,
		procurarTVs: ProcurarTVs,
		ouvintes:    map[chan Estado]struct{}{},
		chegou:      make(chan struct{}, 1),
	}
}

// Estado devolve a última sondagem se ela for recente; senão sonda agora.
func (c *Central) Estado(ctx context.Context) Estado {
	c.mu.Lock()
	e := c.estado
	c.mu.Unlock()
	if !e.Em.IsZero() && time.Since(e.Em) < c.intervalo {
		return e
	}
	return c.Sondar(ctx)
}

// Sondar pergunta aos dois aparelhos, em paralelo, e publica o resultado.
func (c *Central) Sondar(ctx context.Context) Estado {
	pedido := time.Now()
	c.sondaMu.Lock()
	defer c.sondaMu.Unlock()
	// Quem esperou a sondagem de outro aproveita o resultado dela.
	c.mu.Lock()
	if e := c.estado; e.Em.After(pedido) {
		c.mu.Unlock()
		return e
	}
	c.mu.Unlock()

	alvos := c.alvos()
	var e Estado
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		e.PS5 = c.sondarPS5(ctx, alvos.PS5)
	}()
	go func() {
		defer wg.Done()
		e.TV = c.sondarTV(ctx, alvos.TV)
	}()
	wg.Wait()
	e.Em = time.Now()
	c.publicar(e)
	return e
}

func (c *Central) sondarPS5(ctx context.Context, host string) *PS5Visto {
	cs, _ := c.procurarPS5(ctx, host, c.espera)
	var achado *Console
	for i := range cs {
		if cs[i].Tipo != "PS5" {
			continue
		}
		if host == "" || cs[i].Endereco == host {
			achado = &cs[i]
			break
		}
	}
	if achado == nil {
		return nil
	}
	return &PS5Visto{Nome: achado.Nome, Endereco: achado.Endereco, Estado: achado.Estado, Jogo: achado.Jogo}
}

func (c *Central) sondarTV(ctx context.Context, descricao string) *TVVista {
	tv, err := c.tvAtual(ctx, descricao)
	if err != nil {
		return nil
	}
	v, err := tv.Volume(ctx)
	if err != nil {
		c.esquecerTV(tv)
		return nil
	}
	mudo, _ := tv.Mudo(ctx)
	return &TVVista{Nome: tv.Nome, Volume: v, Mudo: mudo, Max: VolumeMax}
}

// tvAtual reaproveita a TV já aberta; se não houver, abre pela descrição
// configurada ou procura por SSDP, preferindo uma Sony.
func (c *Central) tvAtual(ctx context.Context, descricao string) (*TV, error) {
	c.mu.Lock()
	tv := c.tv
	c.mu.Unlock()
	if tv != nil && (descricao == "" || tv.Descricao == descricao) {
		return tv, nil
	}

	cli := &httpCurto
	if descricao != "" {
		t, err := AbrirTV(ctx, descricao, cli)
		if err != nil {
			return nil, ErrTVDesligada
		}
		tv = t
	} else {
		c.mu.Lock()
		ultima := c.ultimaTV
		c.mu.Unlock()
		if ultima != "" {
			tv, _ = AbrirTV(ctx, ultima, cli)
		}
		if tv == nil {
			tvs, _ := c.procurarTVs(ctx, c.espera)
			for _, t := range tvs {
				if tv == nil || (strings.Contains(t.Fabricante, "Sony") && !strings.Contains(tv.Fabricante, "Sony")) {
					tv = t
				}
			}
		}
		if tv == nil {
			return nil, ErrTVDesligada
		}
		tv.HTTP = cli
	}
	c.mu.Lock()
	c.tv = tv
	if descricao == "" {
		c.ultimaTV = tv.Descricao
	}
	c.mu.Unlock()
	return tv, nil
}

// esquecerTV descarta a TV que parou de responder (desligou ou trocou de IP):
// a próxima sondagem abre de novo.
func (c *Central) esquecerTV(tv *TV) {
	c.mu.Lock()
	if c.tv == tv {
		c.tv = nil
	}
	c.mu.Unlock()
}

// DefinirVolume põe o volume em v (limitado a 0–VolumeMax).
func (c *Central) DefinirVolume(ctx context.Context, v int) (Estado, error) {
	return c.mexerNaTV(ctx, func(tv *TV, atual *TVVista) error {
		v = max(0, min(VolumeMax, v))
		if err := tv.DefinirVolume(ctx, v); err != nil {
			return err
		}
		atual.Volume = v
		return nil
	})
}

// Passo soma d ao volume que a TV tem agora — não ao último visto, que pode
// ter mudado pelo controle remoto.
func (c *Central) Passo(ctx context.Context, d int) (Estado, error) {
	return c.mexerNaTV(ctx, func(tv *TV, atual *TVVista) error {
		v, err := tv.Volume(ctx)
		if err != nil {
			return err
		}
		v = max(0, min(VolumeMax, v+d))
		if err := tv.DefinirVolume(ctx, v); err != nil {
			return err
		}
		atual.Volume = v
		return nil
	})
}

func (c *Central) DefinirMudo(ctx context.Context, mudo bool) (Estado, error) {
	return c.mexerNaTV(ctx, func(tv *TV, atual *TVVista) error {
		if err := tv.DefinirMudo(ctx, mudo); err != nil {
			return err
		}
		atual.Mudo = mudo
		return nil
	})
}

// mexerNaTV aplica um comando e publica o estado novo na hora, sem esperar a
// próxima sondagem: quem arrasta o volume no celular vê o número andar.
func (c *Central) mexerNaTV(ctx context.Context, f func(*TV, *TVVista) error) (Estado, error) {
	c.mu.Lock()
	e := c.estado
	c.mu.Unlock()
	// No erro volta o último estado visto, sem sondar de novo: a resposta
	// de um comando que falhou tem que ser rápida.
	tv, err := c.tvAtual(ctx, c.alvos().TV)
	if err != nil {
		return e, err
	}
	var atual TVVista
	if e.TV != nil {
		atual = *e.TV
	} else {
		atual = TVVista{Nome: tv.Nome, Max: VolumeMax}
		atual.Mudo, _ = tv.Mudo(ctx)
	}
	if err := f(tv, &atual); err != nil {
		c.esquecerTV(tv)
		return e, err
	}
	e.TV = &atual
	if e.Em.IsZero() {
		e.Em = time.Now()
	}
	c.publicar(e)
	return e, nil
}

// Assinar entrega cada estado novo. O primeiro ouvinte acorda o laço de Rodar.
func (c *Central) Assinar() (<-chan Estado, func()) {
	ch := make(chan Estado, 4)
	c.mu.Lock()
	c.ouvintes[ch] = struct{}{}
	c.mu.Unlock()
	select {
	case c.chegou <- struct{}{}:
	default:
	}
	return ch, func() {
		c.mu.Lock()
		delete(c.ouvintes, ch)
		c.mu.Unlock()
	}
}

func (c *Central) temOuvintes() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.ouvintes) > 0
}

// publicar guarda o estado e avisa os ouvintes se algo além do horário mudou.
func (c *Central) publicar(e Estado) {
	c.mu.Lock()
	defer c.mu.Unlock()
	mudou := !reflect.DeepEqual(c.estado.PS5, e.PS5) || !reflect.DeepEqual(c.estado.TV, e.TV)
	c.estado = e
	if !mudou {
		return
	}
	for ch := range c.ouvintes {
		select {
		case ch <- e:
		default: // ouvinte lento perde um estado intermediário
		}
	}
}

// Rodar sonda a cada intervalo enquanto houver ouvintes, e dorme sem eles.
func (c *Central) Rodar(ctx context.Context) {
	for {
		if !c.temOuvintes() {
			select {
			case <-ctx.Done():
				return
			case <-c.chegou:
				continue
			}
		}
		c.Sondar(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.intervalo):
		}
	}
}
