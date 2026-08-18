package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"nas/internal/api"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/lock"
	"nas/internal/tunnel"
)

type mode string

const (
	modeLocal  mode = "local"
	modeTunnel mode = "tunnel"
)

func cmdServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	local := fs.Bool("local", false, "servir na rede local (LAN)")
	tunnelFlag := fs.Bool("tunnel", false, "servir por um tunnel Cloudflare")
	name := fs.String("tunnel-name", "", "tunnel nomeado do cloudflared (padrão: quick tunnel)")
	port := fs.Int("port", 0, "porta (sobrescreve a configuração)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *local && *tunnelFlag {
		return errors.New("--local e --tunnel são mutuamente exclusivos: escolha um")
	}
	m := modeLocal
	if *tunnelFlag {
		m = modeTunnel
	}
	return serve(ctx, m, *port, *name)
}

func serve(ctx context.Context, m mode, portOverride int, tunnelName string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if portOverride > 0 {
		cfg.Port = portOverride
	}
	if tunnelName == "" {
		tunnelName = cfg.Tunnel
	}

	// Um modo por vez: o lock é o que garante isso de verdade, inclusive
	// contra dois terminais abertos.
	l, err := lock.Acquire(string(m), cfg.Port)
	if err != nil {
		return err
	}
	defer l.Release()

	if m == modeTunnel {
		if !tunnel.Available() {
			return tunnel.ErrNotInstalled
		}
		if tunnelName != "" && !tunnel.HasNamedTunnel(tunnelName) {
			return fmt.Errorf("tunnel nomeado %q sem credencial em ~/.cloudflared. "+
				"Rode `cloudflared login` e `cloudflared tunnel create %s`, ou use o quick tunnel com `nas config set tunnel \"\"`",
				tunnelName, tunnelName)
		}
	}

	// No modo tunnel o Go escuta só no loopback: o cloudflared é a única porta
	// de entrada.
	//
	// No modo local o endereço fica sem host — ":8787" abre IPv4 e IPv6 ao
	// mesmo tempo. Isso importa: o nome .local da máquina costuma resolver
	// para um endereço IPv6 no macOS, então ouvir só em 0.0.0.0 fazia o acesso
	// por IP funcionar e o por nome falhar.
	addr := fmt.Sprintf(":%d", cfg.Port)
	if m == modeTunnel {
		addr = fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	}

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	srv := api.New(cfg, database, api.Options{
		SecureCookies: m == modeTunnel,
		TrustProxy:    m == modeTunnel,
	})

	// Primeiro boot: cria o usuário e mostra a senha uma única vez.
	user, password, created, err := srv.Auth().EnsureInitialUser(ctx)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("\n  ┌─ primeiro acesso ────────────────────────────\n")
		fmt.Printf("  │  usuário: %s\n", user)
		fmt.Printf("  │  senha:   %s\n", password)
		fmt.Printf("  │  anote agora: ela não será mostrada de novo.\n")
		fmt.Printf("  └──────────────────────────────────────────────\n")
	}

	fmt.Printf("\n  Ozymandias %s — modo %s\n", Version, m)
	if m == modeLocal {
		fmt.Printf("  http://localhost:%d\n", cfg.Port)
		// O nome .local não muda quando o roteador troca o IP, então é o
		// endereço que vale anotar. O QR fica com o IP, que qualquer aparelho
		// resolve mesmo sem mDNS.
		if nome := MDNSName(); nome != "" {
			fmt.Printf("  http://%s:%d  (nome fixo na rede)\n", nome, cfg.Port)
		}
		announceShareURL("endereço na rede local", fmt.Sprintf("http://%s:%d", LANIP(), cfg.Port))
	} else {
		fmt.Printf("  http://127.0.0.1:%d  (só nesta máquina)\n", cfg.Port)
		go runTunnel(ctx, cfg.Port, tunnelName, l)
	}
	fmt.Println("\n  Ctrl+C para parar.")

	err = srv.Serve(ctx, addr)
	fmt.Println("\n  encerrado.")
	return err
}

func runTunnel(ctx context.Context, port int, name string, l *lock.Lock) {
	err := tunnel.Run(ctx, tunnel.Options{
		Port: port,
		Name: name,
		OnURL: func(url string) {
			if url == "" {
				fmt.Printf("  tunnel nomeado %q no ar — use o domínio configurado na Cloudflare\n", name)
				return
			}
			if err := l.SetURL(url); err != nil {
				fmt.Fprintln(os.Stderr, "aviso: não consegui registrar a URL no lock:", err)
			}
			announceShareURL("URL pública ativa; ela muda a cada quick tunnel", url)
			fmt.Println()
		},
		OnState: func(state string) {
			fmt.Printf("  tunnel: %s\n", state)
		},
	})
	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "erro no tunnel:", err)
	}
}

func cmdStatus() error {
	info, running, err := lock.Current()
	if err != nil || !running {
		fmt.Println("o NAS não está rodando.")
		return nil
	}
	fmt.Printf("rodando em modo %s desde %s\n", info.Mode, info.Started.Format("02/01 15:04"))
	fmt.Printf("pid %d · porta %d\n", info.PID, info.Port)
	if info.URL != "" {
		fmt.Printf("url pública: %s\n", info.URL)
	}
	return nil
}

func cmdStop() error {
	info, err := lock.Stop()
	if err != nil {
		return err
	}
	fmt.Printf("encerrando o NAS (pid %d, modo %s)…\n", info.PID, info.Mode)
	return nil
}

func runMenu(ctx context.Context) int {
	fmt.Printf("\n  Ozymandias %s\n\n", Version)

	if info, running, err := lock.Current(); err == nil && running {
		fmt.Printf("  Já existe uma instância no ar (modo %s, pid %d).\n", info.Mode, info.PID)
		fmt.Println("  Use `nas stop` para encerrar antes de trocar de modo.")
		return 1
	}

	fmt.Println("  1) Local    → acessível na sua rede")
	fmt.Println("  2) Tunnel   → acessível pela internet (Cloudflare)")
	fmt.Println("  q) Sair")
	fmt.Print("\n  Escolha: ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nentrada encerrada")
		return 1
	}

	var m mode
	switch strings.TrimSpace(line) {
	case "1":
		m = modeLocal
	case "2":
		m = modeTunnel
	case "q", "Q", "":
		return 0
	default:
		fmt.Fprintln(os.Stderr, "opção inválida")
		return 2
	}

	if err := serve(ctx, m, 0, ""); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	return 0
}
