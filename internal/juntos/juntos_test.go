package juntos

import (
	"testing"
	"time"
)

func relogio(s *Salas) *time.Time {
	t := time.Unix(1_700_000_000, 0)
	s.agora = func() time.Time { return t }
	return &t
}

func ultimo(ch <-chan Mensagem) Mensagem {
	var m Mensagem
	for {
		select {
		case m = <-ch:
		default:
			return m
		}
	}
}

func TestSalaSincronizaEEsperaQuemCarrega(t *testing.T) {
	s := Novas()
	agora := relogio(s)
	e, err := s.Criar(7, 30, false, "ana")
	if err != nil {
		t.Fatal(err)
	}
	cha, sairA, _ := s.Entrar(e.Codigo, "ana")
	chb, _, _ := s.Entrar(e.Codigo, "bia")
	if got := ultimo(cha).Estado.Presenca; len(got) != 2 {
		t.Fatalf("presença = %v", got)
	}

	_ = s.Aplicar(e.Codigo, "ana", Comando{Tipo: "play", Posicao: 30})
	*agora = agora.Add(10 * time.Second)
	_ = s.Aplicar(e.Codigo, "bia", Comando{Tipo: "carregando"})
	m := ultimo(chb).Estado
	if m.Tocando || m.Posicao != 40 || len(m.Aguardando) != 1 {
		t.Fatalf("carregando deveria pausar em 40: %+v", m)
	}
	_ = s.Aplicar(e.Codigo, "bia", Comando{Tipo: "pronto"})
	if m := ultimo(cha).Estado; !m.Tocando || m.Posicao != 40 {
		t.Fatalf("pronto deveria retomar: %+v", m)
	}

	// Pausa de alguém não é desfeita por "pronto".
	_ = s.Aplicar(e.Codigo, "ana", Comando{Tipo: "pause", Posicao: 41})
	_ = s.Aplicar(e.Codigo, "bia", Comando{Tipo: "pronto"})
	if m := ultimo(chb).Estado; m.Tocando {
		t.Fatal("pronto retomou uma pausa de verdade")
	}

	sairA()
	if m := ultimo(chb).Estado; len(m.Presenca) != 1 || m.Presenca[0] != "bia" {
		t.Fatalf("presença depois de sair = %v", m.Presenca)
	}
	if err := s.Aplicar(e.Codigo, "bia", Comando{Tipo: "reacao", Emoji: "<script>"}); err == nil {
		t.Fatal("reação arbitrária aceita")
	}
}

func TestSalaVaziaSome(t *testing.T) {
	s := Novas()
	agora := relogio(s)
	e, _ := s.Criar(1, 0, false, "ana")
	*agora = agora.Add(vidaSemNinguem + time.Minute)
	_, _ = s.Criar(2, 0, false, "ana")
	if _, err := s.Estado(e.Codigo); err != ErrSalaNaoExiste {
		t.Fatalf("sala abandonada ainda existe: %v", err)
	}
}

func TestSoODonoEncerra(t *testing.T) {
	s := Novas()
	e, _ := s.Criar(1, 12, true, "ana")
	if !e.Tocando || e.Posicao != 12 {
		t.Fatalf("sala deveria nascer no ponto do dono: %+v", e)
	}
	chb, _, _ := s.Entrar(e.Codigo, "bia")
	if err := s.Aplicar(e.Codigo, "bia", Comando{Tipo: "encerrar"}); err == nil {
		t.Fatal("convidado encerrou a sala")
	}
	if err := s.Aplicar(e.Codigo, "ana", Comando{Tipo: "encerrar"}); err != nil {
		t.Fatal(err)
	}
	if m := ultimo(chb); m.Tipo != "fim" {
		t.Fatalf("convidado não soube do fim: %+v", m)
	}
	if _, err := s.Estado(e.Codigo); err != ErrSalaNaoExiste {
		t.Fatal("sala encerrada ainda existe")
	}
}

func TestPuloEsperaTodosCarregarem(t *testing.T) {
	s := Novas()
	relogio(s)
	e, _ := s.Criar(1, 0, true, "ana")
	cha, _, _ := s.Entrar(e.Codigo, "ana")
	s.Entrar(e.Codigo, "bia")
	_ = s.Aplicar(e.Codigo, "ana", Comando{Tipo: "seek", Posicao: 600, Cliente: "x", Seq: 3})
	m := ultimo(cha).Estado
	if m.Tocando || m.Posicao != 600 || len(m.Aguardando) != 2 || m.Seq != 3 {
		t.Fatalf("pulo deveria segurar a sala esperando os dois: %+v", m)
	}
	_ = s.Aplicar(e.Codigo, "ana", Comando{Tipo: "pronto"})
	if ultimo(cha).Estado.Tocando {
		t.Fatal("retomou sem esperar a bia")
	}
	_ = s.Aplicar(e.Codigo, "bia", Comando{Tipo: "pronto"})
	if m := ultimo(cha).Estado; !m.Tocando || m.Posicao != 600 {
		t.Fatalf("deveria retomar em 600: %+v", m)
	}
}
