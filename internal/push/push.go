// Package push manda avisos ao celular pelo Web Push do navegador (no iPhone,
// com o site adicionado à Tela de Início). É gratuito: quem entrega é o
// serviço de push da Apple/Google/Mozilla, nada da AWS.
package push

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"nas/internal/db"
)

// Tipos de aviso que um aparelho pode escolher.
const (
	TipoEnvio    = "envio"    // um envio para a nuvem terminou
	TipoPreparo  = "preparo"  // a versão compatível de um filme ficou pronta
	TipoNovidade = "novidade" // entrou coisa nova no acervo
	TipoTeste    = "teste"
)

// Aviso é o que aparece na notificação. URL é o caminho aberto ao tocar.
type Aviso struct {
	Titulo string `json:"titulo"`
	Corpo  string `json:"corpo"`
	URL    string `json:"url,omitempty"`
	Tag    string `json:"tag,omitempty"` // avisos com a mesma tag se substituem
}

// Avisador guarda as chaves VAPID deste nó e entrega os avisos.
type Avisador struct {
	db      *db.DB
	cliente *http.Client
}

func Novo(database *db.DB) *Avisador {
	return NovoCom(database, &http.Client{Timeout: 15 * time.Second})
}

// NovoCom usa o cliente HTTP dado (os testes apontam para um serviço falso).
func NovoCom(database *db.DB, cliente *http.Client) *Avisador {
	return &Avisador{db: database, cliente: cliente}
}

// Chaves devolve o par VAPID deste nó, gerando no primeiro uso. Na instância
// cloud ele fica no banco, que o Litestream guarda no bucket.
func (a *Avisador) Chaves(ctx context.Context) (publica, privada string, err error) {
	publica = a.db.EstadoNuvem(ctx, "vapid_publica")
	privada = a.db.EstadoNuvem(ctx, "vapid_privada")
	if publica != "" && privada != "" {
		return publica, privada, nil
	}
	privada, publica, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	if err := a.db.GravaEstadoNuvem(ctx, "vapid_privada", privada); err != nil {
		return "", "", err
	}
	return publica, privada, a.db.GravaEstadoNuvem(ctx, "vapid_publica", publica)
}

// ParaAdmins avisa todos os administradores inscritos no tipo.
func (a *Avisador) ParaAdmins(tipo string, av Aviso) { a.enviar(tipo, true, nil, av) }

// Para avisa as pessoas dadas (nenhuma = todo mundo).
func (a *Avisador) Para(tipo string, av Aviso, userIDs ...int64) { a.enviar(tipo, false, userIDs, av) }

// enviar roda em segundo plano: um serviço de push lento nunca segura a
// requisição que gerou o aviso.
func (a *Avisador) enviar(tipo string, admins bool, userIDs []int64, av Aviso) {
	go a.entregar(tipo, admins, userIDs, av)
}

func (a *Avisador) entregar(tipo string, admins bool, userIDs []int64, av Aviso) {
	{
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		inscricoes, err := a.db.Inscricoes(ctx, tipo, admins, userIDs)
		if err != nil || len(inscricoes) == 0 {
			return
		}
		publica, privada, err := a.Chaves(ctx)
		if err != nil {
			log.Printf("push: chaves: %v", err)
			return
		}
		corpo, _ := json.Marshal(av)
		for _, i := range inscricoes {
			resp, err := webpush.SendNotificationWithContext(ctx, corpo, &webpush.Subscription{
				Endpoint: i.Endpoint, Keys: webpush.Keys{P256dh: i.P256dh, Auth: i.Auth},
			}, &webpush.Options{
				HTTPClient: a.cliente, Subscriber: "https://github.com/Buenno0/nas",
				VAPIDPublicKey: publica, VAPIDPrivateKey: privada, TTL: 12 * 3600, Urgency: webpush.UrgencyNormal,
			})
			if err != nil {
				a.db.FalhaDaInscricao(ctx, i.ID)
				continue
			}
			resp.Body.Close()
			switch {
			case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
				// O aparelho desfez a inscrição (ou o navegador foi reinstalado).
				_ = a.db.RemoveInscricao(ctx, i.Endpoint, 0)
			case resp.StatusCode >= 400:
				a.db.FalhaDaInscricao(ctx, i.ID)
			default:
				a.db.EntregaDaInscricao(ctx, i.ID)
			}
		}
	}
}
