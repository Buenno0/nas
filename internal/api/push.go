package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"nas/internal/auth"
	"nas/internal/db"
	"nas/internal/push"
)

var tiposDeAviso = map[string]bool{push.TipoEnvio: true, push.TipoPreparo: true, push.TipoNovidade: true}

func (s *Server) handlePushChave(w http.ResponseWriter, r *http.Request) {
	publica, _, err := s.push.Chaves(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"chave": publica})
}

type pedidoDeInscricao struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Tipos []string `json:"tipos"`
}

// handlePushInscrever guarda (ou atualiza) a inscrição deste aparelho.
func (s *Server) handlePushInscrever(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	var p pedidoDeInscricao
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	// Só serviços de push de verdade: o endpoint vira uma requisição que este
	// servidor faz, e não pode apontar para a rede interna.
	if !strings.HasPrefix(p.Endpoint, "https://") || p.Keys.P256dh == "" || p.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "inscrição de push inválida")
		return
	}
	tipos := []string{}
	for _, t := range p.Tipos {
		if tiposDeAviso[t] {
			tipos = append(tipos, t)
		}
	}
	if err := s.db.SalvaInscricao(r.Context(), db.InscricaoPush{
		UserID: user.ID, Endpoint: p.Endpoint, P256dh: p.Keys.P256dh, Auth: p.Keys.Auth, Tipos: tipos,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tipos": tipos})
}

func (s *Server) handlePushRemover(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	var p pedidoDeInscricao
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&p); err != nil || p.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "informe o endpoint")
		return
	}
	if err := s.db.RemoveInscricao(r.Context(), p.Endpoint, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePushEstado diz se este aparelho (pelo endpoint) está inscrito.
func (s *Server) handlePushEstado(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	tipos, ok := s.db.InscricaoDo(r.Context(), user.ID, r.URL.Query().Get("endpoint"))
	if tipos == nil {
		tipos = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"inscrito": ok, "tipos": tipos})
}

// handlePushTeste manda um aviso de teste aos aparelhos de quem pede.
func (s *Server) handlePushTeste(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	s.push.Para(push.TipoTeste, push.Aviso{Titulo: "Ozymandias", Corpo: "Os avisos chegam neste aparelho.", URL: "/", Tag: "teste"}, user.ID)
	w.WriteHeader(http.StatusAccepted)
}

// --- gatilhos ---------------------------------------------------------------

// destinoDoArquivo é a página do título do arquivo, para o toque na notificação.
func (s *Server) destinoDoArquivo(ctx context.Context, fileID int64) (nome, url string) {
	f, err := s.db.FileByID(ctx, fileID)
	if err != nil {
		return "", "/"
	}
	nome = path.Base(f.RelPath)
	if f.TitleID != nil {
		if t, err := s.db.TitleByID(ctx, *f.TitleID); err == nil {
			nome = t.Name
		}
		return nome, fmt.Sprintf("/title/%d", *f.TitleID)
	}
	return nome, "/"
}

// avisarEnvio: um envio para a nuvem terminou. userID 0 = feito pelo Mac a
// pedido de um admin (vai aos admins).
func (s *Server) avisarEnvio(fileID int64, userID *int64) {
	nome, url := s.destinoDoArquivo(context.Background(), fileID)
	av := push.Aviso{Titulo: "Na nuvem", Corpo: nome + " terminou de subir.", URL: url, Tag: fmt.Sprintf("envio-%d", fileID)}
	if userID != nil && *userID != 0 {
		s.push.Para(push.TipoEnvio, av, *userID)
		return
	}
	s.push.ParaAdmins(push.TipoEnvio, av)
}

// avisarPreparo: o worker deixou pronta a versão que toca em qualquer aparelho.
func (s *Server) avisarPreparo(fileID int64) {
	nome, url := s.destinoDoArquivo(context.Background(), fileID)
	s.push.ParaAdmins(push.TipoPreparo, push.Aviso{
		Titulo: "Pronto para assistir", Corpo: nome + " já toca em qualquer aparelho.", URL: url,
		Tag: fmt.Sprintf("preparo-%d", fileID),
	})
}

// novidades junta o que entrou no acervo em 2 min num aviso só: um lote de
// uploads não vira uma chuva de notificações.
type novidades struct {
	mu     sync.Mutex
	nomes  []string
	ultima string
	timer  *time.Timer
}

func (s *Server) avisarNovidade(fileID int64) {
	nome, url := s.destinoDoArquivo(context.Background(), fileID)
	s.novas.mu.Lock()
	defer s.novas.mu.Unlock()
	s.novas.nomes = append(s.novas.nomes, nome)
	s.novas.ultima = url
	if s.novas.timer != nil {
		return
	}
	s.novas.timer = time.AfterFunc(2*time.Minute, func() {
		s.novas.mu.Lock()
		nomes, url := s.novas.nomes, s.novas.ultima
		s.novas.nomes, s.novas.timer = nil, nil
		s.novas.mu.Unlock()
		corpo := nomes[0] + " entrou no acervo."
		if len(nomes) > 1 {
			corpo = fmt.Sprintf("%s e mais %d entraram no acervo.", nomes[0], len(nomes)-1)
			url = "/"
		}
		s.push.Para(push.TipoNovidade, push.Aviso{Titulo: "Novo no Ozymandias", Corpo: corpo, URL: url, Tag: "novidade"})
	})
}
