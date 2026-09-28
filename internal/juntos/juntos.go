// Package juntos guarda as salas de "assistir junto": quem está vendo o quê,
// se está tocando e em que ponto. Cada espectador toca o vídeo do seu jeito
// (direto ou a versão preparada); a sala só sincroniza o relógio, então não
// há vídeo passando por aqui, nem custo além de algumas mensagens.
//
// As salas vivem na memória do nó que as criou: some uma reinicialização,
// e tudo bem — é uma sessão de cinema, não um dado.
package juntos

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"
)

// Salas sem ninguém somem depois disto.
const vidaSemNinguem = 30 * time.Minute

// Limite para não virar um depósito de salas abandonadas.
const maxSalas = 64

var ErrSalaNaoExiste = errors.New("sala não existe")
var ErrMuitasSalas = errors.New("salas demais abertas agora")

// Estado é o que cada espectador precisa para se alinhar. Posicao vale no
// instante Em (ms Unix do servidor); tocando, o ponto atual é
// Posicao + (agora − Em).
type Estado struct {
	Codigo  string  `json:"codigo"`
	FileID  int64   `json:"file_id"`
	Tocando bool    `json:"tocando"`
	Posicao float64 `json:"posicao"`
	Em      int64   `json:"em"`
	Por     string  `json:"por,omitempty"` // quem fez a última ação
	Dono    string  `json:"dono"`          // quem abriu; só ele encerra
	// Aba que mandou o último comando e o número dele: essa aba ignora ecos
	// de comandos seus mais velhos que o último que ela já aplicou.
	Cliente  string   `json:"cliente,omitempty"`
	Seq      int64    `json:"seq,omitempty"`
	Presenca []string `json:"presenca"`
	// Aguardando lista quem está carregando: a sala espera por eles.
	Aguardando []string `json:"aguardando,omitempty"`
}

// Mensagem é o que vai pelo SSE: um estado novo ou uma reação.
type Mensagem struct {
	Tipo   string  `json:"tipo"` // "estado" | "reacao" | "fim"
	Estado *Estado `json:"estado,omitempty"`
	De     string  `json:"de,omitempty"`
	Emoji  string  `json:"emoji,omitempty"`
}

type espectador struct {
	nome string
	ch   chan Mensagem
}

type sala struct {
	estado     Estado
	vistos     map[*espectador]struct{}
	carregando map[string]bool
	vazia      time.Time
	pausouPor  bool // a pausa atual foi da sala, esperando alguém carregar
	espera     *time.Timer
}

// Depois de um pulo, a sala espera todo mundo carregar no ponto novo, mas
// não para sempre: quem demora mais que isto se alinha andando.
const esperaMaxima = 8 * time.Second

type Salas struct {
	mu    sync.Mutex
	salas map[string]*sala
	agora func() time.Time
}

func Novas() *Salas {
	return &Salas{salas: map[string]*sala{}, agora: time.Now}
}

// Alfabeto sem letras que se confundem ditas em voz alta (0/O, 1/I/L).
const alfabeto = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func novoCodigo() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alfabeto[int(b[i])%len(alfabeto)]
	}
	return string(b)
}

// Criar abre uma sala no ponto (e no estado, tocando ou não) de quem a abriu,
// para ele não levar um tranco ao virar anfitrião.
func (s *Salas) Criar(fileID int64, posicao float64, tocando bool, por string) (Estado, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limpar()
	if len(s.salas) >= maxSalas {
		return Estado{}, ErrMuitasSalas
	}
	codigo := novoCodigo()
	for s.salas[codigo] != nil {
		codigo = novoCodigo()
	}
	sl := &sala{
		estado:     Estado{Codigo: codigo, FileID: fileID, Tocando: tocando, Posicao: posicao, Em: s.ms(), Por: por, Dono: por},
		vistos:     map[*espectador]struct{}{},
		carregando: map[string]bool{},
		vazia:      s.agora(),
	}
	s.salas[codigo] = sl
	return s.foto(sl), nil
}

func Normaliza(codigo string) string { return strings.ToUpper(strings.TrimSpace(codigo)) }

func (s *Salas) Estado(codigo string) (Estado, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl := s.salas[Normaliza(codigo)]
	if sl == nil {
		return Estado{}, ErrSalaNaoExiste
	}
	return s.foto(sl), nil
}

// Entrar inscreve um espectador. O canal recebe o estado atual logo de cara;
// sair deve ser chamado quando a conexão cair.
func (s *Salas) Entrar(codigo, nome string) (<-chan Mensagem, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl := s.salas[Normaliza(codigo)]
	if sl == nil {
		return nil, nil, ErrSalaNaoExiste
	}
	e := &espectador{nome: nome, ch: make(chan Mensagem, 16)}
	sl.vistos[e] = struct{}{}
	s.espalhar(sl)
	sair := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := sl.vistos[e]; !ok {
			return
		}
		delete(sl.vistos, e)
		if !s.presente(sl, nome) {
			delete(sl.carregando, nome)
		}
		if len(sl.vistos) == 0 {
			sl.vazia = s.agora()
		}
		s.espalhar(sl)
	}
	return e.ch, sair, nil
}

// Comando é uma ação de um espectador.
type Comando struct {
	Tipo    string  `json:"tipo"` // play | pause | seek | arquivo | carregando | pronto | reacao
	Posicao float64 `json:"posicao"`
	FileID  int64   `json:"file_id,omitempty"`
	Emoji   string  `json:"emoji,omitempty"`
	Cliente string  `json:"cliente,omitempty"`
	Seq     int64   `json:"seq,omitempty"`
}

var reacoes = map[string]bool{"😂": true, "😱": true, "😍": true, "👏": true, "😢": true, "🔥": true, "🍿": true, "👀": true}

func (s *Salas) Aplicar(codigo, nome string, c Comando) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl := s.salas[Normaliza(codigo)]
	if sl == nil {
		return ErrSalaNaoExiste
	}
	if c.Posicao < 0 || c.Posicao != c.Posicao {
		c.Posicao = 0
	}
	e := &sl.estado
	if c.Tipo == "play" || c.Tipo == "pause" || c.Tipo == "seek" {
		e.Cliente, e.Seq = c.Cliente, c.Seq
	}
	switch c.Tipo {
	case "play":
		e.Tocando, e.Posicao, e.Em, e.Por = true, c.Posicao, s.ms(), nome
		s.desarmar(sl)
	case "pause":
		e.Tocando, e.Posicao, e.Em, e.Por = false, c.Posicao, s.ms(), nome
		s.desarmar(sl)
	case "seek":
		e.Posicao, e.Em, e.Por = c.Posicao, s.ms(), nome
		// Tocando, um pulo faz todo mundo carregar ao mesmo tempo: a sala
		// segura o relógio até cada um avisar "pronto" no ponto novo, senão
		// o ponto seguia andando e quem carregava nunca alcançava.
		if e.Tocando {
			e.Tocando, sl.pausouPor = false, true
			sl.carregando = map[string]bool{}
			for v := range sl.vistos {
				sl.carregando[v.nome] = true
			}
			s.armarEspera(sl)
		}
	case "arquivo":
		if c.FileID <= 0 {
			return errors.New("arquivo inválido")
		}
		e.FileID, e.Posicao, e.Em, e.Por, e.Tocando = c.FileID, 0, s.ms(), nome, false
		s.desarmar(sl)
	case "carregando":
		// Alguém travou: a sala para no ponto atual e espera.
		if !sl.carregando[nome] {
			sl.carregando[nome] = true
			if e.Tocando {
				e.Posicao, e.Em, e.Tocando = s.posicaoAgora(sl), s.ms(), false
				sl.pausouPor = true
				s.armarEspera(sl)
			}
		}
	case "pronto":
		delete(sl.carregando, nome)
		if len(sl.carregando) == 0 && sl.pausouPor {
			s.retomar(sl)
		}
	case "encerrar":
		if nome != e.Dono {
			return errors.New("só quem abriu a sala pode encerrá-la")
		}
		for v := range sl.vistos {
			entregar(v, Mensagem{Tipo: "fim", De: nome})
		}
		delete(s.salas, e.Codigo)
		return nil
	case "reacao":
		if !reacoes[c.Emoji] {
			return errors.New("reação desconhecida")
		}
		for v := range sl.vistos {
			entregar(v, Mensagem{Tipo: "reacao", De: nome, Emoji: c.Emoji})
		}
		return nil
	default:
		return errors.New("comando desconhecido")
	}
	s.espalhar(sl)
	return nil
}

func (s *Salas) retomar(sl *sala) {
	sl.estado.Tocando, sl.estado.Em = true, s.ms()
	s.desarmar(sl)
}

// desarmar encerra a espera por quem carrega (uma pausa ou play de verdade
// passa por cima dela).
func (s *Salas) desarmar(sl *sala) {
	sl.pausouPor = false
	sl.carregando = map[string]bool{}
	if sl.espera != nil {
		sl.espera.Stop()
		sl.espera = nil
	}
}

func (s *Salas) armarEspera(sl *sala) {
	if sl.espera != nil {
		sl.espera.Stop()
	}
	sl.espera = time.AfterFunc(esperaMaxima, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if sl.pausouPor && s.salas[sl.estado.Codigo] == sl {
			s.retomar(sl)
			s.espalhar(sl)
		}
	})
}

func (s *Salas) ms() int64 { return s.agora().UnixMilli() }

func (s *Salas) posicaoAgora(sl *sala) float64 {
	e := sl.estado
	if !e.Tocando {
		return e.Posicao
	}
	return e.Posicao + float64(s.ms()-e.Em)/1000
}

func (s *Salas) presente(sl *sala, nome string) bool {
	for v := range sl.vistos {
		if v.nome == nome {
			return true
		}
	}
	return false
}

func (s *Salas) foto(sl *sala) Estado {
	e := sl.estado
	vistos := map[string]bool{}
	e.Presenca = []string{}
	for v := range sl.vistos {
		if !vistos[v.nome] {
			vistos[v.nome] = true
			e.Presenca = append(e.Presenca, v.nome)
		}
	}
	for nome := range sl.carregando {
		e.Aguardando = append(e.Aguardando, nome)
	}
	return e
}

func (s *Salas) espalhar(sl *sala) {
	e := s.foto(sl)
	for v := range sl.vistos {
		entregar(v, Mensagem{Tipo: "estado", Estado: &e})
	}
}

// entregar nunca bloqueia: quem está lento perde mensagens velhas, e a
// próxima traz o estado inteiro de novo.
func entregar(v *espectador, m Mensagem) {
	select {
	case v.ch <- m:
	default:
		select {
		case <-v.ch:
		default:
		}
		select {
		case v.ch <- m:
		default:
		}
	}
}

func (s *Salas) limpar() {
	for k, sl := range s.salas {
		if len(sl.vistos) == 0 && s.agora().Sub(sl.vazia) > vidaSemNinguem {
			delete(s.salas, k)
		}
	}
}
