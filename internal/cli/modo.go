package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"nas/internal/api"
	"nas/internal/config"
	"nas/internal/lock"
)

// vigiarModo aplica o modo de nuvem do boot e recarrega a cada SIGHUP, que é
// como `nas modo` fala com o servidor já no ar.
func vigiarModo(ctx context.Context, srv *api.Server, cfg config.Config, semNuvem bool) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	aplicar := func(cfg config.Config) {
		if cfg.Modo == config.ModoHibrido {
			fmt.Println("  nuvem: conectando…")
		}
		if semNuvem && cfg.Modo == config.ModoHibrido {
			fmt.Println("  nuvem: --sem-nuvem ativo, ficando no modo local")
			return
		}
		if err := srv.RecarregarModo(ctx, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "  nuvem: não foi possível entrar no híbrido:", err)
			return
		}
		fmt.Printf("  nuvem: modo %s\n", srv.Nuvem().Modo())
	}

	if cfg.Modo == config.ModoHibrido {
		aplicar(cfg)
	}
	for {
		select {
		case <-ctx.Done():
			srv.Nuvem().Desligar()
			return
		case <-hup:
			novo, err := config.Load()
			if err != nil {
				fmt.Fprintln(os.Stderr, "  nuvem: config ilegível, mantendo o modo atual:", err)
				continue
			}
			aplicar(novo)
		}
	}
}

// cmdModo grava o modo de nuvem e avisa o servidor no ar.
//
//	nas modo                mostra o modo gravado
//	nas modo local          kill switch: corta a nuvem agora
//	nas modo hibrido        liga o híbrido
func cmdModo(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Printf("modo de nuvem: %s\n", cfg.Modo)
		if cfg.Nuvem.Configurada() {
			fmt.Printf("bucket: %s (%s)\n", cfg.Nuvem.Bucket, cfg.Nuvem.Regiao)
		} else {
			fmt.Println("nuvem não configurada (nas config set nuvem.bucket …)")
		}
		return nil
	}
	novo := args[0]
	switch novo {
	case config.ModoLocal, config.ModoHibrido:
	case "híbrido":
		novo = config.ModoHibrido
	default:
		return errors.New(`uso: nas modo local|hibrido`)
	}
	if novo == config.ModoHibrido && !cfg.Nuvem.Configurada() {
		return errors.New("nuvem não configurada: defina nuvem.bucket e nuvem.regiao com `nas config set`")
	}
	cfg.Modo = novo
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("modo de nuvem gravado: %s\n", novo)

	info, running, err := lock.Current()
	if err != nil || !running {
		fmt.Println("o NAS não está no ar; o modo vale no próximo `nas serve`.")
		return nil
	}
	proc, err := os.FindProcess(info.PID)
	if err == nil {
		err = proc.Signal(syscall.SIGHUP)
	}
	if err != nil {
		return fmt.Errorf("avisando o servidor (pid %d): %w", info.PID, err)
	}
	fmt.Printf("servidor avisado (pid %d).\n", info.PID)
	return nil
}
