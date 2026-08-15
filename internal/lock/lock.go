// Package lock garante que só exista um NAS no ar por vez — o requisito de
// "local OU tunnel, nunca os dois".
package lock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"nas/internal/config"
)

// Info é o conteúdo de ~/.nas/nas.pid.
type Info struct {
	PID     int       `json:"pid"`
	Mode    string    `json:"mode"` // local | tunnel
	Port    int       `json:"port"`
	URL     string    `json:"url,omitempty"`
	Started time.Time `json:"started"`
}

// ErrRunning é devolvido quando já há uma instância viva.
type ErrRunning struct{ Info Info }

func (e ErrRunning) Error() string {
	return fmt.Sprintf("o NAS já está rodando em modo %s (pid %d, porta %d). Use `nas stop` para encerrar",
		e.Info.Mode, e.Info.PID, e.Info.Port)
}

type Lock struct {
	path string
	info Info
}

// Acquire cria o lock. Se o arquivo existir mas o processo tiver morrido
// (queda de energia, kill -9), o lock órfão é substituído.
func Acquire(mode string, port int) (*Lock, error) {
	path, err := config.PIDPath()
	if err != nil {
		return nil, err
	}
	if _, err := config.EnsureDir(); err != nil {
		return nil, err
	}

	if info, alive, err := read(path); err == nil && alive {
		return nil, ErrRunning{Info: info}
	}

	l := &Lock{
		path: path,
		info: Info{PID: os.Getpid(), Mode: mode, Port: port, Started: time.Now()},
	}
	if err := l.write(); err != nil {
		return nil, err
	}
	return l, nil
}

// SetURL registra a URL pública depois que o tunnel sobe.
func (l *Lock) SetURL(url string) error {
	l.info.URL = url
	return l.write()
}

func (l *Lock) write() error {
	data, err := json.MarshalIndent(l.info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.path, append(data, '\n'), 0o600)
}

// Release remove o lock. Seguro de chamar mais de uma vez.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	err := os.Remove(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Current devolve a instância viva, se houver.
func Current() (Info, bool, error) {
	path, err := config.PIDPath()
	if err != nil {
		return Info{}, false, err
	}
	return read(path)
}

func read(path string) (Info, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, false, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, false, err
	}
	return info, alive(info.PID), nil
}

// alive testa o processo com o sinal 0, que só verifica a existência.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// Stop pede o encerramento gracioso da instância em execução.
func Stop() (Info, error) {
	info, running, err := Current()
	if err != nil {
		return Info{}, errors.New("nenhuma instância em execução")
	}
	if !running {
		path, _ := config.PIDPath()
		os.Remove(path) // lock órfão
		return Info{}, errors.New("nenhuma instância em execução (lock órfão removido)")
	}

	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return Info{}, err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return Info{}, fmt.Errorf("encerrando pid %d: %w", info.PID, err)
	}
	return info, nil
}
