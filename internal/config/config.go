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

	// Transcodificação sob demanda.
	//
	// CacheGB é o orçamento do cache de arquivos preparados; ReservaGB é o
	// espaço livre que o NAS nunca consome — o cache divide o volume com o
	// nas.db, e encher o disco derrubaria login e progresso, não só a
	// reprodução. Trabalhos limita quantos ffmpeg rodam juntos: o motor de
	// hardware da Apple tem vazão fixa, então mais processos só repartem a
	// mesma banda.
	Transcode bool    `json:"transcode"`
	CacheGB   float64 `json:"cache_gb"`
	ReservaGB float64 `json:"reserva_gb"`
	Trabalhos int     `json:"trabalhos"`

	// Modo de nuvem: "local" (padrão, zero AWS) ou "hibrido". É um eixo
	// independente do acesso LAN/Tunnel.
	Modo  string `json:"modo"`
	Nuvem Nuvem  `json:"nuvem"`
}

// Nuvem descreve o bucket do modo híbrido. Nenhum segredo mora aqui: as
// credenciais vêm da cadeia padrão da AWS, e Perfil costuma apontar para um
// perfil com credential_process (o aws_signing_helper do IAM Roles Anywhere,
// com a chave privada no Keychain).
type Nuvem struct {
	Regiao string `json:"regiao"`
	Bucket string `json:"bucket"`
	Perfil string `json:"perfil,omitempty"`
	// Endpoint troca a AWS por qualquer API S3: MinIO nos testes, R2, B2.
	Endpoint  string `json:"endpoint,omitempty"`
	PathStyle bool   `json:"path_style,omitempty"`
	// Prefixo isola o Ozymandias dentro de um bucket compartilhado.
	Prefixo string `json:"prefixo,omitempty"`

	// CloudFront com OAC: com os três preenchidos, a leitura sai assinada
	// pela CDN em vez de URL pré-assinada do S3. A chave privada mora no SSM
	// (CDNParametro) e só é lida para a memória ao entrar no híbrido.
	CDNDominio   string `json:"cdn_dominio,omitempty"`
	CDNChaveID   string `json:"cdn_chave_id,omitempty"`
	CDNParametro string `json:"cdn_parametro,omitempty"`

	// V3/V4: FilaJobs recebe pedidos de processamento (URL da fila SQS).
	// FilaEventos é a assinatura DESTE nó no tópico catalogo (no-mac no Mac,
	// no-cloud na instância cloud). Topico é o ARN do catalogo, onde este nó
	// publica o que muda aqui.
	FilaJobs    string `json:"fila_jobs,omitempty"`
	FilaEventos string `json:"fila_eventos,omitempty"`
	Topico      string `json:"topico_catalogo,omitempty"`

	// Orcamento é o nome do AWS Budget que o painel de custo compara com o
	// gasto. Vazio = "ozymandias-mensal", o que o OpenTofu cria.
	Orcamento string `json:"orcamento,omitempty"`

	// Papel não é gravado: "mac" (padrão) ou "nuvem" (nas serve --nuvem).
	Papel string `json:"-"`
}

const (
	PapelMac   = "mac"
	PapelNuvem = "nuvem"
)

// OrigemDosEventos é o nome deste nó nos eventos que ele publica.
func (n Nuvem) OrigemDosEventos() string {
	if n.Papel == PapelNuvem {
		return PapelNuvem
	}
	return PapelMac
}

// CDN diz se a leitura deve passar pelo CloudFront.
func (n Nuvem) CDN() bool { return n.CDNDominio != "" && n.CDNChaveID != "" && n.CDNParametro != "" }

// Configurada diz se há o mínimo para tentar o modo híbrido.
func (n Nuvem) Configurada() bool { return n.Bucket != "" && n.Regiao != "" }

const (
	ModoLocal   = "local"
	ModoHibrido = "hibrido"
)

// Default devolve a configuração usada no primeiro boot.
func Default() Config {
	return Config{
		Port:      8787,
		TMDBLang:  "pt-BR",
		ScanEvery: "6h",
		Transcode: true,
		CacheGB:   8,
		ReservaGB: 5,
		Trabalhos: 2,
		Modo:      ModoLocal,
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
	for _, sub := range []string{"", "cache/posters", "cache/thumbs", "cache/preparados"} {
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

func FilePath() (string, error)   { return pathIn("config.json") }
func DBPath() (string, error)     { return pathIn("nas.db") }
func PIDPath() (string, error)    { return pathIn("nas.pid") }
func LogPath() (string, error)    { return pathIn("nas.log") }
func PosterDir() (string, error)  { return pathIn("cache/posters") }
func ThumbDir() (string, error)   { return pathIn("cache/thumbs") }
func PrepareDir() (string, error) { return pathIn("cache/preparados") }

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
	if cfg.CacheGB <= 0 {
		cfg.CacheGB = Default().CacheGB
	}
	if cfg.ReservaGB <= 0 {
		cfg.ReservaGB = Default().ReservaGB
	}
	if cfg.Modo != ModoHibrido {
		cfg.Modo = ModoLocal
	}
	if cfg.Trabalhos <= 0 || cfg.Trabalhos > 8 {
		cfg.Trabalhos = Default().Trabalhos
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
