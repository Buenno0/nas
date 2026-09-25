package cloud

import (
	"context"
	"time"
)

// Inspecao é o raio-X do bucket para a tela técnica. Cada parte que falhar
// (tipicamente por falta de permissão) vai para Erros e o resto segue.
type Inspecao struct {
	Bucket           string            `json:"bucket"`
	Regiao           string            `json:"regiao"`
	Versionamento    string            `json:"versionamento"`
	Lifecycle        []RegraDoBucket   `json:"lifecycle"`
	CORS             []string          `json:"cors"`
	Prefixos         []UsoDoPrefixo    `json:"prefixos"`
	Pendentes        []EnvioPendente   `json:"multiparts_pendentes"`
	CredencialExpira *time.Time        `json:"credencial_expira,omitempty"`
	Erros            map[string]string `json:"erros,omitempty"`
}

type RegraDoBucket struct {
	ID      string `json:"id"`
	Prefixo string `json:"prefixo"`
	Ativa   bool   `json:"ativa"`
	Resumo  string `json:"resumo"`
}

// UsoDoPrefixo soma o que há sob um prefixo de primeiro nível. Truncado: a
// contagem parou no teto, para não varrer um bucket enorme a cada consulta.
type UsoDoPrefixo struct {
	Prefixo  string           `json:"prefixo"`
	Bytes    int64            `json:"bytes"`
	Objetos  int64            `json:"objetos"`
	Classes  map[string]int64 `json:"classes"`
	Truncado bool             `json:"truncado"`
}

type EnvioPendente struct {
	Key      string    `json:"key"`
	Iniciado time.Time `json:"iniciado"`
}

// EstadoDaFila são os atributos aproximados de uma fila SQS.
type EstadoDaFila struct {
	Nome      string `json:"nome"`
	Visiveis  int64  `json:"visiveis"`
	EmVoo     int64  `json:"em_voo"`
	Atrasadas int64  `json:"atrasadas"`
	DLQ       string `json:"dlq,omitempty"`
}

// Inspetor é opcional no adapter, como a Mensageria: o fake dos testes e um
// S3 qualquer podem não ter.
type Inspetor interface {
	Inspecionar(ctx context.Context) Inspecao
	EstadoDaFila(ctx context.Context, url string) (EstadoDaFila, error)
}
