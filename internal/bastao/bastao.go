// Package bastao faz a troca de task da instância cloud sem derrubar o site
// por minutos. Só pode haver um escritor no SQLite (e um nó com o nome do
// Funnel), então duas tasks não servem ao mesmo tempo; mas a nova pode subir,
// baixar a imagem e esperar com a antiga ainda no ar, e só assumir quando a
// antiga devolver o bastão.
//
// O combinado mora em dois objetos pequenos no bucket:
//
//   - bastao.json: quem está servindo, renovado a cada RenovaCada. Parado há
//     mais de Vencido, o dono morreu (Spot, OOM) e qualquer um assume.
//   - pedido.json: a task nova pedindo a vez. A dona vê, encerra com calma
//     (o Litestream manda as últimas escritas) e marca o bastão como liberado.
package bastao

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"time"

	"nas/internal/cloud"
)

const (
	chaveBastao = "estado/nuvem/bastao.json"
	chavePedido = "estado/nuvem/pedido.json"
)

var (
	RenovaCada   = 10 * time.Second
	Vencido      = 30 * time.Second
	OlhaCada     = 2 * time.Second
	EsperaMaxima = 90 * time.Second
)

// Bucket é o pedaço do armazenamento que o bastão usa.
type Bucket interface {
	Gravar(ctx context.Context, key string, corpo []byte, contentType string) error
	Baixar(ctx context.Context, key string, desde int64) (io.ReadCloser, error)
}

type registro struct {
	Dono     string    `json:"dono"`
	Em       time.Time `json:"em"`
	Liberado bool      `json:"liberado,omitempty"`
}

type pedido struct {
	Por string    `json:"por"`
	Em  time.Time `json:"em"`
}

func ler[T any](ctx context.Context, b Bucket, key string) (T, bool, error) {
	var v T
	r, err := b.Baixar(ctx, key, 0)
	if errors.Is(err, cloud.ErrNaoExiste) {
		return v, false, nil
	}
	if err != nil {
		return v, false, err
	}
	defer r.Close()
	if err := json.NewDecoder(io.LimitReader(r, 4<<10)).Decode(&v); err != nil {
		return v, false, nil // corrompido conta como ausente
	}
	return v, true, nil
}

func gravar(ctx context.Context, b Bucket, key string, v any) error {
	corpo, _ := json.Marshal(v)
	return b.Gravar(ctx, key, corpo, "application/json")
}

// livre diz se dá para assumir: ninguém nunca serviu, a dona liberou, ou ela
// sumiu sem avisar.
func livre(r registro, achou bool, agora time.Time) bool {
	return !achou || r.Liberado || agora.Sub(r.Em) > Vencido
}

// Esperar bloqueia até esta task poder assumir: pede a vez e espera a dona
// liberar. Se a dona não responder em EsperaMaxima (versão antiga, travada),
// assume mesmo assim — ficar fora do ar para sempre é pior.
func Esperar(ctx context.Context, b Bucket, eu string, agora func() time.Time) error {
	r, achou, err := ler[registro](ctx, b, chaveBastao)
	if err != nil {
		return err
	}
	if livre(r, achou, agora()) {
		log.Printf("bastão: livre, assumindo")
		return nil
	}
	log.Printf("bastão: %s está servindo; pedindo a vez", r.Dono)
	if err := gravar(ctx, b, chavePedido, pedido{Por: eu, Em: agora()}); err != nil {
		return err
	}
	limite := agora().Add(EsperaMaxima)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(OlhaCada):
		}
		r, achou, err := ler[registro](ctx, b, chaveBastao)
		if err == nil && livre(r, achou, agora()) {
			log.Printf("bastão: %s liberou", r.Dono)
			return nil
		}
		if agora().After(limite) {
			log.Printf("bastão: %s não respondeu em %s; assumindo", r.Dono, EsperaMaxima)
			return nil
		}
	}
}

// Manter segura o bastão enquanto ctx vive: renova o registro e, quando outra
// task pede a vez, chama passar (que encerra o servidor com calma).
func Manter(ctx context.Context, b Bucket, eu string, agora func() time.Time, passar func()) {
	inicio := agora()
	renovar := func() {
		if err := gravar(ctx, b, chaveBastao, registro{Dono: eu, Em: agora()}); err != nil && ctx.Err() == nil {
			log.Printf("bastão: renovar: %v", err)
		}
	}
	renovar()
	ultima := agora()
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(OlhaCada):
		}
		if agora().Sub(ultima) >= RenovaCada {
			renovar()
			ultima = agora()
		}
		p, achou, err := ler[pedido](ctx, b, chavePedido)
		// Só pedidos feitos depois que esta task assumiu: o da própria
		// chegada dela fica no bucket e não pode derrubá-la.
		if err == nil && achou && p.Por != eu && p.Em.After(inicio) {
			log.Printf("bastão: %s pediu a vez; encerrando", p.Por)
			passar()
			return
		}
	}
}

// Liberar marca o bastão como devolvido. Chamado depois que o Litestream
// terminou: a task nova restaura um banco com tudo.
func Liberar(ctx context.Context, b Bucket, eu string, agora func() time.Time) error {
	// Se a nova já assumiu (a espera venceu antes), o bastão é dela agora:
	// marcar como liberado a exporia a uma terceira task.
	if r, achou, err := ler[registro](ctx, b, chaveBastao); err == nil && achou && r.Dono != eu {
		return nil
	}
	return gravar(ctx, b, chaveBastao, registro{Dono: eu, Em: agora(), Liberado: true})
}
