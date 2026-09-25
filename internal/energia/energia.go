// Package energia diz se o Mac está numa boa hora para trabalho pesado: na
// tomada, frio e com folga. É o que decide mandar um preparo para os workers
// da nuvem (bursting) em vez de gastar bateria com ffmpeg.
package energia

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Estado é uma leitura. Desconhecido (outra plataforma, pmset ausente) conta
// como "na tomada e frio": na dúvida, o Mac trabalha, que é o modo base.
type Estado struct {
	NaBateria bool   `json:"na_bateria"`
	Carga     int    `json:"carga,omitempty"` // %, 0 = desconhecida
	Quente    bool   `json:"quente"`
	Motivo    string `json:"motivo,omitempty"`
	LidoEm    time.Time
}

// Ruim diz se vale poupar o Mac.
func (e Estado) Ruim() bool { return e.NaBateria || e.Quente }

var (
	cargaRe  = regexp.MustCompile(`(\d+)%`)
	limiteRe = regexp.MustCompile(`CPU_Speed_Limit\s*=\s*(\d+)`)
	avisoRe  = regexp.MustCompile(`(?i)thermal warning level\s*=\s*(\d+)`)
)

// InterpretarBateria lê a saída de `pmset -g batt`.
func InterpretarBateria(saida string) (naBateria bool, carga int) {
	naBateria = strings.Contains(saida, "'Battery Power'")
	if m := cargaRe.FindStringSubmatch(saida); m != nil {
		carga, _ = strconv.Atoi(m[1])
	}
	return naBateria, carga
}

// InterpretarTermica lê a saída de `pmset -g therm`. O macOS só imprime
// níveis depois que algum aviso aconteceu; "No thermal warning" é frio.
func InterpretarTermica(saida string) bool {
	if m := limiteRe.FindStringSubmatch(saida); m != nil {
		if v, _ := strconv.Atoi(m[1]); v < 100 {
			return true
		}
	}
	if m := avisoRe.FindStringSubmatch(saida); m != nil {
		if v, _ := strconv.Atoi(m[1]); v > 0 {
			return true
		}
	}
	return false
}

// Leitor guarda a última leitura: pmset custa um processo, e o playback
// consulta a cada pedido.
type Leitor struct {
	mu     sync.Mutex
	ultimo Estado
	ttl    time.Duration
	ler    func(context.Context) Estado
}

func NovoLeitor() *Leitor { return &Leitor{ttl: 30 * time.Second, ler: lerAgora} }

// Fixo devolve sempre o mesmo estado (testes, e o container da AWS).
func Fixo(e Estado) *Leitor {
	return &Leitor{ttl: time.Hour * 24 * 365, ler: func(context.Context) Estado { return e }}
}

func (l *Leitor) Estado(ctx context.Context) Estado {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Since(l.ultimo.LidoEm) < l.ttl {
		return l.ultimo
	}
	l.ultimo = l.ler(ctx)
	l.ultimo.LidoEm = time.Now()
	return l.ultimo
}

func lerAgora(ctx context.Context) Estado {
	switch runtime.GOOS {
	case "darwin":
		return lerDarwin(ctx)
	case "linux":
		return lerLinux()
	}
	return Estado{}
}

func lerDarwin(ctx context.Context) Estado {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var e Estado
	if out, err := exec.CommandContext(ctx, "pmset", "-g", "batt").Output(); err == nil {
		e.NaBateria, e.Carga = InterpretarBateria(string(out))
	}
	if out, err := exec.CommandContext(ctx, "pmset", "-g", "therm").Output(); err == nil {
		e.Quente = InterpretarTermica(string(out))
	}
	e.Motivo = motivo(e)
	return e
}

// lerLinux cobre um notebook Linux; num servidor sem bateria, nada é lido.
func lerLinux() Estado {
	var e Estado
	fontes, _ := filepath.Glob("/sys/class/power_supply/*/online")
	for _, f := range fontes {
		if b, err := os.ReadFile(f); err == nil && strings.TrimSpace(string(b)) == "0" &&
			strings.Contains(strings.ToLower(f), "ac") {
			e.NaBateria = true
		}
	}
	e.Motivo = motivo(e)
	return e
}

func motivo(e Estado) string {
	switch {
	case e.NaBateria && e.Quente:
		return "na bateria e quente"
	case e.NaBateria:
		return "na bateria"
	case e.Quente:
		return "quente"
	}
	return ""
}
