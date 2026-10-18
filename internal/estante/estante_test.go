package estante

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// centralDeTeste aponta para a TV falsa e responde o DDP com consoles fixos,
// contando quantas vezes a rede foi consultada.
func centralDeTeste(t *testing.T, descricao string, consoles ...Console) (*Central, *atomic.Int32) {
	t.Helper()
	var sondas atomic.Int32
	c := NovaCentral(func() Alvos { return Alvos{TV: descricao} })
	c.espera = 10 * time.Millisecond
	c.procurarPS5 = func(context.Context, string, time.Duration) ([]Console, error) {
		sondas.Add(1)
		return consoles, nil
	}
	c.procurarTVs = func(context.Context, time.Duration) ([]*TV, error) { return nil, nil }
	return c, &sondas
}

func TestCentralSondaEComanda(t *testing.T) {
	srv := tvFalsa(t)
	defer srv.Close()
	ctx := context.Background()
	c, _ := centralDeTeste(t, srv.URL+"/dmr.xml",
		Console{Tipo: "PS4", Nome: "velho", Endereco: "10.0.0.2", Estado: PS5Ligado},
		Console{Tipo: "PS5", Nome: "PS5-858", Endereco: "10.0.0.3", Estado: PS5Descanso},
	)

	e := c.Sondar(ctx)
	if e.PS5 == nil || e.PS5.Nome != "PS5-858" || e.PS5.Estado != PS5Descanso {
		t.Fatalf("PS5: %+v", e.PS5)
	}
	if e.TV == nil || e.TV.Nome != "KDL-32W655D" || e.TV.Volume != 2 || e.TV.Max != VolumeMax {
		t.Fatalf("TV: %+v", e.TV)
	}

	if e, err := c.Passo(ctx, 5); err != nil || e.TV.Volume != 7 {
		t.Fatalf("passo: %+v %v", e.TV, err)
	}
	if e, err := c.DefinirVolume(ctx, 250); err != nil || e.TV.Volume != VolumeMax {
		t.Fatalf("volume acima do teto: %+v %v", e.TV, err)
	}
	if e, err := c.Passo(ctx, -500); err != nil || e.TV.Volume != 0 {
		t.Fatalf("volume abaixo de zero: %+v %v", e.TV, err)
	}
	if e, err := c.DefinirMudo(ctx, true); err != nil || !e.TV.Mudo {
		t.Fatalf("mudo: %+v %v", e.TV, err)
	}
}

func TestCentralTVDesligada(t *testing.T) {
	srv := tvFalsa(t)
	desc := srv.URL + "/dmr.xml"
	srv.Close() // a TV desligou: ninguém atende
	c, _ := centralDeTeste(t, desc)

	if e := c.Sondar(context.Background()); e.TV != nil || e.PS5 != nil {
		t.Fatalf("nada devia responder: %+v", e)
	}
	if _, err := c.DefinirVolume(context.Background(), 10); !errors.Is(err, ErrTVDesligada) {
		t.Fatalf("esperava ErrTVDesligada, veio %v", err)
	}
}

func TestCentralSoAvisaQuandoMuda(t *testing.T) {
	c, _ := centralDeTeste(t, "")
	ch, sair := c.Assinar()
	defer sair()

	ps5 := &PS5Visto{Nome: "PS5-858", Estado: PS5Ligado}
	c.publicar(Estado{PS5: ps5, Em: time.Now()})
	c.publicar(Estado{PS5: &PS5Visto{Nome: "PS5-858", Estado: PS5Ligado}, Em: time.Now()})
	c.publicar(Estado{Em: time.Now()}) // desligou

	if e := <-ch; e.PS5 == nil {
		t.Fatal("primeiro aviso devia trazer o PS5")
	}
	if e := <-ch; e.PS5 != nil {
		t.Fatal("o segundo aviso devia ser o desligado; o repetido não devia ter saído")
	}
	select {
	case e := <-ch:
		t.Fatalf("aviso a mais: %+v", e)
	default:
	}
}

func TestCentralSoSondaComOuvintes(t *testing.T) {
	c, sondas := centralDeTeste(t, "")
	c.intervalo = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Rodar(ctx)

	time.Sleep(40 * time.Millisecond)
	if n := sondas.Load(); n != 0 {
		t.Fatalf("sem ouvintes, sondou %d vezes", n)
	}

	_, sair := c.Assinar()
	prazo := time.Now().Add(2 * time.Second)
	for sondas.Load() < 3 && time.Now().Before(prazo) {
		time.Sleep(5 * time.Millisecond)
	}
	if sondas.Load() < 3 {
		t.Fatalf("com ouvinte, sondou só %d vezes", sondas.Load())
	}

	sair()
	time.Sleep(30 * time.Millisecond) // a sondagem em curso termina
	antes := sondas.Load()
	time.Sleep(40 * time.Millisecond)
	if depois := sondas.Load(); depois > antes+1 {
		t.Fatalf("depois de sair, continuou sondando: %d → %d", antes, depois)
	}
}

// O SSDP acha a TV uma vez; depois o multicast se perde, mas a Central
// lembra a descrição e reabre a TV por HTTP direto.
func TestCentralLembraATVSemMulticast(t *testing.T) {
	srv := tvFalsa(t)
	defer srv.Close()
	ctx := context.Background()
	c, _ := centralDeTeste(t, "")
	var buscas atomic.Int32
	c.procurarTVs = func(ctx context.Context, _ time.Duration) ([]*TV, error) {
		if buscas.Add(1) > 1 {
			return nil, nil // o Wi-Fi perdeu o M-SEARCH
		}
		tv, err := AbrirTV(ctx, srv.URL+"/dmr.xml", nil)
		return []*TV{tv}, err
	}

	if e := c.Sondar(ctx); e.TV == nil {
		t.Fatal("primeira sondagem devia achar a TV pelo SSDP")
	}
	c.esquecerTV(c.tv) // como se ela tivesse parado de responder uma vez
	if e := c.Sondar(ctx); e.TV == nil || e.TV.Nome != "KDL-32W655D" {
		t.Fatalf("devia reabrir pela última descrição: %+v", e.TV)
	}
	if n := buscas.Load(); n != 1 {
		t.Fatalf("não devia ter precisado de SSDP de novo: %d buscas", n)
	}
}
