// Package tunnel supervisiona o cloudflared, que publica o NAS na internet.
//
// Dois modos: quick tunnel (URL aleatória em trycloudflare.com, sem conta) e
// tunnel nomeado (URL fixa no seu domínio, exige `cloudflared login` antes).
package tunnel

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
	"regexp"
	"strings"
	"sync"
	"time"
)

var ErrNotInstalled = errors.New("cloudflared não encontrado: instale com `brew install cloudflared`")

// quickURLRe casa a URL que o cloudflared imprime ao abrir um quick tunnel.
var quickURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

type Options struct {
	Port int
	// Name vazio = quick tunnel. Preenchido = `cloudflared tunnel run <name>`.
	Name string
	// OnURL é chamado quando a URL pública fica conhecida.
	OnURL func(string)
	// OnState reporta mudanças ("conectando", "no ar", "reconectando").
	OnState func(string)
}

// Available diz se o cloudflared está instalado.
func Available() bool {
	_, err := exec.LookPath("cloudflared")
	return err == nil
}

// HasNamedTunnel verifica se existe credencial local para um tunnel nomeado.
func HasNamedTunnel(name string) bool {
	if name == "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	// O cloudflared guarda <uuid>.json; o nome também vale como arquivo de
	// configuração próprio.
	for _, candidate := range []string{name + ".json", "config.yml", "config.yaml"} {
		if _, err := os.Stat(filepath.Join(home, ".cloudflared", candidate)); err == nil {
			return true
		}
	}
	return false
}

// Run mantém o cloudflared vivo até o contexto ser cancelado. Se ele cair,
// sobe de novo com backoff — a internet de casa cai, e o NAS não pode sumir
// junto de forma permanente.
func Run(ctx context.Context, opts Options) error {
	bin, err := exec.LookPath("cloudflared")
	if err != nil {
		return ErrNotInstalled
	}

	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		start := time.Now()
		err := runOnce(ctx, bin, opts)
		if ctx.Err() != nil {
			return nil // encerramento pedido por nós
		}

		// Uptime decente significa que o problema foi pontual: recomeça rápido.
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		if opts.OnState != nil {
			opts.OnState(fmt.Sprintf("cloudflared caiu (%v); reconectando em %s", err, backoff))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func runOnce(ctx context.Context, bin string, opts Options) error {
	args := []string{"tunnel", "--no-autoupdate", "--url", fmt.Sprintf("http://127.0.0.1:%d", opts.Port)}
	if opts.Name != "" {
		args = append(args, "run", opts.Name)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	// O processo filho tem que morrer junto com o NAS, sem virar órfão.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iniciando cloudflared: %w", err)
	}

	if opts.OnState != nil {
		opts.OnState("conectando à Cloudflare…")
	}

	var once sync.Once
	report := func(url string) {
		once.Do(func() {
			if opts.OnURL != nil {
				opts.OnURL(url)
			}
		})
	}

	var wg sync.WaitGroup
	// O cloudflared escreve o essencial no stderr, mas lemos os dois.
	for _, pipe := range []io.Reader{stdout, stderr} {
		wg.Add(1)
		go func(r io.Reader) {
			defer wg.Done()
			scanner := bufio.NewScanner(r)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				line := scanner.Text()
				if url := quickURLRe.FindString(line); url != "" {
					report(url)
				}
				if strings.Contains(line, "ERR") || strings.Contains(line, "error") {
					log.Printf("cloudflared: %s", strings.TrimSpace(line))
				}
			}
		}(pipe)
	}

	// Tunnel nomeado não imprime URL: o endereço é o domínio configurado.
	if opts.Name != "" {
		report("")
	}

	wg.Wait()
	return cmd.Wait()
}
