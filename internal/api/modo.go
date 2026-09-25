package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nas/internal/cloud"
	"nas/internal/config"
)

// Nuvem expõe o kill switch para a CLI (SIGHUP recarrega o modo).
func (s *Server) Nuvem() *cloud.Chave { return s.nuvem }

// estadoModo é o Estado da chave mais o papel deste nó, que a interface usa
// para decidir o que é "indisponível" (na nuvem, no Mac).
type estadoModo struct {
	cloud.Estado
	Papel string `json:"papel"`
}

func (s *Server) estadoModo() estadoModo { return estadoModo{s.nuvem.Estado(), s.papel()} }

func (s *Server) handleGetModo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.estadoModo())
}

// handlePutModo alterna o modo e grava no config.json, para sobreviver a
// reinícios. Desligar é instantâneo; ligar espera a conexão e devolve o erro
// se ela falhar — o modo nunca fica meio-conectado.
func (s *Server) handlePutModo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Modo string `json:"modo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	switch body.Modo {
	case config.ModoLocal:
		s.nuvem.Desligar()
	case config.ModoHibrido:
		// O contexto do servidor, não o da requisição: fechar a aba não deve
		// deixar a transição pela metade.
		if err := s.nuvem.Ativar(s.fundo); err != nil {
			writeJSON(w, http.StatusBadGateway, s.estadoModo())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, `modo deve ser "local" ou "hibrido"`)
		return
	}
	if err := s.gravaModo(body.Modo); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.estadoModo())
}

func (s *Server) gravaModo(modo string) error {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	cfg := s.cfg
	cfg.Modo = modo
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("gravando modo: %w", err)
	}
	s.cfg = cfg
	return nil
}

// RecarregarModo aplica o modo do config.json (nas modo … manda SIGHUP).
func (s *Server) RecarregarModo(ctx context.Context, cfg config.Config) error {
	s.cfgMu.Lock()
	s.cfg.Modo = cfg.Modo
	s.cfg.Nuvem = cfg.Nuvem
	s.cfgMu.Unlock()
	if cfg.Modo == config.ModoHibrido {
		return s.nuvem.Ativar(ctx)
	}
	s.nuvem.Desligar()
	return nil
}

// ConfigNuvem é o que a chave lê ao conectar.
func (s *Server) ConfigNuvem() config.Nuvem {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.Nuvem
}

// handleModoEvents avisa a interface por SSE a cada mudança de modo. O upload
// do navegador depende disso para pausar no instante do kill switch.
func (s *Server) handleModoEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, sair := s.nuvem.Assinar()
	defer sair()

	envia := func(e cloud.Estado) {
		if payload, err := json.Marshal(estadoModo{e, s.papel()}); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
	envia(s.nuvem.Estado())

	// Comentário periódico: proxies derrubam SSE mudo.
	pulso := time.NewTicker(25 * time.Second)
	defer pulso.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			envia(e)
		case <-pulso.C:
			fmt.Fprint(w, ": pulso\n\n")
			flusher.Flush()
		}
	}
}
