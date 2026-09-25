package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/worker"
)

// cmdWorker é o processo do container na AWS. Tudo vem do ambiente (task
// definition do ECS); as credenciais, da task role.
//
//	NAS_BUCKET, NAS_REGIAO           bucket de mídia
//	NAS_FILA_JOBS                    URL da fila SQS de jobs
//	NAS_TOPICO_CATALOGO              ARN do tópico SNS catalogo
//	NAS_ENDPOINT, NAS_PREFIXO        opcionais (MinIO, bucket compartilhado)
//	NAS_TRABALHO                     área de trabalho (padrão: $TMPDIR)
func cmdWorker(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	umaVez := fs.Bool("uma-vez", false, "sai quando a fila esvaziar")
	processar := fs.String("processar", "", "processa só esta chave do bucket e sai (sem fila)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	nuvem := nuvemDoAmbiente()
	cfg := worker.Config{
		FilaJobs: os.Getenv("NAS_FILA_JOBS"),
		Topico:   os.Getenv("NAS_TOPICO_CATALOGO"),
		Dir:      os.Getenv("NAS_TRABALHO"),
		UmaVez:   *umaVez,
	}
	if cfg.Dir == "" {
		cfg.Dir = os.TempDir()
	}
	if !nuvem.Configurada() {
		return errors.New("defina NAS_BUCKET e NAS_REGIAO")
	}

	// Na AWS não há kill switch a respeitar: a chave só existe porque é ela
	// que monta o adapter com o cliente HTTP certo.
	chave := cloud.Nova(func() config.Nuvem { return nuvem }, false)
	if err := chave.Ativar(ctx); err != nil {
		return err
	}
	arm, _, _ := chave.Hibrido()

	if *processar != "" {
		man, err := worker.Processar(ctx, arm, cfg.Dir, *processar)
		if err != nil {
			return err
		}
		for _, d := range man.Derivados {
			fmt.Printf("%-8s %s\n", d.Tipo, d.Key)
		}
		return nil
	}
	if cfg.FilaJobs == "" || cfg.Topico == "" {
		return errors.New("defina NAS_FILA_JOBS e NAS_TOPICO_CATALOGO")
	}
	fmt.Printf("worker no ar: fila %s\n", cfg.FilaJobs)
	return worker.Rodar(ctx, arm, cfg)
}
