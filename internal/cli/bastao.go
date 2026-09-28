package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"nas/internal/bastao"
	"nas/internal/cloud"
)

// euNaNuvem identifica esta task. O hostname da Fargate é único por task e
// igual para os comandos que o script de entrada roda nela.
func euNaNuvem() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "desconhecida"
	}
	return h
}

func bucketDoBastao(ctx context.Context) (bastao.Bucket, error) {
	cfg := nuvemDoAmbiente()
	if !cfg.Configurada() {
		return nil, errors.New("defina NAS_BUCKET e NAS_REGIAO")
	}
	return cloud.ConectarDireto(ctx, cfg)
}

// cmdBastao: nas bastao esperar | liberar (usados pelo script de entrada da
// instância cloud, antes e depois do Litestream).
func cmdBastao(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("uso: nas bastao esperar|liberar")
	}
	b, err := bucketDoBastao(ctx)
	if err != nil {
		return err
	}
	switch args[0] {
	case "esperar":
		return bastao.Esperar(ctx, b, euNaNuvem(), time.Now)
	case "liberar":
		// O contexto do sinal pode já estar cancelado (a task parando): a
		// liberação precisa sair mesmo assim.
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return bastao.Liberar(c, b, euNaNuvem(), time.Now)
	}
	return fmt.Errorf("bastao: ação desconhecida %q", args[0])
}

// cmdSaude é o health check do container: 0 se o servidor local responde.
func cmdSaude() error {
	porta := os.Getenv("NAS_PORTA")
	if porta == "" {
		porta = "8787"
	}
	c := http.Client{Timeout: 3 * time.Second}
	r, err := c.Get("http://127.0.0.1:" + porta + "/healthz")
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz respondeu %d", r.StatusCode)
	}
	return nil
}
