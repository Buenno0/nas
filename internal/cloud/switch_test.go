package cloud

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nas/internal/config"
)

// falso é um Armazenamento que fala HTTP pelo cliente guardado, como o S3.
type falso struct {
	Armazenamento
	cliente *http.Client
	url     string
	espera  chan struct{} // se não nil, Verificar trava até fechar ou ctx morrer
}

func (f *falso) Verificar(ctx context.Context) error {
	if f.espera != nil {
		select {
		case <-f.espera:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.get(ctx)
}

func (f *falso) get(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	resp, err := f.cliente.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}

func preparar(t *testing.T, espera chan struct{}) (*Chave, *falso) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)

	var f *falso
	anterior := conectorAtual()
	Registrar(func(_ context.Context, _ config.Nuvem, c *http.Client) (Armazenamento, error) {
		f.cliente = c
		return f, nil
	})
	t.Cleanup(func() { Registrar(anterior) })

	c := Nova(func() config.Nuvem { return config.Nuvem{Bucket: "b", Regiao: "r"} }, false)
	f = &falso{cliente: c.Cliente(), url: srv.URL, espera: espera}
	return c, f
}

func TestGuardBloqueiaNoModoLocal(t *testing.T) {
	c, f := preparar(t, nil)
	if err := f.get(context.Background()); !errors.Is(err, ErrModoLocal) {
		t.Fatalf("esperava ErrModoLocal, veio %v", err)
	}
	if c.Bloqueadas() != 1 {
		t.Fatalf("nuvem_bloqueadas_total = %d, esperava 1", c.Bloqueadas())
	}
}

func TestAtivarEDesligar(t *testing.T) {
	c, f := preparar(t, nil)
	if err := c.Ativar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Modo() != ModoHibrido {
		t.Fatalf("modo = %s", c.Modo())
	}
	if err := f.get(context.Background()); err != nil {
		t.Fatalf("no híbrido a requisição deveria passar: %v", err)
	}

	_, raiz, _ := c.Hibrido()
	inicio := time.Now()
	c.Desligar()
	select {
	case <-raiz.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("o contexto raiz não foi cancelado em 100 ms")
	}
	if d := time.Since(inicio); d > 100*time.Millisecond {
		t.Fatalf("kill switch levou %s", d)
	}
	if err := f.get(context.Background()); !errors.Is(err, ErrModoLocal) {
		t.Fatalf("depois do kill switch deveria bloquear, veio %v", err)
	}
	if _, _, ok := c.Hibrido(); ok {
		t.Fatal("Hibrido() ainda devolve o bucket depois do kill switch")
	}
}

func TestKillSwitchDuranteConexao(t *testing.T) {
	espera := make(chan struct{})
	c, _ := preparar(t, espera)

	erro := make(chan error, 1)
	go func() { erro <- c.Ativar(context.Background()) }()

	for c.Modo() != ModoConectando {
		time.Sleep(time.Millisecond)
	}
	c.Desligar()

	select {
	case err := <-erro:
		if !errors.Is(err, ErrInterrompido) && !errors.Is(err, context.Canceled) {
			t.Fatalf("esperava interrupção, veio %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Ativar não respeitou o kill switch")
	}
	if c.Modo() != ModoLocal {
		t.Fatalf("modo = %s, esperava local", c.Modo())
	}
}

func TestVincularMorreComKillSwitch(t *testing.T) {
	c, _ := preparar(t, nil)
	if err := c.Ativar(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, ctx, cancel, ok := c.Vincular(context.Background())
	if !ok {
		t.Fatal("Vincular falhou no híbrido")
	}
	defer cancel()
	c.Desligar()
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("contexto vinculado sobreviveu ao kill switch")
	}
}

func TestTravadoENaoConfigurado(t *testing.T) {
	c := Nova(func() config.Nuvem { return config.Nuvem{} }, true)
	if err := c.Ativar(context.Background()); !errors.Is(err, ErrTravado) {
		t.Fatalf("--sem-nuvem: esperava ErrTravado, veio %v", err)
	}
	c = Nova(func() config.Nuvem { return config.Nuvem{} }, false)
	if err := c.Ativar(context.Background()); !errors.Is(err, ErrNaoConfig) {
		t.Fatalf("esperava ErrNaoConfig, veio %v", err)
	}
	if c.Estado().Erro == "" {
		t.Fatal("o erro de conexão deveria ficar visível no Estado")
	}
}
