package api

import (
	"net/http"
	"strconv"
	"time"

	"nas/internal/auth"
	"nas/internal/db"
)

// handleRetrospectiva devolve o ano de quem pede ("seu ano no Ozymandias").
// Cada pessoa só vê o próprio histórico.
func (s *Server) handleRetrospectiva(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	ano := time.Now().Year()
	if v, err := strconv.Atoi(r.URL.Query().Get("ano")); err == nil && v >= 2000 && v <= ano {
		ano = v
	}
	ret, err := s.db.AnoDaRetrospectiva(r.Context(), user.ID, ano)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Os nomes das imagens viram URLs, como no resto da API.
	for i := range ret.MaisVistos {
		ret.MaisVistos[i].Poster = posterURL(ret.MaisVistos[i].Poster)
		ret.MaisVistos[i].Backdrop = posterURL(ret.MaisVistos[i].Backdrop)
	}
	if ret.Primeiro != nil {
		ret.Primeiro.Poster = posterURL(ret.Primeiro.Poster)
		ret.Primeiro.Backdrop = posterURL(ret.Primeiro.Backdrop)
	}
	if ret.Ultimo != nil {
		ret.Ultimo.Poster = posterURL(ret.Ultimo.Poster)
		ret.Ultimo.Backdrop = posterURL(ret.Ultimo.Backdrop)
	}
	if ret.Maratona != nil {
		ret.Maratona.Titulo.Poster = posterURL(ret.Maratona.Titulo.Poster)
		ret.Maratona.Titulo.Backdrop = posterURL(ret.Maratona.Titulo.Backdrop)
	}
	if ret.Juntos, err = s.db.CompanhiasDoAno(r.Context(), user.ID, ret.Ano); err != nil {
		ret.Juntos = []db.Companhia{}
	}
	writeJSON(w, http.StatusOK, ret)
}
