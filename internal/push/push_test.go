package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"nas/internal/db"
)

func chaveDeAparelho(t *testing.T) (string, string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	rand.Read(auth)
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(auth)
}

// O aviso chega a quem se inscreveu no tipo; uma inscrição que o serviço diz
// ter morrido (410) é apagada.
func TestAvisoEntregaEApagaInscricaoMorta(t *testing.T) {
	ctx := context.Background()
	d, err := db.OpenAt(filepath.Join(t.TempDir(), "nas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ana, _ := d.CreateUser(ctx, "ana", "$argon2id$x", false, true)

	var entregues atomic.Int32
	servico := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" || r.Header.Get("Content-Encoding") != "aes128gcm" {
			t.Errorf("push sem VAPID ou sem criptografia: %v", r.Header)
		}
		if r.URL.Path == "/morto" {
			w.WriteHeader(http.StatusGone)
			return
		}
		entregues.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer servico.Close()

	for _, caminho := range []string{"/vivo", "/morto"} {
		p, a := chaveDeAparelho(t)
		if err := d.SalvaInscricao(ctx, db.InscricaoPush{UserID: ana.ID, Endpoint: servico.URL + caminho, P256dh: p, Auth: a, Tipos: []string{TipoEnvio}}); err != nil {
			t.Fatal(err)
		}
	}
	av := NovoCom(d, servico.Client())
	av.entregar(TipoEnvio, false, []int64{ana.ID}, Aviso{Titulo: "Na nuvem", Corpo: "Duna terminou de subir."})

	if entregues.Load() != 1 {
		t.Fatalf("entregues = %d, quero 1", entregues.Load())
	}
	restantes, _ := d.Inscricoes(ctx, TipoEnvio, false, nil)
	if len(restantes) != 1 || restantes[0].Endpoint != servico.URL+"/vivo" {
		t.Fatalf("a inscrição morta ficou: %+v", restantes)
	}
	// Quem não quis o tipo não recebe.
	av.entregar(TipoPreparo, false, nil, Aviso{Titulo: "x"})
	if entregues.Load() != 1 {
		t.Fatal("entregou um tipo que o aparelho não escolheu")
	}
}
