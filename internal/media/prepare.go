// Preparo de arquivos para o navegador: remux, conserto de áudio ou
// recodificação de imagem, sempre para um MP4 completo em cache.
//
// A saída é um arquivo inteiro com o índice na frente (`-movflags +faststart`),
// servido depois pelo mesmo http.ServeContent do direct play. Isso dá Range,
// seek e retomada de graça, e mantém a linha de tempo em segundos do original —
// o progresso salvo continua valendo entre o arquivo cru e o preparado.
package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Estado de um preparo.
type Estado string

const (
	EstadoAusente     Estado = "ausente" // nunca preparado
	EstadoFila        Estado = "fila"    // pedido, esperando vaga
	EstadoTrabalhando Estado = "trabalhando"
	EstadoPronto      Estado = "pronto"
	EstadoErro        Estado = "erro"
)

// Progresso é o que a interface mostra durante o preparo.
type Progresso struct {
	Estado     Estado  `json:"estado"`
	Receita    string  `json:"receita,omitempty"`
	Segundos   float64 `json:"segundos_prontos"`
	Total      float64 `json:"segundos_total"`
	Percentual int     `json:"percentual"`
	Velocidade float64 `json:"velocidade"` // vezes o tempo real
	Restante   int     `json:"restante_segundos"`
	Erro       string  `json:"erro,omitempty"`
	Arquivo    string  `json:"-"`
}

// Pedido identifica um preparo. A chave de cache inclui a receita e a versão,
// para uma mudança de receita não servir arquivo velho.
type Pedido struct {
	FileID  int64
	Origem  string
	MTime   int64
	Duracao float64
	Receita string
	// Audio é o índice absoluto da faixa escolhida; -1 usa a padrão do arquivo.
	Audio int
	// Identidade substitui Origem na chave do cache quando a origem é uma URL
	// assinada da nuvem, que muda a cada assinatura (e pode nem estar
	// preenchida numa consulta). Tamanho substitui o stat da origem na conta
	// de espaço.
	Identidade string
	Tamanho    int64
}

func (p Pedido) identidade() string {
	if p.Identidade != "" {
		return p.Identidade
	}
	return p.Origem
}

// nome é o que aparece nos logs: o arquivo, não a URL assinada inteira.
func (p Pedido) nome() string {
	if p.Identidade != "" {
		return filepath.Base(p.Identidade)
	}
	return filepath.Base(p.Origem)
}

// versaoDasReceitas invalida o cache inteiro quando as receitas mudam. Foi para
// 2 quando as receitas passaram a aceitar faixa de áudio: um preparo da versão
// 1 é sempre da faixa padrão, e reaproveitá-lo entregaria o áudio errado a quem
// pediu a outra.
const versaoDasReceitas = 2

// Chave é o nome estável do arquivo em cache.
//
// A faixa entra na chave: sem ela, pedir o áudio original devolveria o dublado
// que já estava em cache, silenciosamente e para sempre.
func (p Pedido) Chave() string {
	return fmt.Sprintf("%s_v%d_%s.mp4",
		p.Receita, versaoDasReceitas,
		hashCurto(fmt.Sprintf("%s|%d|a%d", p.identidade(), p.MTime, p.Audio)))
}

// Preparador executa e acompanha os preparos. Um por servidor.
type Preparador struct {
	dir       string
	limite    int64 // orçamento de disco em bytes
	reservado int64 // espaço livre que nunca deve ser consumido

	mu        sync.Mutex
	trabalhos map[string]*trabalho
	vagas     chan struct{}
}

type trabalho struct {
	pedido    Pedido
	progresso Progresso
	pronto    chan struct{}
	cancelar  context.CancelFunc
}

// NovoPreparador cria o executor. paralelo limita quantos ffmpeg rodam ao mesmo
// tempo: o motor de hardware da Apple tem vazão fixa (~9x tempo real no total),
// então mais processos só dividem a mesma banda.
func NovoPreparador(dir string, limiteBytes, reservadoBytes int64, paralelo int) *Preparador {
	if paralelo < 1 {
		paralelo = 1
	}
	p := &Preparador{
		dir:       dir,
		limite:    limiteBytes,
		reservado: reservadoBytes,
		trabalhos: map[string]*trabalho{},
		vagas:     make(chan struct{}, paralelo),
	}
	for range paralelo {
		p.vagas <- struct{}{}
	}
	return p
}

// Consultar diz em que pé está o preparo, sem começar nada.
func (p *Preparador) Consultar(pedido Pedido) Progresso {
	chave := pedido.Chave()

	p.mu.Lock()
	t, emAndamento := p.trabalhos[chave]
	p.mu.Unlock()
	if emAndamento {
		t2 := *t
		return t2.progresso
	}

	destino := filepath.Join(p.dir, chave)
	if info, err := os.Stat(destino); err == nil && info.Size() > 0 {
		p.tocar(destino, info)
		return Progresso{Estado: EstadoPronto, Percentual: 100, Arquivo: destino, Receita: pedido.Receita}
	}
	return Progresso{Estado: EstadoAusente, Receita: pedido.Receita}
}

// folgaDoToque é a granularidade do atime do cache. Consultar é chamado com
// muito mais frequência do que parece — uma vez por carregamento de página, uma
// vez por segundo enquanto o SSE de preparo está aberto e UMA VEZ POR
// REQUISIÇÃO RANGE, ou seja, dezenas de vezes por minuto durante uma
// reprodução com seek.
//
// Escrever o atime em toda uma dessas chamadas custa uma escrita de metadados
// no disco por requisição, para nada: o cache é podado por "usado há mais
// tempo", e a diferença entre "usado agora" e "usado há dez minutos" jamais
// muda quem é o mais antigo de um cache cujos arquivos vivem horas ou dias.
const folgaDoToque = 10 * time.Minute

// tocar atualiza o atime do arquivo em cache, que é o critério de remoção.
// Pula quando o atime já está recente — o os.Stat que o chamador já fez
// entrega o valor, então a checagem não custa nenhuma syscall extra.
func (p *Preparador) tocar(destino string, info os.FileInfo) {
	if time.Since(time.Unix(atime(info), 0)) < folgaDoToque {
		return
	}
	_ = os.Chtimes(destino, time.Now(), info.ModTime())
}

// Pedir garante que o preparo esteja em andamento e devolve o estado atual.
// Chamadas repetidas para o mesmo arquivo compartilham um único ffmpeg.
func (p *Preparador) Pedir(ctx context.Context, pedido Pedido) (Progresso, error) {
	if atual := p.Consultar(pedido); atual.Estado == EstadoPronto {
		return atual, nil
	}
	if _, err := FFmpegPath(); err != nil {
		return Progresso{Estado: EstadoErro, Erro: "ffmpeg não instalado"}, err
	}
	if err := p.cabeNoDisco(pedido); err != nil {
		return Progresso{Estado: EstadoErro, Erro: err.Error()}, err
	}

	chave := pedido.Chave()
	p.mu.Lock()
	if t, existe := p.trabalhos[chave]; existe {
		prog := t.progresso
		p.mu.Unlock()
		return prog, nil
	}

	// O contexto do trabalho é independente da requisição HTTP que o pediu (o
	// navegador desiste, o preparo continua), mas NÃO é imortal: nasce do
	// contexto do servidor, então `nas stop` mata o ffmpeg junto. Era o vazamento
	// que o mapa do backend apontou no padrão do scan.
	ctxTrabalho, cancelar := context.WithCancel(ctx)
	t := &trabalho{
		pedido:   pedido,
		pronto:   make(chan struct{}),
		cancelar: cancelar,
		progresso: Progresso{
			Estado:  EstadoFila,
			Receita: pedido.Receita,
			Total:   pedido.Duracao,
		},
	}
	p.trabalhos[chave] = t
	p.mu.Unlock()

	go p.executar(ctxTrabalho, t)
	return t.progresso, nil
}

// Esperar bloqueia até o preparo terminar (ou o contexto morrer).
func (p *Preparador) Esperar(ctx context.Context, pedido Pedido) (Progresso, error) {
	chave := pedido.Chave()
	p.mu.Lock()
	t := p.trabalhos[chave]
	p.mu.Unlock()
	if t == nil {
		return p.Consultar(pedido), nil
	}
	select {
	case <-ctx.Done():
		return p.Consultar(pedido), ctx.Err()
	case <-t.pronto:
		return p.Consultar(pedido), nil
	}
}

func (p *Preparador) atualizar(t *trabalho, f func(*Progresso)) {
	p.mu.Lock()
	f(&t.progresso)
	p.mu.Unlock()
}

func (p *Preparador) executar(ctx context.Context, t *trabalho) {
	chave := t.pedido.Chave()
	defer func() {
		close(t.pronto)
		p.mu.Lock()
		delete(p.trabalhos, chave)
		p.mu.Unlock()
	}()

	// Espera vaga entre os processos simultâneos.
	select {
	case <-ctx.Done():
		return
	case <-p.vagas:
	}
	defer func() { p.vagas <- struct{}{} }()

	p.atualizar(t, func(pr *Progresso) { pr.Estado = EstadoTrabalhando })

	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		p.falhar(t, err)
		return
	}

	// Escreve em temporário: um preparo interrompido nunca deixa MP4 truncado
	// no cache passando por pronto.
	temp, err := os.CreateTemp(p.dir, "preparo-*.mp4")
	if err != nil {
		p.falhar(t, err)
		return
	}
	tempNome := temp.Name()
	temp.Close()
	defer os.Remove(tempNome)

	args, err := argumentosDaReceita(t.pedido.Receita, t.pedido.Origem, tempNome, t.pedido.Audio)
	if err != nil {
		p.falhar(t, err)
		return
	}

	bin, err := FFmpegPath()
	if err != nil {
		p.falhar(t, err)
		return
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	// Interrupt em vez de Kill: o ffmpeg fecha o arquivo e sai; e WaitDelay
	// garante que um processo teimoso morra de qualquer forma.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second

	saida, err := cmd.StdoutPipe()
	if err != nil {
		p.falhar(t, err)
		return
	}
	var erroDoFFmpeg strings.Builder
	cmd.Stderr = &erroDoFFmpeg

	inicio := time.Now()
	if err := cmd.Start(); err != nil {
		p.falhar(t, err)
		return
	}
	go p.lerProgresso(t, saida, inicio)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			log.Printf("preparo de %s cancelado", t.pedido.nome())
			return
		}
		p.falhar(t, fmt.Errorf("ffmpeg: %s", primeiraLinha(erroDoFFmpeg.String())))
		return
	}

	if info, err := os.Stat(tempNome); err != nil || info.Size() == 0 {
		p.falhar(t, errors.New("ffmpeg terminou sem escrever nada"))
		return
	}
	if err := os.Rename(tempNome, filepath.Join(p.dir, chave)); err != nil {
		p.falhar(t, err)
		return
	}

	decorrido := time.Since(inicio)
	log.Printf("preparo pronto: %s (%s) em %s",
		t.pedido.nome(), NomeDaReceita(t.pedido.Receita), decorrido.Round(time.Second))
	p.atualizar(t, func(pr *Progresso) {
		pr.Estado = EstadoPronto
		pr.Percentual = 100
	})
	p.limparExcedente()
}

// lerProgresso traduz a saída de `-progress pipe:1` em percentual e ETA.
func (p *Preparador) lerProgresso(t *trabalho, saida io.Reader, inicio time.Time) {
	scanner := bufio.NewScanner(saida)
	for scanner.Scan() {
		linha := scanner.Text()
		chave, valor, achou := strings.Cut(linha, "=")
		if !achou {
			continue
		}
		switch chave {
		case "out_time_ms", "out_time_us":
			micros, err := strconv.ParseFloat(valor, 64)
			if err != nil {
				continue
			}
			segundos := micros / 1_000_000
			p.atualizar(t, func(pr *Progresso) {
				pr.Segundos = segundos
				if pr.Total > 0 {
					pr.Percentual = int(segundos / pr.Total * 100)
					if pr.Percentual > 99 {
						pr.Percentual = 99 // 100 só quando o rename acontece
					}
					decorrido := time.Since(inicio).Seconds()
					if decorrido > 0.5 && segundos > 0 {
						pr.Velocidade = segundos / decorrido
						restante := (pr.Total - segundos) / pr.Velocidade
						pr.Restante = int(restante)
					}
				}
			})
		}
	}
}

func (p *Preparador) falhar(t *trabalho, err error) {
	log.Printf("preparo de %s falhou: %v", t.pedido.nome(), err)
	p.atualizar(t, func(pr *Progresso) {
		pr.Estado = EstadoErro
		pr.Erro = err.Error()
	})
}

func primeiraLinha(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	if s == "" {
		return "erro sem mensagem"
	}
	return s
}

// Ocupado diz se todas as vagas de ffmpeg estão em uso: um pedido novo iria
// para a fila. É um dos sinais para mandar o preparo para a nuvem.
func (p *Preparador) Ocupado() bool { return len(p.vagas) == 0 }
