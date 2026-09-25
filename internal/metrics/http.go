package metrics

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Limites dos buckets de latência. Contar por faixa, em vez de guardar cada
// amostra, mantém a memória em O(1) por rota — um servidor ligado há semanas
// ocupa o mesmo tanto do primeiro dia.
//
// A escala é apertada embaixo porque é lá que este servidor vive: SQLite local
// responde em frações de milissegundo. Uma primeira faixa larga (0–10ms, por
// exemplo) faria o percentil interpolado devolver "5ms" para uma rota cujo pior
// caso real foi 1ms — número inventado, pior que faixa nenhuma.
var limites = []time.Duration{
	time.Millisecond,
	2500 * time.Microsecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	5 * time.Second,
}

const nBuckets = 12 // len(limites) + 1; o último é "≥ 5s"

// maxRotas trava a cardinalidade. Na prática as chaves são os padrões
// registrados no mux, que são finitos — isto é só seguro contra um caminho
// futuro que gere chave dinâmica sem ninguém perceber.
const maxRotas = 200

// RotaStats acumula o que se sabe de uma rota desde o boot.
type RotaStats struct {
	Total    int64
	Erros4xx int64
	Erros5xx int64
	SomaNs   int64
	MaxNs    int64
	Buckets  [nBuckets]int64
}

// Coletor guarda as estatísticas por rota. Seguro para uso concorrente.
type Coletor struct {
	mu          sync.Mutex
	porRota     map[string]*RotaStats
	descartadas int64 // observações perdidas pelo teto de cardinalidade
}

func NovoColetor() *Coletor {
	return &Coletor{porRota: make(map[string]*RotaStats, 32)}
}

// Observa registra uma requisição concluída.
//
// streaming existe porque uma resposta de vídeo de duas horas entraria na conta
// como "latência de 7200s" e destruiria qualquer percentil útil — o mesmo vale
// para SSE, que fica aberto de propósito. Essas respostas são contadas em
// nenhum lugar: aparecer com número errado é pior que não aparecer.
func (c *Coletor) Observa(rota string, status int, duracao time.Duration, streaming bool) {
	if rota == "" || streaming {
		return
	}
	if duracao < 0 {
		duracao = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	st := c.porRota[rota]
	if st == nil {
		if len(c.porRota) >= maxRotas {
			c.descartadas++
			return
		}
		st = &RotaStats{}
		c.porRota[rota] = st
	}

	st.Total++
	st.SomaNs += int64(duracao)
	if int64(duracao) > st.MaxNs {
		st.MaxNs = int64(duracao)
	}
	st.Buckets[indiceBucket(duracao)]++

	switch {
	case status >= 500:
		st.Erros5xx++
	case status >= 400:
		st.Erros4xx++
	}
}

// indiceBucket devolve a faixa de uma duração. A borda pertence à faixa de
// cima: exatamente 10ms conta como "10–50ms", não como "<10ms".
func indiceBucket(d time.Duration) int {
	for i, l := range limites {
		if d < l {
			return i
		}
	}
	return len(limites)
}

// RotaSnapshot é a versão pronta para JSON, em milissegundos.
type RotaSnapshot struct {
	Rota     string  `json:"rota"`
	Total    int64   `json:"total"`
	Erros4xx int64   `json:"erros_4xx"`
	Erros5xx int64   `json:"erros_5xx"`
	TaxaErro float64 `json:"taxa_erro"` // 0–1
	MediaMs  float64 `json:"media_ms"`
	MaxMs    float64 `json:"max_ms"`
	P50Ms    float64 `json:"p50_ms"`
	P95Ms    float64 `json:"p95_ms"`
}

// Resumo agrega o servidor inteiro, para o painel ter um número de manchete.
type Resumo struct {
	Total       int64          `json:"total"`
	Erros       int64          `json:"erros"`
	TaxaErro    float64        `json:"taxa_erro"`
	Descartadas int64          `json:"descartadas,omitempty"`
	Rotas       []RotaSnapshot `json:"rotas"`
}

// Snapshot devolve as rotas ordenadas da mais movimentada para a menos.
func (c *Coletor) Snapshot() Resumo {
	c.mu.Lock()
	defer c.mu.Unlock()

	res := Resumo{
		Descartadas: c.descartadas,
		Rotas:       make([]RotaSnapshot, 0, len(c.porRota)),
	}

	for rota, st := range c.porRota {
		erros := st.Erros4xx + st.Erros5xx
		snap := RotaSnapshot{
			Rota:     rota,
			Total:    st.Total,
			Erros4xx: st.Erros4xx,
			Erros5xx: st.Erros5xx,
			MaxMs:    float64(st.MaxNs) / 1e6,
		}
		// O percentil é interpolado dentro da faixa e pode passar do topo real
		// (95% de [0,1ms) dá 0,95ms mesmo se a pior requisição levou 0,74ms).
		// O máximo é medido, não estimado: ele manda.
		snap.P50Ms = min(percentil(st.Buckets, st.Total, 50), snap.MaxMs)
		snap.P95Ms = min(percentil(st.Buckets, st.Total, 95), snap.MaxMs)
		if st.Total > 0 {
			snap.MediaMs = float64(st.SomaNs) / float64(st.Total) / 1e6
			snap.TaxaErro = float64(erros) / float64(st.Total)
		}
		res.Rotas = append(res.Rotas, snap)
		res.Total += st.Total
		res.Erros += erros
	}

	if res.Total > 0 {
		res.TaxaErro = float64(res.Erros) / float64(res.Total)
	}

	// Empate resolvido pelo nome: sem isso a ordem do painel dançaria a cada
	// segundo (mapa em Go não tem ordem) e o SSE reemitiria sem nada ter mudado.
	sort.Slice(res.Rotas, func(i, j int) bool {
		if res.Rotas[i].Total != res.Rotas[j].Total {
			return res.Rotas[i].Total > res.Rotas[j].Total
		}
		return res.Rotas[i].Rota < res.Rotas[j].Rota
	})
	return res
}

// percentil interpola dentro do bucket onde o alvo cai — o mesmo método
// aproximado que o Prometheus usa. É uma estimativa, não a amostra real: com
// buckets largos o valor erra dentro da faixa, e é o preço de não guardar
// milhões de amostras.
func percentil(buckets [nBuckets]int64, total int64, p float64) float64 {
	if total == 0 {
		return 0
	}
	alvo := p / 100 * float64(total)

	var acum float64
	for i, n := range buckets {
		inferior, superior := bordasMs(i)
		if n > 0 && acum+float64(n) >= alvo {
			if math.IsInf(superior, 1) {
				// Último bucket não tem teto: devolver o piso conhecido é o
				// máximo que se pode afirmar sem inventar número.
				return inferior
			}
			return inferior + (alvo-acum)/float64(n)*(superior-inferior)
		}
		acum += float64(n)
	}
	// Só se chega aqui por arredondamento; o teto do último bucket com dados.
	inferior, _ := bordasMs(nBuckets - 1)
	return inferior
}

// bordasMs devolve o intervalo do bucket i em milissegundos.
func bordasMs(i int) (inferior, superior float64) {
	if i > 0 {
		inferior = float64(limites[i-1]) / 1e6
	}
	if i < len(limites) {
		return inferior, float64(limites[i]) / 1e6
	}
	return inferior, math.Inf(1)
}
