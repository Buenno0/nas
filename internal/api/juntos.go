package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"nas/internal/auth"
	assistir "nas/internal/juntos"
)

type pedidoDeSala struct {
	FileID  int64   `json:"file_id"`
	Posicao float64 `json:"posicao"`
	Tocando bool    `json:"tocando"`
}

// handleCriarSala abre uma sala de "assistir junto" para um arquivo.
func (s *Server) handleCriarSala(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	var p pedidoDeSala
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&p); err != nil || p.FileID <= 0 {
		writeError(w, http.StatusBadRequest, "informe o arquivo")
		return
	}
	if _, err := s.db.FileByID(r.Context(), p.FileID); err != nil {
		writeError(w, http.StatusNotFound, "arquivo não encontrado")
		return
	}
	e, err := s.juntos.Criar(p.FileID, p.Posicao, p.Tocando, user.Username)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleSala(w http.ResponseWriter, r *http.Request) {
	e, err := s.juntos.Estado(r.PathValue("codigo"))
	if err != nil {
		writeError(w, http.StatusNotFound, "essa sala não existe mais")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleComandoDaSala(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	var c assistir.Comando
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if c.Tipo == "arquivo" {
		if _, err := s.db.FileByID(r.Context(), c.FileID); err != nil {
			writeError(w, http.StatusNotFound, "arquivo não encontrado")
			return
		}
	}
	if err := s.juntos.Aplicar(r.PathValue("codigo"), user.Username, c); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, assistir.ErrSalaNaoExiste) {
			status = http.StatusNotFound
		}
		if errors.Is(err, assistir.ErrSoDono) {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEventosDaSala é o SSE da sala: estado a cada mudança, e reações.
// Cada mensagem leva a hora do servidor, para o cliente medir o atraso.
func (s *Server) handleEventosDaSala(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return
	}
	ch, sair, err := s.juntos.Entrar(r.PathValue("codigo"), user.Username)
	if err != nil {
		writeError(w, http.StatusNotFound, "essa sala não existe mais")
		return
	}
	defer sair()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	pulso := time.NewTicker(20 * time.Second)
	defer pulso.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-ch:
			payload, err := json.Marshal(struct {
				assistir.Mensagem
				Agora int64 `json:"agora"`
			}{m, time.Now().UnixMilli()})
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		case <-pulso.C:
			fmt.Fprint(w, ": pulso\n\n")
			flusher.Flush()
		}
	}
}
