package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"nas/internal/api"
	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
)

// nuvemDoAmbiente lê a configuração de nuvem das variáveis que a task
// definition do ECS injeta. Worker e instância cloud usam as mesmas.
func nuvemDoAmbiente() config.Nuvem {
	endpoint := os.Getenv("NAS_ENDPOINT")
	return config.Nuvem{
		Bucket:       os.Getenv("NAS_BUCKET"),
		Regiao:       os.Getenv("NAS_REGIAO"),
		Endpoint:     endpoint,
		PathStyle:    endpoint != "",
		Prefixo:      strings.Trim(os.Getenv("NAS_PREFIXO"), "/"),
		CDNDominio:   os.Getenv("NAS_CDN_DOMINIO"),
		CDNChaveID:   os.Getenv("NAS_CDN_CHAVE_ID"),
		CDNParametro: os.Getenv("NAS_CDN_PARAMETRO"),
		FilaJobs:     os.Getenv("NAS_FILA_JOBS"),
		FilaEventos:  os.Getenv("NAS_FILA_EVENTOS"),
		Topico:       os.Getenv("NAS_TOPICO_CATALOGO"),
		Worker:       os.Getenv("NAS_WORKER"),
		Aceleracao:   os.Getenv("NAS_ACELERACAO") == "true",
	}
}

// serveNuvem é a instância cloud do Ozymandias (nas serve --nuvem).
//
// Ela serve a mesma interface quando o Mac está dormindo: mostra a união dos
// catálogos, toca o que tem cópia no bucket, recebe uploads e registra
// progresso. Não tem disco de mídia nem é dona de usuários: tudo isso chega
// do Mac pelo snapshot. Escuta só no loopback; a porta de entrada é o sidecar
// do Tailscale (HTTPS em nuvem.<tailnet>.ts.net, Funnel para quem não tem o
// app). O banco é replicado para o bucket pelo Litestream, que embrulha este
// processo no container.
func serveNuvem(ctx context.Context, portOverride int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Nuvem = nuvemDoAmbiente()
	cfg.Nuvem.Papel = config.PapelNuvem
	cfg.Modo = config.ModoHibrido
	// Chave do TMDB (SSM → secret do ECS): capa na hora para o que for
	// enviado por aqui. Sem ela, a capa chega depois, pelo snapshot do Mac.
	if k := strings.TrimSpace(os.Getenv("NAS_TMDB_KEY")); k != "" {
		cfg.TMDBKey = k
	}
	// 0,5 vCPU não transcodifica nada: preparo de item da nuvem é do worker.
	cfg.Transcode = false
	if p, err := strconv.Atoi(os.Getenv("NAS_PORTA")); err == nil && p > 0 {
		cfg.Port = p
	}
	if portOverride > 0 {
		cfg.Port = portOverride
	}
	if !cfg.Nuvem.Configurada() {
		return errors.New("defina NAS_BUCKET e NAS_REGIAO")
	}

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	var srv *api.Server
	chave := cloud.Nova(func() config.Nuvem { return srv.ConfigNuvem() }, false)
	srv = api.New(cfg, database, api.Options{
		SecureCookies: true, // o Tailscale termina o HTTPS
		TrustProxy:    true,
		Nuvem:         chave,
		NaNuvem:       true,
	})

	// Na nuvem a nuvem está sempre ligada; se a primeira conexão falhar (a
	// rede da task ainda subindo, credencial atrasada), tenta de novo.
	go func() {
		espera := 5 * time.Second
		for ctx.Err() == nil {
			if err := chave.Ativar(ctx); err == nil {
				log.Println("nuvem: conectado")
				return
			} else {
				log.Printf("nuvem: conectando (%v); nova tentativa em %s", err, espera)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(espera):
			}
			espera = min(espera*2, 2*time.Minute)
		}
	}()
	defer chave.Desligar()

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	fmt.Printf("  Ozymandias %s — instância cloud em %s\n", Version, addr)
	return srv.Serve(ctx, addr)
}
