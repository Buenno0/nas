// Package cloud é a fronteira do modo híbrido.
//
// O resto do Ozymandias depende só das interfaces daqui. O SDK da AWS é
// importado apenas em internal/cloud/aws, que se registra por build tag: com
// `-tags nocloud` o binário sai sem ele, e o compilador prova o desacoplamento.
package cloud

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"nas/internal/config"
)

// Parte é um pedaço já enviado de um upload multipart.
type Parte struct {
	Numero int32  `json:"n"`
	ETag   string `json:"etag"`
}

// Armazenamento é o bucket, visto pelo app. Qualquer API S3 serve.
type Armazenamento interface {
	// Verificar confirma que credencial e bucket respondem. É o que impede o
	// modo de ficar "meio conectado".
	Verificar(ctx context.Context) error
	// Tamanho devolve o tamanho do objeto, ou ErrNaoExiste.
	Tamanho(ctx context.Context, key string) (int64, error)

	// URLDeLeitura assina um GET: o cliente lê direto do bucket, sem passar
	// pelo Mac. Assinar é só cálculo, não abre conexão.
	URLDeLeitura(ctx context.Context, key string, ttl time.Duration) (string, error)

	IniciarEnvio(ctx context.Context, key, contentType string) (uploadID string, err error)
	URLDaParte(ctx context.Context, key, uploadID string, n int32, ttl time.Duration) (string, error)
	EnviarParte(ctx context.Context, key, uploadID string, n int32, corpo io.ReadSeeker, tamanho int64) (etag string, err error)
	PartesEnviadas(ctx context.Context, key, uploadID string) ([]Parte, error)
	ConcluirEnvio(ctx context.Context, key, uploadID string, partes []Parte) error
	AbortarEnvio(ctx context.Context, key, uploadID string) error
}

// Conector cria o Armazenamento. Todo tráfego dele precisa sair pelo cliente
// HTTP recebido, que é o guardado pelo kill switch.
type Conector func(ctx context.Context, cfg config.Nuvem, cliente *http.Client) (Armazenamento, error)

var (
	ErrModoLocal    = errors.New("modo local: nenhuma chamada à nuvem")
	ErrSemSuporte   = errors.New("este binário foi compilado sem suporte a nuvem (-tags nocloud)")
	ErrNaoExiste    = errors.New("objeto não existe no bucket")
	ErrNaoConfig    = errors.New("nuvem não configurada: defina nuvem.bucket e nuvem.regiao no config.json")
	ErrTravado      = errors.New("--sem-nuvem ativo: o modo híbrido está travado nesta execução")
	ErrInterrompido = errors.New("kill switch acionado durante a conexão")
)

var (
	conectorMu sync.RWMutex
	conector   Conector = semSuporte
	registrado bool
)

// Registrar instala o adapter real. Chamado no init do pacote do adapter.
func Registrar(c Conector) {
	conectorMu.Lock()
	defer conectorMu.Unlock()
	conector = c
	registrado = true
}

func conectorAtual() Conector {
	conectorMu.RLock()
	defer conectorMu.RUnlock()
	return conector
}
