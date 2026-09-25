package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"nas/internal/config"
)

func cmdConfig(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(args) == 0 || args[0] == "show" {
		path, _ := config.FilePath()
		fmt.Printf("arquivo:    %s\n", path)
		fmt.Printf("porta:      %d\n", cfg.Port)
		fmt.Printf("tmdb_key:   %s\n", maskKey(cfg.TMDBKey))
		fmt.Printf("tmdb_lang:  %s\n", cfg.TMDBLang)
		fmt.Printf("scan_every: %s\n", cfg.ScanEvery)
		fmt.Printf("tunnel:     %s\n", orDash(cfg.Tunnel))
		fmt.Printf("modo:       %s\n", cfg.Modo)
		if cfg.Nuvem.Configurada() {
			fmt.Printf("nuvem:      s3://%s (%s)", cfg.Nuvem.Bucket, cfg.Nuvem.Regiao)
			if cfg.Nuvem.Perfil != "" {
				fmt.Printf(" perfil %s", cfg.Nuvem.Perfil)
			}
			if cfg.Nuvem.Endpoint != "" {
				fmt.Printf(" via %s", cfg.Nuvem.Endpoint)
			}
			if cfg.Nuvem.CDN() {
				fmt.Printf(" · CDN %s", cfg.Nuvem.CDNDominio)
			}
			if cfg.Nuvem.FilaJobs != "" {
				fmt.Print(" · workers")
			}
			fmt.Println()
		}
		return nil
	}

	if args[0] != "set" || len(args) != 3 {
		return errors.New("uso: nas config [show | set <chave> <valor>]")
	}

	key, value := strings.ToLower(args[1]), args[2]
	switch key {
	case "tmdb_key":
		cfg.TMDBKey = strings.TrimSpace(value)
	case "tmdb_lang":
		cfg.TMDBLang = value
	case "scan_every":
		cfg.ScanEvery = value
	case "tunnel", "tunnel_name":
		cfg.Tunnel = value
	case "nuvem.bucket":
		cfg.Nuvem.Bucket = strings.TrimSpace(value)
	case "nuvem.regiao":
		cfg.Nuvem.Regiao = strings.TrimSpace(value)
	case "nuvem.perfil":
		cfg.Nuvem.Perfil = strings.TrimSpace(value)
	case "nuvem.endpoint":
		cfg.Nuvem.Endpoint = strings.TrimSpace(value)
	case "nuvem.prefixo":
		cfg.Nuvem.Prefixo = strings.Trim(strings.TrimSpace(value), "/")
	case "nuvem.cdn_dominio":
		cfg.Nuvem.CDNDominio = strings.TrimSpace(value)
	case "nuvem.cdn_chave_id":
		cfg.Nuvem.CDNChaveID = strings.TrimSpace(value)
	case "nuvem.cdn_parametro":
		cfg.Nuvem.CDNParametro = strings.TrimSpace(value)
	case "nuvem.fila_jobs":
		cfg.Nuvem.FilaJobs = strings.TrimSpace(value)
	case "nuvem.fila_eventos":
		cfg.Nuvem.FilaEventos = strings.TrimSpace(value)
	case "nuvem.topico_catalogo":
		cfg.Nuvem.Topico = strings.TrimSpace(value)
	case "nuvem.orcamento":
		cfg.Nuvem.Orcamento = strings.TrimSpace(value)
	case "nuvem.path_style":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("nuvem.path_style deve ser true ou false")
		}
		cfg.Nuvem.PathStyle = b
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("porta inválida: %s", value)
		}
		cfg.Port = port
	default:
		return fmt.Errorf("chave desconhecida: %s (use port, tmdb_key, tmdb_lang, scan_every, tunnel, nuvem.bucket, nuvem.regiao, nuvem.perfil, nuvem.endpoint, nuvem.prefixo, nuvem.path_style, nuvem.cdn_dominio, nuvem.cdn_chave_id, nuvem.cdn_parametro, nuvem.fila_jobs, nuvem.fila_eventos, nuvem.topico_catalogo, nuvem.orcamento; o modo muda com `nas modo`)", key)
	}

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("%s atualizado\n", key)
	return nil
}

// maskKey evita que a chave apareça inteira em um terminal compartilhado.
func maskKey(key string) string {
	if key == "" {
		return "(não configurada)"
	}
	if len(key) <= 8 {
		return "********"
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

func orDash(s string) string {
	if s == "" {
		return "(quick tunnel)"
	}
	return s
}
