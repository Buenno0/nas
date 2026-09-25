package sincro

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
)

// Testes de caos do kill switch: acionar no meio de cada operação de rede e
// provar que (1) ela para, (2) nada se perde, (3) ela retoma no híbrido.

// lento entrega o download aos poucos e para quando o contexto morre, como o
// corpo HTTP do SDK faz no kill switch.
type lento struct {
	*memoria
}

func (l lento) Baixar(ctx context.Context, key string, desde int64) (io.ReadCloser, error) {
	rc, err := l.memoria.Baixar(ctx, key, desde)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(&gotejador{ctx: ctx, r: rc}), nil
}

type gotejador struct {
	ctx context.Context
	r   io.Reader
}

func (g *gotejador) Read(p []byte) (int, error) {
	if err := g.ctx.Err(); err != nil {
		return 0, err
	}
	time.Sleep(time.Millisecond)
	if len(p) > 64<<10 {
		p = p[:64<<10]
	}
	return g.r.Read(p)
}

// Kill switch no meio de uma fixação: o parcial fica no disco, e ao voltar o
// híbrido o download continua dele até o arquivo idêntico.
func TestCaosFixarInterrompido(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	f := a.arquivoLocal(t, "Andrei Rublev (1966).mkv", 8<<20)
	original, _ := os.ReadFile(f.Path)
	if err := a.motor.Enviar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	a.esperar(t)
	if err := a.motor.Liberar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	a.esperar(t)

	// Daqui em diante o bucket goteja: ~128 ms para o arquivo inteiro.
	a.chave.Desligar()
	cloud.Registrar(func(context.Context, config.Nuvem, *http.Client) (cloud.Armazenamento, error) {
		return lento{a.mem}, nil
	})
	if err := a.chave.Ativar(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.motor.Fixar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	parcial := f.Path + ".ozy-parcial"
	prazo := time.Now().Add(3 * time.Second)
	for {
		if info, err := os.Stat(parcial); err == nil && info.Size() > 0 {
			break
		}
		if time.Now().After(prazo) {
			t.Fatal("o download nunca começou")
		}
		time.Sleep(time.Millisecond)
	}

	inicio := time.Now()
	a.chave.Desligar()
	a.esperar(t) // a tarefa precisa ficar pausada, não em erro
	if d := time.Since(inicio); d > time.Second {
		t.Fatalf("a fixação levou %s para parar", d)
	}
	info, err := os.Stat(parcial)
	if err != nil || info.Size() >= int64(len(original)) {
		t.Skip("o download terminou antes do kill switch; máquina rápida demais para este teste")
	}
	if g := a.loc(t, f.ID); g.Localizacao != db.LocalBaixando {
		t.Fatalf("depois do kill switch: %s, esperado baixando", g.Localizacao)
	}
	pausado := info.Size()

	if err := a.chave.Ativar(ctx); err != nil {
		t.Fatal(err)
	}
	a.motor.Reconciliar(ctx)
	a.esperar(t)
	if g := a.loc(t, f.ID); g.Localizacao != db.LocalAmbos {
		t.Fatalf("depois de retomar: %s", g.Localizacao)
	}
	baixado, _ := os.ReadFile(f.Path)
	if !bytes.Equal(baixado, original) {
		t.Fatal("o arquivo retomado difere do original")
	}
	if pausado == 0 {
		t.Fatal("nada tinha sido baixado antes da pausa")
	}
}

// filaQueEspera segura o long-poll até o contexto morrer, como o SQS com
// WaitTimeSeconds=20.
type filaQueEspera struct {
	*memoria
	entrou chan struct{}
}

func (f filaQueEspera) EnviarMensagem(context.Context, string, []byte) error { return nil }
func (f filaQueEspera) ApagarMensagem(context.Context, string, string) error { return nil }
func (f filaQueEspera) EstenderVisibilidade(context.Context, string, string, time.Duration) error {
	return nil
}
func (f filaQueEspera) Publicar(context.Context, string, []byte, string) error { return nil }
func (f filaQueEspera) ReceberMensagens(ctx context.Context, _ string, _ int32, _ time.Duration) ([]cloud.Mensagem, error) {
	select {
	case f.entrou <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

// Kill switch com o consumidor do catálogo parado no long-poll: ele sai em
// menos de 100 ms, sem esperar os 20 s do SQS.
func TestCaosLongPollCortado(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	fila := filaQueEspera{memoria: a.mem, entrou: make(chan struct{}, 1)}
	a.chave.Desligar()
	cloud.Registrar(func(context.Context, config.Nuvem, *http.Client) (cloud.Armazenamento, error) {
		return fila, nil
	})
	if err := a.chave.Ativar(ctx); err != nil {
		t.Fatal(err)
	}
	m := Novo(a.db, a.chave, func() int64 { return 0 },
		func() config.Nuvem { return config.Nuvem{FilaEventos: "no-mac"} })

	fim := make(chan struct{})
	go func() {
		m.consumirCatalogo(ctx)
		close(fim)
	}()
	select {
	case <-fila.entrou:
	case <-time.After(2 * time.Second):
		t.Fatal("o consumidor não chegou ao long-poll")
	}

	inicio := time.Now()
	a.chave.Desligar()
	select {
	case <-fim:
		if d := time.Since(inicio); d > 100*time.Millisecond {
			t.Fatalf("o long-poll levou %s para cair", d)
		}
	case <-time.After(time.Second):
		t.Fatal("o consumidor sobreviveu ao kill switch")
	}
}
