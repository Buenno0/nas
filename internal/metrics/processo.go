// Package metrics coleta a saúde do próprio servidor: quanto de CPU e memória
// o processo consome e quanto tempo cada rota HTTP leva para responder.
//
// Tudo vive em memória e reseta a cada `nas serve`, no mesmo espírito do
// progresso de scan (internal/api/scan.go) e do placar das ruínas: é um painel
// para olhar agora, não uma série histórica.
package metrics

import (
	"runtime"
	"sync"
	"syscall"
	"time"
)

// ProcessoAmostra é uma leitura instantânea do processo do NAS.
//
// Memória aparece em dois números porque nenhum dos dois conta a história
// sozinho: HeapBytes é o que o Go tem alocado AGORA (sobe e desce), e
// RSSPicoBytes é o pico de residência desde o boot — que nunca desce, e por
// isso seria enganoso mostrar como "uso atual".
type ProcessoAmostra struct {
	CPUPercent   float64 `json:"cpu_percent"` // 0–100 sobre a máquina inteira
	CPUNucleos   float64 `json:"cpu_nucleos"` // núcleos consumidos, ex. 1.6
	HeapBytes    uint64  `json:"heap_bytes"`
	RSSPicoBytes uint64  `json:"rss_pico_bytes"`
	Goroutines   int     `json:"goroutines"`
	Nucleos      int     `json:"nucleos"` // runtime.NumCPU
}

// AmostradorProcesso guarda a leitura anterior de CPU-time. É preciso: o
// sistema só oferece tempo de CPU ACUMULADO, então a porcentagem instantânea
// sai da diferença entre duas leituras espaçadas no tempo.
//
// Justamente por manter estado, deve existir um único amostrador por processo e
// um único chamador de Amostra — dois clientes lendo em paralelo dividiriam a
// mesma janela de delta e ambos veriam metade do valor real.
type AmostradorProcesso struct {
	mu       sync.Mutex
	ultCPU   time.Duration
	ultRelog time.Time
}

func NovoAmostradorProcesso() *AmostradorProcesso { return &AmostradorProcesso{} }

// rssEmBytes converte o Maxrss do getrusage, cuja unidade muda por sistema:
// bytes no macOS/BSD, kilobytes no Linux. Hoje o NAS só roda em macOS, mas
// errar isso silenciosamente daria um número 1024× errado numa eventual porta.
func rssEmBytes(maxrss int64) uint64 {
	if maxrss < 0 {
		return 0
	}
	if runtime.GOOS == "linux" {
		return uint64(maxrss) * 1024
	}
	return uint64(maxrss)
}

func tempoDe(tv syscall.Timeval) time.Duration {
	return time.Duration(tv.Sec)*time.Second + time.Duration(tv.Usec)*time.Microsecond
}

// Amostra lê CPU, memória e goroutines. A primeira chamada devolve CPU 0: sem
// leitura anterior não existe delta, e inventar um número seria pior que zero.
func (a *AmostradorProcesso) Amostra() ProcessoAmostra {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	am := ProcessoAmostra{
		HeapBytes:  mem.HeapAlloc,
		Goroutines: runtime.NumGoroutine(),
		Nucleos:    runtime.NumCPU(),
	}

	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return am
	}
	am.RSSPicoBytes = rssEmBytes(int64(ru.Maxrss))

	cpu := tempoDe(ru.Utime) + tempoDe(ru.Stime)
	agora := time.Now()

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.ultRelog.IsZero() {
		parede := agora.Sub(a.ultRelog)
		gasto := cpu - a.ultCPU
		if parede > 0 && gasto >= 0 {
			am.CPUNucleos = gasto.Seconds() / parede.Seconds()
			am.CPUPercent = am.CPUNucleos / float64(am.Nucleos) * 100
			// Arredondamento de relógio pode gerar 100.4%; um painel mostrando
			// isso perde credibilidade antes de informar qualquer coisa.
			if am.CPUPercent > 100 {
				am.CPUPercent = 100
			}
		}
	}
	a.ultCPU = cpu
	a.ultRelog = agora

	return am
}
