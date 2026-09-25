package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"nas/internal/cloud"
	"nas/internal/db"
	"nas/internal/energia"
	"nas/internal/sincro"
)

func (s *Server) handleSincronizacao(w http.ResponseWriter, r *http.Request) {
	// A energia vai junto: é ela que explica por que um preparo foi para a
	// nuvem (bursting) em vez de rodar aqui.
	writeJSON(w, http.StatusOK, struct {
		sincro.Estado
		Energia energia.Estado `json:"energia"`
	}{s.sincro.Estado(r.Context()), s.opts.Energia.Estado(r.Context())})
}

func (s *Server) handleReconciliar(w http.ResponseWriter, r *http.Request) {
	if s.nuvem.Modo() != cloud.ModoHibrido {
		writeError(w, http.StatusServiceUnavailable, "modo local: a nuvem está desligada")
		return
	}
	go s.sincro.Reconciliar(s.fundo)
	writeJSON(w, http.StatusAccepted, map[string]bool{"iniciado": true})
}

func (s *Server) handleAcaoDeNuvem(w http.ResponseWriter, r *http.Request) {
	id := atoi64(r.PathValue("id"))
	var err error
	switch r.PathValue("acao") {
	case "enviar":
		err = s.sincro.Enviar(r.Context(), id)
	case "fixar":
		err = s.sincro.Fixar(r.Context(), id)
	case "liberar":
		err = s.sincro.Liberar(r.Context(), id)
	case "remover":
		err = s.sincro.RemoverDaNuvem(r.Context(), id)
	default:
		writeError(w, http.StatusNotFound, "ação desconhecida")
		return
	}
	if err != nil {
		writeError(w, statusDaSincro(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"na_fila": true})
}

func statusDaSincro(err error) int {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, sincro.ErrSoHibrido):
		return http.StatusServiceUnavailable
	case errors.Is(err, sincro.ErrSemEspaco):
		return http.StatusInsufficientStorage
	case errors.Is(err, sincro.ErrEstado), errors.Is(err, sincro.ErrJaNaFila):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (s *Server) handleEspelhada(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Espelhada bool `json:"espelhada"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if err := s.sincro.DefinirEspelho(r.Context(), atoi64(r.PathValue("id")), body.Espelhada); err != nil {
		writeError(w, statusDaSincro(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"espelhada": body.Espelhada})
}
