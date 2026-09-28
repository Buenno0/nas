package bastao

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"nas/internal/cloud"
)

type memoria struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memoria) Gravar(_ context.Context, k string, c []byte, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.m[k] = append([]byte(nil), c...)
	return nil
}

func (b *memoria) Baixar(_ context.Context, k string, _ int64) (io.ReadCloser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.m[k]
	if !ok {
		return nil, cloud.ErrNaoExiste
	}
	return io.NopCloser(bytes.NewReader(c)), nil
}

func rapido(t *testing.T) {
	t.Helper()
	ant := []time.Duration{RenovaCada, Vencido, OlhaCada, EsperaMaxima}
	RenovaCada, Vencido, OlhaCada, EsperaMaxima = 20*time.Millisecond, 200*time.Millisecond, 5*time.Millisecond, 2*time.Second
	t.Cleanup(func() { RenovaCada, Vencido, OlhaCada, EsperaMaxima = ant[0], ant[1], ant[2], ant[3] })
}

func TestPassagemDeBastao(t *testing.T) {
	rapido(t)
	b := &memoria{m: map[string][]byte{}}
	ctx := context.Background()

	// Primeira task: bucket vazio, assume na hora.
	if err := Esperar(ctx, b, "velha", time.Now); err != nil {
		t.Fatal(err)
	}
	velhaCtx, encerrar := context.WithCancel(ctx)
	passou := make(chan struct{})
	go Manter(velhaCtx, b, "velha", time.Now, func() {
		close(passou)
		encerrar()
	})
	time.Sleep(30 * time.Millisecond)

	// A nova pede a vez; a velha passa, e só depois de liberar a nova segue.
	pronta := make(chan error, 1)
	go func() { pronta <- Esperar(ctx, b, "nova", time.Now) }()
	select {
	case <-passou:
	case <-time.After(time.Second):
		t.Fatal("a velha não viu o pedido")
	}
	select {
	case <-pronta:
		t.Fatal("a nova assumiu antes da velha liberar")
	case <-time.After(50 * time.Millisecond):
	}
	if err := Liberar(ctx, b, "velha", time.Now); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-pronta:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("a nova não assumiu depois da liberação")
	}
}

func TestDonaSumidaVence(t *testing.T) {
	rapido(t)
	b := &memoria{m: map[string][]byte{}}
	ctx := context.Background()
	_ = gravar(ctx, b, chaveBastao, registro{Dono: "morta", Em: time.Now()})
	inicio := time.Now()
	if err := Esperar(ctx, b, "nova", time.Now); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(inicio); d < Vencido/2 || d > EsperaMaxima {
		t.Fatalf("assumiu em %s; devia esperar o registro vencer (%s)", d, Vencido)
	}
}

func TestPedidoAntigoNaoDerrubaQuemAssumiu(t *testing.T) {
	rapido(t)
	b := &memoria{m: map[string][]byte{}}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_ = gravar(ctx, b, chavePedido, pedido{Por: "outra", Em: time.Now().Add(-time.Minute)})
	derrubou := false
	Manter(ctx, b, "eu", time.Now, func() { derrubou = true })
	if derrubou {
		t.Fatal("um pedido de antes de assumir derrubou a task")
	}
}
