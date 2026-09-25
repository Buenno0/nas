package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"nas/internal/auth"
	"nas/internal/db"
)

func (s *Server) handleColecoes(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	cols, err := s.db.Colecoes(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range cols {
		cols[i].Poster = posterURL(cols[i].Poster)
	}
	writeJSON(w, http.StatusOK, cols)
}

func (s *Server) handleCriarColecao(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())

	var corpo struct {
		Nome string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&corpo); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	col, err := s.db.CriarColecao(r.Context(), user.ID, corpo.Nome)
	if err != nil {
		if errors.Is(err, db.ErrColecaoDuplicada) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

func (s *Server) handleColecao(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())

	col, itens, err := s.db.ItensDaColecao(r.Context(), user.ID, atoi64(r.PathValue("id")))
	if err != nil {
		// 404 também quando a coleção é de outra pessoa: dizer "403" revelaria
		// que ela existe.
		writeError(w, http.StatusNotFound, "coleção não encontrada")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"colecao": col,
		"itens":   withImageURLs(itens),
	})
}

func (s *Server) handleRenomearColecao(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())

	var corpo struct {
		Nome string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&corpo); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	err := s.db.RenomearColecao(r.Context(), user.ID, atoi64(r.PathValue("id")), corpo.Nome)
	switch {
	case errors.Is(err, db.ErrColecaoDuplicada):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "coleção não encontrada")
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (s *Server) handleApagarColecao(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	if err := s.db.ApagarColecao(r.Context(), user.ID, atoi64(r.PathValue("id"))); err != nil {
		writeError(w, http.StatusNotFound, "coleção não encontrada")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleItemDaColecao(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	colecaoID := atoi64(r.PathValue("id"))
	titleID := atoi64(r.PathValue("titulo"))

	var err error
	if r.Method == http.MethodDelete {
		err = s.db.RemoverDaColecao(r.Context(), user.ID, colecaoID, titleID)
	} else {
		err = s.db.AdicionarNaColecao(r.Context(), user.ID, colecaoID, titleID)
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "coleção não encontrada")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleReordenarColecao(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())

	var corpo struct {
		Titulos []int64 `json:"titulos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&corpo); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if err := s.db.ReordenarColecao(r.Context(), user.ID, atoi64(r.PathValue("id")), corpo.Titulos); err != nil {
		writeError(w, http.StatusNotFound, "coleção não encontrada")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleColecoesDoTitulo diz em quais listas este título já está.
func (s *Server) handleColecoesDoTitulo(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	ids, err := s.db.ColecoesComTitulo(r.Context(), user.ID, atoi64(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"colecoes": ids})
}
