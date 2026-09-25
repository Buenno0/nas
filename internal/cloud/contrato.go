package cloud

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"
)

// VersaoDoContrato é o schema_version de tudo o que Mac e nuvem trocam por
// fila. Um lado nunca lê o banco do outro: só estas mensagens.
const VersaoDoContrato = 1

// Mensagem é o que sai de uma fila.
type Mensagem struct {
	ID           string
	Recibo       string
	Corpo        []byte
	Recebimentos int
}

// Mensageria são as filas e tópicos. O adapter AWS usa SQS e SNS; a
// interface existe para trocar por NATS sem mexer no resto.
type Mensageria interface {
	EnviarMensagem(ctx context.Context, fila string, corpo []byte) error
	ReceberMensagens(ctx context.Context, fila string, max int32, espera time.Duration) ([]Mensagem, error)
	ApagarMensagem(ctx context.Context, fila, recibo string) error
	// EstenderVisibilidade segura a mensagem enquanto um job longo roda,
	// para outra réplica não pegá-la no meio.
	EstenderVisibilidade(ctx context.Context, fila, recibo string, d time.Duration) error
	Publicar(ctx context.Context, topico string, corpo []byte) error
}

// Job pede o processamento de um objeto do bucket.
type Job struct {
	SchemaVersion int    `json:"schema_version"`
	Tipo          string `json:"tipo"` // "processar"
	Key           string `json:"key"`
	Origem        string `json:"origem"` // "s3" (evento do bucket) | "mac"
}

// Derivado é um arquivo gerado a partir do original.
type Derivado struct {
	Tipo    string `json:"tipo"`             // frame | thumb320 | thumb800 | capa | compat | legenda
	Indice  int    `json:"indice,omitempty"` // stream da legenda
	Key     string `json:"key"`
	Receita string `json:"receita,omitempty"`
}

// Manifesto descreve tudo o que o worker produziu para um objeto. Mora ao
// lado dos derivados (manifesto.json) e viaja no evento job.concluido.
type Manifesto struct {
	SchemaVersion int             `json:"schema_version"`
	Key           string          `json:"key"`
	ETag          string          `json:"etag"`
	Tamanho       int64           `json:"tamanho"`
	Probe         json.RawMessage `json:"probe,omitempty"`
	Derivados     []Derivado      `json:"derivados"`
	ProcessadoEm  time.Time       `json:"processado_em"`
}

// EventoDoCatalogo é o que o tópico catalogo publica.
type EventoDoCatalogo struct {
	SchemaVersion int        `json:"schema_version"`
	Tipo          string     `json:"tipo"` // job.concluido | job.falhou
	Key           string     `json:"key"`
	Manifesto     *Manifesto `json:"manifesto,omitempty"`
	Erro          string     `json:"erro,omitempty"`
}

// PrefixoDosDerivados é estável por chave: reprocessar sobrescreve em vez de
// acumular lixo no bucket.
func PrefixoDosDerivados(key string) string {
	soma := sha1.Sum([]byte(key))
	return "derivados/" + hex.EncodeToString(soma[:10]) + "/"
}

// JobsDaMensagem entende as duas formas que chegam à fila de jobs: o evento
// de ObjectCreated do S3 e um Job pedido pelo Mac.
func JobsDaMensagem(corpo []byte) ([]Job, error) {
	var s3 struct {
		Records []struct {
			EventName string `json:"eventName"`
			S3        struct {
				Object struct {
					Key string `json:"key"`
				} `json:"object"`
			} `json:"s3"`
		} `json:"Records"`
		Event string `json:"Event"` // s3:TestEvent na criação da notificação
	}
	if err := json.Unmarshal(corpo, &s3); err != nil {
		return nil, err
	}
	if s3.Event == "s3:TestEvent" {
		return nil, nil
	}
	if len(s3.Records) > 0 {
		var jobs []Job
		for _, r := range s3.Records {
			if !strings.HasPrefix(r.EventName, "ObjectCreated:") {
				continue
			}
			// O S3 manda a chave url-encoded, com + no lugar de espaço.
			key, err := url.QueryUnescape(r.S3.Object.Key)
			if err != nil {
				continue
			}
			jobs = append(jobs, Job{SchemaVersion: VersaoDoContrato, Tipo: "processar", Key: key, Origem: "s3"})
		}
		return jobs, nil
	}
	var j Job
	if err := json.Unmarshal(corpo, &j); err != nil {
		return nil, err
	}
	if j.Tipo != "processar" || j.Key == "" {
		return nil, nil
	}
	return []Job{j}, nil
}
