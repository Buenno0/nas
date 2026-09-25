package metrics

import (
	"math"
	"testing"
	"time"
)

func TestIndiceBucketBordas(t *testing.T) {
	casos := []struct {
		nome     string
		duracao  time.Duration
		esperado int
	}{
		{"zero", 0, 0},
		{"0.2ms", 200 * time.Microsecond, 0},
		// A borda é o caso que mais dá errado em histograma: 1ms tem de cair na
		// faixa de cima, senão "<1ms" mente sobre incluir o próprio 1.
		{"1ms exato", time.Millisecond, 1},
		{"2.4ms", 2400 * time.Microsecond, 1},
		{"2.5ms exato", 2500 * time.Microsecond, 2},
		{"5ms exato", 5 * time.Millisecond, 3},
		{"9ms", 9 * time.Millisecond, 3},
		{"10ms exato", 10 * time.Millisecond, 4},
		{"25ms exato", 25 * time.Millisecond, 5},
		{"50ms exato", 50 * time.Millisecond, 6},
		{"100ms exato", 100 * time.Millisecond, 7},
		{"250ms exato", 250 * time.Millisecond, 8},
		{"500ms exato", 500 * time.Millisecond, 9},
		{"999ms", 999 * time.Millisecond, 9},
		{"1s exato", time.Second, 10},
		{"4.9s", 4900 * time.Millisecond, 10},
		{"5s exato", 5 * time.Second, 11},
		{"2h", 2 * time.Hour, 11},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := indiceBucket(c.duracao); got != c.esperado {
				t.Fatalf("indiceBucket(%v) = %d, queria %d", c.duracao, got, c.esperado)
			}
		})
	}
}

func TestObservaContagemEErros(t *testing.T) {
	c := NovoColetor()
	c.Observa("GET /api/home", 200, 5*time.Millisecond, false)
	c.Observa("GET /api/home", 404, 6*time.Millisecond, false)
	c.Observa("GET /api/home", 500, 7*time.Millisecond, false)
	c.Observa("GET /api/titles", 200, 80*time.Millisecond, false)

	res := c.Snapshot()
	if res.Total != 4 {
		t.Fatalf("total = %d, queria 4", res.Total)
	}
	if res.Erros != 2 {
		t.Fatalf("erros = %d, queria 2", res.Erros)
	}

	// Ordenado por movimento: /api/home tem 3, /api/titles tem 1.
	if len(res.Rotas) != 2 || res.Rotas[0].Rota != "GET /api/home" {
		t.Fatalf("ordem inesperada: %+v", res.Rotas)
	}
	home := res.Rotas[0]
	if home.Erros4xx != 1 || home.Erros5xx != 1 {
		t.Fatalf("erros por classe = 4xx:%d 5xx:%d, queria 1 e 1", home.Erros4xx, home.Erros5xx)
	}
	if math.Abs(home.TaxaErro-2.0/3.0) > 1e-9 {
		t.Fatalf("taxa de erro = %v, queria 0.666…", home.TaxaErro)
	}
	if math.Abs(home.MediaMs-6) > 1e-9 {
		t.Fatalf("média = %v ms, queria 6", home.MediaMs)
	}
	if math.Abs(home.MaxMs-7) > 1e-9 {
		t.Fatalf("máximo = %v ms, queria 7", home.MaxMs)
	}
}

func TestObservaIgnoraStreamingERotaVazia(t *testing.T) {
	c := NovoColetor()
	// Duas horas de vídeo: se entrasse na conta, arruinaria todo percentil.
	c.Observa("GET /stream/{id}", 200, 2*time.Hour, true)
	c.Observa("", 200, time.Millisecond, false)

	res := c.Snapshot()
	if res.Total != 0 || len(res.Rotas) != 0 {
		t.Fatalf("nada devia ter sido registrado, veio %+v", res)
	}
}

func TestPercentilInterpolado(t *testing.T) {
	c := NovoColetor()
	// 105 requisições espalhadas na faixa 10–25ms: o p50 tem de cair no meio
	// dela (17,5ms), não numa das bordas. O máximo real é 24ms, então o p95
	// interpolado (24,25) chega perto sem ser cortado longe.
	for i := 0; i < 105; i++ {
		c.Observa("GET /x", 200, time.Duration(10+i%15)*time.Millisecond, false)
	}
	r := c.Snapshot().Rotas[0]
	if math.Abs(r.P50Ms-17.5) > 0.5 {
		t.Fatalf("p50 = %v ms, queria ~17.5 (meio da faixa 10–25)", r.P50Ms)
	}
	if math.Abs(r.P95Ms-24) > 0.5 {
		t.Fatalf("p95 = %v ms, queria ~24 (estimativa 24,25 limitada ao máximo real)", r.P95Ms)
	}
}

func TestPercentilUltimoBucketNaoInventaTeto(t *testing.T) {
	c := NovoColetor()
	c.Observa("GET /lento", 200, 30*time.Second, false)
	r := c.Snapshot().Rotas[0]
	// O bucket ≥5s não tem teto: o único valor honesto é o piso.
	if r.P95Ms != 5000 {
		t.Fatalf("p95 = %v ms, queria 5000 (piso do último bucket)", r.P95Ms)
	}
	// O máximo real, esse sim, é medido de verdade.
	if math.Abs(r.MaxMs-30000) > 1 {
		t.Fatalf("máximo = %v ms, queria 30000", r.MaxMs)
	}
}

func TestPercentilPulaBucketsVazios(t *testing.T) {
	c := NovoColetor()
	// 99 rápidas e 1 lenta: o p50 fica no bucket rápido, o p95 também, e nada
	// deve estourar por causa dos buckets vazios no meio.
	for i := 0; i < 99; i++ {
		c.Observa("GET /y", 200, time.Millisecond, false)
	}
	c.Observa("GET /y", 200, 3*time.Second, false)
	r := c.Snapshot().Rotas[0]
	if r.P50Ms <= 0 || r.P50Ms > 10 {
		t.Fatalf("p50 = %v ms, queria dentro de 0–10", r.P50Ms)
	}
	if r.P95Ms <= 0 || r.P95Ms > 10 {
		t.Fatalf("p95 = %v ms, queria dentro de 0–10", r.P95Ms)
	}
}

func TestSnapshotVazio(t *testing.T) {
	res := NovoColetor().Snapshot()
	if res.Total != 0 || res.TaxaErro != 0 || len(res.Rotas) != 0 {
		t.Fatalf("coletor novo devia estar zerado, veio %+v", res)
	}
}

func TestTetoDeCardinalidade(t *testing.T) {
	c := NovoColetor()
	for i := 0; i < maxRotas+10; i++ {
		c.Observa(string(rune('a'+i%26))+time.Duration(i).String(), 200, time.Millisecond, false)
	}
	res := c.Snapshot()
	if len(res.Rotas) > maxRotas {
		t.Fatalf("%d rotas guardadas, o teto é %d", len(res.Rotas), maxRotas)
	}
	if res.Descartadas == 0 {
		t.Fatal("descartes deviam ter sido contados, não escondidos")
	}
}

// O bug que a verificação ao vivo pegou: com faixas largas embaixo, uma rota
// cujo PIOR caso real foi 1ms reportava "p50 de 5ms". O percentil estimado
// nunca deve passar longe do máximo medido, que é exato.
func TestPercentilNaoExcedeOMaximoRealNoSubMilissegundo(t *testing.T) {
	c := NovoColetor()
	for i := 0; i < 50; i++ {
		c.Observa("GET /api/home", 200, 500*time.Microsecond, false)
	}
	r := c.Snapshot().Rotas[0]
	if r.P50Ms > 1.0 {
		t.Fatalf("p50 = %v ms para requisições de 0,5ms — a faixa de baixo está larga demais", r.P50Ms)
	}
	if r.P95Ms > 1.0 {
		t.Fatalf("p95 = %v ms para requisições de 0,5ms", r.P95Ms)
	}
}

// Percentil não pode passar do máximo: o máximo é medido, o percentil é
// estimado, e uma estimativa acima do observado é simplesmente falsa.
func TestPercentilNuncaPassaDoMaximo(t *testing.T) {
	c := NovoColetor()
	for i := 0; i < 20; i++ {
		c.Observa("GET /api/home", 200, 740*time.Microsecond, false)
	}
	r := c.Snapshot().Rotas[0]
	if r.P95Ms > r.MaxMs {
		t.Fatalf("p95 = %v ms acima do máximo medido de %v ms", r.P95Ms, r.MaxMs)
	}
	if r.P50Ms > r.MaxMs {
		t.Fatalf("p50 = %v ms acima do máximo medido de %v ms", r.P50Ms, r.MaxMs)
	}
}
