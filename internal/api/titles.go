package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nas/internal/auth"
	"nas/internal/db"
	"nas/internal/media"
	"nas/internal/sincro"
)

// posterURL e thumbURL traduzem o nome do arquivo em cache para uma URL que o
// navegador consegue pedir. O front nunca fala com o TMDB direto.
func posterURL(name string) string {
	if name == "" {
		return ""
	}
	return "/img/posters/" + name
}

func thumbURL(name string) string {
	if name == "" {
		return ""
	}
	return "/img/thumbs/" + name
}

func withImageURLs(cards []db.TitleCard) []db.TitleCard {
	for i := range cards {
		cards[i].Poster = posterURL(cards[i].Poster)
		cards[i].Backdrop = posterURL(cards[i].Backdrop)
	}
	return cards
}

type homeRow struct {
	Key   string         `json:"key"`
	Title string         `json:"title"`
	Items []db.TitleCard `json:"items"`
}

type homeResponse struct {
	Hero     *db.TitleCard     `json:"hero,omitempty"`
	Continue []db.ContinueItem `json:"continue"`
	// Esquecidos é o que foi começado há mais de 30 dias e nunca terminou.
	Esquecidos []db.ContinueItem `json:"esquecidos,omitempty"`
	// Fotos deste mesmo dia, em anos anteriores.
	NoDia []db.FotoDoDia `json:"no_dia,omitempty"`
	Rows  []homeRow      `json:"rows"`
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := auth.UserFrom(ctx)

	cont, err := s.db.ContinueWatching(ctx, user.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range cont {
		cont[i].Poster = posterURL(cont[i].Poster)
		cont[i].Backdrop = posterURL(cont[i].Backdrop)
	}

	recent, err := s.db.RecentTitles(ctx, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	recent = withImageURLs(recent)

	resp := homeResponse{Continue: cont, Rows: []homeRow{}}
	if len(recent) > 0 {
		resp.Rows = append(resp.Rows, homeRow{Key: "recent", Title: "Adicionados recentemente", Items: recent})
		hero := recent[0]
		resp.Hero = &hero
	}

	favorites, err := s.db.FavoriteTitles(ctx, user.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(favorites) > 0 {
		resp.Rows = append(resp.Rows, homeRow{Key: "favorites", Title: "Meus favoritos", Items: withImageURLs(favorites)})
	}

	// As prateleiras abaixo falham em silêncio: uma consulta com problema não
	// pode derrubar a home inteira, que é a primeira tela do aplicativo.
	if esquecidos, err := s.db.Esquecidos(ctx, user.ID, 12); err == nil && len(esquecidos) > 0 {
		for i := range esquecidos {
			esquecidos[i].Poster = posterURL(esquecidos[i].Poster)
			esquecidos[i].Backdrop = posterURL(esquecidos[i].Backdrop)
		}
		resp.Esquecidos = esquecidos
	} else if err != nil {
		log.Printf("prateleira de esquecidos: %v", err)
	}

	if fotos, err := s.db.FotosDoDia(ctx, 20); err == nil {
		resp.NoDia = fotos
	} else {
		log.Printf("prateleira do dia: %v", err)
	}

	if acaso, err := s.db.TitulosAoAcaso(ctx, user.ID, 20); err == nil && len(acaso) > 0 {
		resp.Rows = append(resp.Rows, homeRow{
			Key: "acaso", Title: "Você nunca abriu", Items: withImageURLs(acaso),
		})
	} else if err != nil {
		log.Printf("prateleira ao acaso: %v", err)
	}

	libs, err := s.db.Libraries(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		items, err := s.db.ListTitles(ctx, db.TitleFilter{LibraryID: lib.ID, Limit: 20, Sort: "recent"})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(items) == 0 {
			continue
		}
		resp.Rows = append(resp.Rows, homeRow{
			Key:   "lib-" + strconv.FormatInt(lib.ID, 10),
			Title: lib.Name,
			Items: withImageURLs(items),
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.db.Libraries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if libs == nil {
		libs = []db.Library{}
	}

	// Quem não é admin recebe os nomes, não os caminhos: a estrutura de pastas
	// do dono da máquina não é da conta de quem só quer assistir.
	if user, ok := auth.UserFrom(r.Context()); !ok || !user.IsAdmin {
		for i := range libs {
			libs[i].Path = ""
		}
	}

	writeJSON(w, http.StatusOK, libs)
}

type titlesResponse struct {
	Items  []db.TitleCard `json:"items"`
	Total  int            `json:"total"`
	Offset int            `json:"offset"`
	Limit  int            `json:"limit"`
}

func (s *Server) handleTitles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := db.TitleFilter{
		Query:  q.Get("q"),
		Sort:   q.Get("sort"),
		Limit:  atoiDefault(q.Get("limit"), 60),
		Offset: atoiDefault(q.Get("offset"), 0),
	}
	if v := q.Get("library"); v != "" {
		filter.LibraryID = atoi64(v)
	}
	if v := q.Get("kind"); v != "" {
		filter.Kind = db.TitleKind(v)
	}

	items, err := s.db.ListTitles(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := s.db.CountTitles(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, titlesResponse{
		Items:  withImageURLs(items),
		Total:  total,
		Offset: filter.Offset,
		Limit:  filter.Limit,
	})
}

type season struct {
	Number   int           `json:"number"`
	Episodes []db.FileInfo `json:"episodes"`
}

type titleDetail struct {
	db.Title
	PosterURL   string        `json:"poster_url,omitempty"`
	BackdropURL string        `json:"backdrop_url,omitempty"`
	Library     string        `json:"library"`
	Favorite    bool          `json:"favorite"`
	Files       []db.FileInfo `json:"files"`
	Seasons     []season      `json:"seasons,omitempty"`
}

func (s *Server) handleTitle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := auth.UserFrom(ctx)

	id := atoi64(r.PathValue("id"))
	title, err := s.db.TitleByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "título não encontrado")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	files, err := s.db.TitleFiles(ctx, id, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range files {
		files[i].Thumb = thumbURL(files[i].Thumb)
		files[i].PreparoNuvem = s.preparoNaNuvem(ctx, files[i])
	}

	fav, err := s.db.IsFavorite(ctx, user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	detail := titleDetail{
		Title:       title,
		PosterURL:   posterURL(title.Poster),
		BackdropURL: posterURL(title.Backdrop),
		Favorite:    fav,
		Files:       files,
	}
	if lib, err := s.db.Library(ctx, title.LibraryID); err == nil {
		detail.Library = lib.Name
	}
	if title.Kind == db.TitleTV {
		detail.Seasons = groupSeasons(files)
	}

	writeJSON(w, http.StatusOK, detail)
}

// groupSeasons organiza os arquivos de uma série em temporadas, preservando a
// ordem já vinda do banco.
func groupSeasons(files []db.FileInfo) []season {
	var seasons []season
	index := map[int]int{}
	for _, f := range files {
		pos, ok := index[f.Season]
		if !ok {
			seasons = append(seasons, season{Number: f.Season})
			pos = len(seasons) - 1
			index[f.Season] = pos
		}
		seasons[pos].Episodes = append(seasons[pos].Episodes, f)
	}
	return seasons
}

type progressRequest struct {
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	fileID := atoi64(r.PathValue("id"))

	var req progressRequest
	// sendBeacon manda text/plain no unload; o corpo continua sendo JSON.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}
	if req.Position < 0 {
		req.Position = 0
	}
	if err := s.db.SaveProgress(r.Context(), user.ID, fileID, req.Position, req.Duration); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFavorite(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	titleID := atoi64(r.PathValue("id"))
	on := r.Method == http.MethodPost

	if err := s.db.SetFavorite(r.Context(), user.ID, titleID, on); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": on})
}

// handleFile entrega ao player tudo sobre um arquivo: título, progresso e
// dados técnicos.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())

	info, err := s.db.PlaybackByFile(r.Context(), atoi64(r.PathValue("id")), user.ID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "arquivo não encontrado")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	info.Poster = posterURL(info.Poster)
	info.Thumb = thumbURL(info.Thumb)
	writeJSON(w, http.StatusOK, info)
}

// handleNext devolve o arquivo do próximo episódio, para o player emendar.
func (s *Server) handleNext(w http.ResponseWriter, r *http.Request) {
	fileID := atoi64(r.PathValue("id"))
	next, err := s.db.NextEpisode(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"next": nil})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"next": next})
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// preparoNaNuvem diz se o worker está gerando (ou já gerou) a versão que
// toca em qualquer navegador, para a página do título avisar antes do play.
func (s *Server) preparoNaNuvem(ctx context.Context, f db.FileInfo) string {
	if f.Localizacao != db.LocalNuvem && f.Localizacao != db.LocalAmbos {
		return ""
	}
	if f.Type != db.TypeVideo {
		return ""
	}
	if k, err := s.db.DerivadoDe(ctx, f.ID, "compat", 0); err == nil && k != "" {
		return "pronto"
	}
	estado, _, quando := s.db.ProcessamentoDesde(ctx, f.ID)
	switch {
	case estado == "falhou":
		return "falhou"
	case estado == "pedido" && time.Since(quando) < sincro.EsperaPeloWorker:
		// Sem probe ainda (acabou de chegar): o worker é quem vai dizer.
		if f.ACodec == "" {
			return "preparando"
		}
		plano := media.DecideWithSize(f.Ext, f.VCodec, "", "", f.ACodec, f.Width, f.Height, false, media.Caps{})
		if plano.Mode != media.ModeDirect {
			return "preparando"
		}
	}
	return ""
}
