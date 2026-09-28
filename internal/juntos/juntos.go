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
	// SoDono: "modo cinema", só quem abriu controla play, pausa e pulos.
	SoDono bool `json:"so_dono,omitempty"`
}

// Linha é uma mensagem do chat da sala.
type Linha struct {
	De    string `json:"de"`
	Texto string `json:"texto"`
	Em    int64  `json:"em"`
}

// Conexao é como um espectador está: a diferença para o ponto da sala e se
// está travado carregando. Quem manda é o próprio espectador.
type Conexao struct {
	Dif     float64 `json:"dif"`
	Travado bool    `json:"travado"`
}

// Mensagem é o que vai pelo SSE.
type Mensagem struct {
	Tipo    string   `json:"tipo"` // estado | reacao | fim | chat | historico | conexao
	Estado  *Estado  `json:"estado,omitempty"`
	De      string   `json:"de,omitempty"`
	Emoji   string   `json:"emoji,omitempty"`
	Linha   *Linha   `json:"linha,omitempty"`
	Chat    []Linha  `json:"chat,omitempty"`
	Conexao *Conexao `json:"conexao,omitempty"`
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
	chat       []Linha
	pausouEm   time.Time // para a contagem regressiva depois de pausa longa
	contadoAte int64     // até onde o tempo assistido junto já foi somado (ms)
}

// Contagem regressiva: dar play depois de uma pausa mais longa que
// pausaLonga começa em contagem ms, para ninguém perder o começo.
const (
	contagem   = 3000
	pausaLonga = 5 * time.Second
	maxChat    = 50
	maxTexto   = 300
)

// Depois de um pulo, a sala espera todo mundo carregar no ponto novo, mas
// não para sempre: quem demora mais que isto se alinha andando.
const esperaMaxima = 8 * time.Second

type Salas struct {
	mu    sync.Mutex
	salas map[string]*sala
	agora func() time.Time
	// AoAssistirJunto recebe os nomes de quem viu junto, o arquivo e
	// quantos segundos. Chamado fora da trava, numa goroutine.
	AoAssistirJunto func(nomes []string, fileID int64, segundos float64)
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
		pausouEm:   s.agora(),
		contadoAte: s.ms(),
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
	s.contabilizar(sl)
	e := &espectador{nome: nome, ch: make(chan Mensagem, 64)}
	sl.vistos[e] = struct{}{}
	if len(sl.chat) > 0 {
		entregar(e, Mensagem{Tipo: "historico", Chat: append([]Linha(nil), sl.chat...)})
	}
	s.espalhar(sl)
	sair := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := sl.vistos[e]; !ok {
			return
		}
		s.contabilizar(sl)
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
	Tipo    string  `json:"tipo"` // play | pause | seek | arquivo | carregando | pronto | reacao | chat | modo | conexao | encerrar
	Posicao float64 `json:"posicao"`
	Texto   string  `json:"texto,omitempty"`
	SoDono  bool    `json:"so_dono,omitempty"`
	Dif     float64 `json:"dif,omitempty"`
	Travado bool    `json:"travado,omitempty"`
	FileID  int64   `json:"file_id,omitempty"`
	Emoji   string  `json:"emoji,omitempty"`
	Cliente string  `json:"cliente,omitempty"`
	Seq     int64   `json:"seq,omitempty"`
}

var ErrSoDono = errors.New("nesta sala só o anfitrião controla o vídeo")

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
	controle := c.Tipo == "play" || c.Tipo == "pause" || c.Tipo == "seek" || c.Tipo == "arquivo"
	if controle && e.SoDono && nome != e.Dono {
		return ErrSoDono
	}
	if c.Tipo != "conexao" && c.Tipo != "chat" && c.Tipo != "reacao" {
		s.contabilizar(sl)
	}
	if c.Tipo == "play" || c.Tipo == "pause" || c.Tipo == "seek" {
		e.Cliente, e.Seq = c.Cliente, c.Seq
	}
	switch c.Tipo {
	case "play":
		inicio := s.ms()
		if !e.Tocando && s.agora().Sub(sl.pausouEm) > pausaLonga {
			inicio += contagem
		}
		e.Tocando, e.Posicao, e.Em, e.Por = true, c.Posicao, inicio, nome
		s.desarmar(sl)
	case "pause":
		e.Tocando, e.Posicao, e.Em, e.Por = false, c.Posicao, s.ms(), nome
		sl.pausouEm = s.agora()
		s.desarmar(sl)
	case "modo":
		if nome != e.Dono {
			return errors.New("só quem abriu a sala muda o modo")
		}
		e.SoDono = c.SoDono
	case "chat":
		texto := strings.TrimSpace(c.Texto)
		if texto == "" {
			return errors.New("mensagem vazia")
		}
		if r := []rune(texto); len(r) > maxTexto {
			texto = string(r[:maxTexto])
		}
		l := Linha{De: nome, Texto: texto, Em: s.ms()}
		sl.chat = append(sl.chat, l)
		if len(sl.chat) > maxChat {
			sl.chat = sl.chat[len(sl.chat)-maxChat:]
		}
		for v := range sl.vistos {
			entregar(v, Mensagem{Tipo: "chat", Linha: &l})
		}
		return nil
	case "conexao":
		if c.Dif != c.Dif {
			c.Dif = 0
		}
		for v := range sl.vistos {
			if v.nome != nome {
				entregar(v, Mensagem{Tipo: "conexao", De: nome, Conexao: &Conexao{Dif: c.Dif, Travado: c.Travado}})
			}
		}
		return nil
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
		sl.pausouEm = s.agora()
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
		s.contabilizar(sl)
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

// contabilizar soma o tempo tocado desde a última conta para quem está
// junto (duas pessoas ou mais). Chamado antes de qualquer mudança.
func (s *Salas) contabilizar(sl *sala) {
	agora := s.ms()
	de := max(sl.contadoAte, sl.estado.Em)
	sl.contadoAte = agora
	if !sl.estado.Tocando || agora <= de || s.AoAssistirJunto == nil {
		return
	}
	nomes := s.foto(sl).Presenca
	if len(nomes) < 2 {
		return
	}
	cb, fileID, seg := s.AoAssistirJunto, sl.estado.FileID, float64(agora-de)/1000
	go cb(nomes, fileID, seg)
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
	return e.Posicao + float64(max(0, s.ms()-e.Em))/1000
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
