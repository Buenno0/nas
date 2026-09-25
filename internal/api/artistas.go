package api

import (
	"net/http"
	"net/url"

	"nas/internal/auth"
	"nas/internal/db"
)

type artistaResposta struct {
	Nome    string         `json:"nome"`
	Albuns  []db.TitleCard `json:"albuns"`
	Faixas  []db.FileInfo  `json:"faixas"`
	Duracao float64        `json:"duracao"`
}

// comThumbs traduz os nomes de cache em URLs, como o resto da API faz.
func comThumbs(files []db.FileInfo) []db.FileInfo {
	for i := range files {
		files[i].Thumb = thumbURL(files[i].Thumb)
	}
	return files
}

func (s *Server) handleArtistas(w http.ResponseWriter, r *http.Request) {
	artistas, err := s.db.Artistas(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range artistas {
		artistas[i].Poster = posterURL(artistas[i].Poster)
	}
	writeJSON(w, http.StatusOK, artistas)
}

func (s *Server) handleArtista(w http.ResponseWriter, r *http.Request) {
	// O nome vem no caminho e pode ter espaço, acento e barra ("AC/DC").
	nome, err := url.PathUnescape(r.PathValue("nome"))
	if err != nil || nome == "" {
		writeError(w, http.StatusBadRequest, "artista inválido")
		return
	}

	user, _ := auth.UserFrom(r.Context())

	albuns, err := s.db.AlbunsDoArtista(r.Context(), nome)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(albuns) == 0 {
		writeError(w, http.StatusNotFound, "artista não encontrado")
		return
	}

	faixas, err := s.db.FaixasDoArtista(r.Context(), nome, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var total float64
	for _, f := range faixas {
		total += f.Duration
	}

	writeJSON(w, http.StatusOK, artistaResposta{
		Nome:    nome,
		Albuns:  withImageURLs(albuns),
		Faixas:  comThumbs(faixas),
		Duracao: total,
	})
}
