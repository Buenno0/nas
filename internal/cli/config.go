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
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("porta inválida: %s", value)
		}
		cfg.Port = port
	default:
		return fmt.Errorf("chave desconhecida: %s (use port, tmdb_key, tmdb_lang, scan_every, tunnel)", key)
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
