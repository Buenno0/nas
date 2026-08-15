// Package config gerencia o estado persistente do NAS em ~/.nas.
//
// O binário é global e pode ser executado de qualquer diretório, então nada
// aqui depende do diretório de trabalho atual.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config é o conteúdo de ~/.nas/config.json. As bibliotecas de mídia ficam no
// banco (tabela libraries), não aqui — este arquivo guarda só as preferências
// do servidor.
type Config struct {
	Port      int    `json:"port"`
	TMDBKey   string `json:"tmdb_key"`
	TMDBLang  string `json:"tmdb_lang"`
	ScanEvery string `json:"scan_every"`  // duração Go, ex: "6h". Vazio desliga.
	Tunnel    string `json:"tunnel_name"` // vazio = quick tunnel
}

// Default devolve a configuração usada no primeiro boot.
func Default() Config {
	return Config{
		Port:      8787,
		TMDBLang:  "pt-BR",
		ScanEvery: "6h",
	}
}

// Dir é ~/.nas — criado sob demanda.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("descobrindo home: %w", err)
	}
	return filepath.Join(home, ".nas"), nil
}

// EnsureDir cria ~/.nas e os subdiretórios de cache, devolvendo o caminho base.
func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	for _, sub := range []string{"", "cache/posters", "cache/thumbs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return "", fmt.Errorf("criando %s: %w", filepath.Join(dir, sub), err)
		}
	}
	return dir, nil
}

func pathIn(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func FilePath() (string, error)  { return pathIn("config.json") }
func DBPath() (string, error)    { return pathIn("nas.db") }
func PIDPath() (string, error)   { return pathIn("nas.pid") }
func LogPath() (string, error)   { return pathIn("nas.log") }
func PosterDir() (string, error) { return pathIn("cache/posters") }
func ThumbDir() (string, error)  { return pathIn("cache/thumbs") }

// Load lê a configuração, criando o arquivo padrão se ele ainda não existir.
func Load() (Config, error) {
	if _, err := EnsureDir(); err != nil {
		return Config{}, err
	}
	path, err := FilePath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		if err := Save(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("lendo %s: %w", path, err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config.json inválido: %w", err)
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = Default().Port
	}
	return cfg, nil
}

// Save grava a configuração de forma atômica (arquivo temporário + rename).
// O modo 0600 protege a chave do TMDB.
func Save(cfg Config) error {
	dir, err := EnsureDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, "config-*.json")
	if err != nil {
		return fmt.Errorf("criando temporário: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("escrevendo config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ScanInterval interpreta ScanEvery; 0 significa scan periódico desligado.
func (c Config) ScanInterval() time.Duration {
	if c.ScanEvery == "" {
		return 0
	}
	d, err := time.ParseDuration(c.ScanEvery)
	if err != nil || d < time.Minute {
		return 0
	}
	return d
}
