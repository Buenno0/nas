package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/energia"
)

// multipartFalso conhece os envios que iniciou; os "esquecidos" somem.
type multipartFalso struct {
	*filaFalsa
	mu        sync.Mutex
	n         int
	abertos   map[string]bool
	iniciados int
}

func (m *multipartFalso) IniciarEnvio(context.Context, string, string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.n++
	m.iniciados++
	id := fmt.Sprintf("up-%d", m.n)
	m.abertos[id] = true
	return id, nil
}

func (m *multipartFalso) PartesEnviadas(_ context.Context, _, id string) ([]cloud.Parte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.abertos[id] {
		return nil, cloud.ErrNaoExiste
	}
	return nil, nil
}

// Soltar o mesmo arquivo duas vezes continua o primeiro envio; se o bucket
// já esqueceu dele, o velho é fechado e um novo começa.
func TestMesmoArquivoReaproveitaOEnvio(t *testing.T) {
	srv, token, _, f := preparaHibrido(t, energia.Estado{})
	arm, _, _ := srv.nuvem.Hibrido()
	falso := &multipartFalso{filaFalsa: arm.(*filaFalsa), abertos: map[string]bool{}}
	srv.nuvem.Desligar()
	cloud.Registrar(func(context.Context, config.Nuvem, *http.Client) (cloud.Armazenamento, error) { return falso, nil })
	if err := srv.nuvem.Ativar(context.Background()); err != nil {
		t.Fatal(err)
	}

	corpo := fmt.Sprintf(`{"library_id":%d,"nome":"Novo.mkv","tamanho":123456}`, f.LibraryID)
	cria := func() db.Upload {
		t.Helper()
		rec := chama(t, srv, http.MethodPost, "/api/uploads", token, corpo)
		var r struct{ Upload db.Upload }
		if err := json.NewDecoder(rec.Body).Decode(&r); err != nil || rec.Code >= 300 {
			t.Fatalf("%d %v", rec.Code, err)
		}
		return r.Upload
	}
	a, b := cria(), cria()
	if a.ID != b.ID || falso.iniciados != 1 {
		t.Fatalf("o mesmo arquivo abriu dois envios: %d e %d (%d no bucket)", a.ID, b.ID, falso.iniciados)
	}

	falso.abertos = map[string]bool{} // lifecycle limpou
	c := cria()
	if c.ID == a.ID || falso.iniciados != 2 {
		t.Fatalf("envio esquecido pelo bucket foi reaproveitado: %+v", c)
	}
	if velho, _ := srv.db.UploadPorID(context.Background(), a.ID); velho.Estado != "abortado" {
		t.Fatalf("o envio velho ficou %q", velho.Estado)
	}
}
