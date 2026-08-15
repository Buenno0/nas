package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/meta"
)

type settingsResponse struct {
	Port           int    `json:"port"`
	TMDBConfigured bool   `json:"tmdb_configured"`
	TMDBLang       string `json:"tmdb_lang"`
	ScanEvery      string `json:"scan_every"`
	Tunnel         string `json:"tunnel_name,omitempty"`
	FFmpeg         bool   `json:"ffmpeg"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()

	writeJSON(w, http.StatusOK, settingsResponse{
		Port:           cfg.Port,
		TMDBConfigured: cfg.TMDBKey != "",
		TMDBLang:       cfg.TMDBLang,
		ScanEvery:      cfg.ScanEvery,
		Tunnel:         cfg.Tunnel,
		FFmpeg:         s.ffmpegAvailable(),
	})
}

type settingsRequest struct {
	// Ponteiros para distinguir "não mandou o campo" de "mandou vazio"
	// (mandar vazio na chave é como se remove a integração).
	TMDBKey   *string `json:"tmdb_key"`
	TMDBLang  *string `json:"tmdb_lang"`
	ScanEvery *string `json:"scan_every"`
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}

	s.cfgMu.Lock()
	cfg := s.cfg
	if req.TMDBKey != nil {
		cfg.TMDBKey = *req.TMDBKey
	}
	if req.TMDBLang != nil && *req.TMDBLang != "" {
		cfg.TMDBLang = *req.TMDBLang
	}
	if req.ScanEvery != nil {
		cfg.ScanEvery = *req.ScanEvery
	}
	if err := config.Save(cfg); err != nil {
		s.cfgMu.Unlock()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.cfg = cfg
	s.cfgMu.Unlock()

	s.handleGetSettings(w, r)
}

// enricher usa sempre a configuração atual: a chave do TMDB pode ter sido
// colada na tela de configurações há um segundo.
func (s *Server) enricher() (*meta.Enricher, error) {
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()
	return meta.NewEnricher(s.db, cfg)
}

type matchCandidate struct {
	TMDBID   int     `json:"tmdb_id"`
	Name     string  `json:"name"`
	Year     int     `json:"year,omitempty"`
	Overview string  `json:"overview,omitempty"`
	Rating   float64 `json:"rating,omitempty"`
	Poster   string  `json:"poster,omitempty"`
}

// handleMatchSearch lista candidatos do TMDB para o usuário corrigir o match.
func (s *Server) handleMatchSearch(w http.ResponseWriter, r *http.Request) {
	titleID := atoi64(r.PathValue("id"))
	title, err := s.db.TitleByID(r.Context(), titleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "título não encontrado")
		return
	}

	enricher, err := s.enricher()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		query = title.Name
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))

	results, err := enricher.SearchCandidates(r.Context(), title.Kind, query, year)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	out := make([]matchCandidate, 0, len(results))
	for _, res := range results {
		poster := ""
		if res.PosterPath != "" {
			poster = "https://image.tmdb.org/t/p/w185" + res.PosterPath
		}
		out = append(out, matchCandidate{
			TMDBID:   res.ID,
			Name:     res.DisplayName(),
			Year:     res.Year(),
			Overview: res.Overview,
			Rating:   res.Rating,
			Poster:   poster,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// handleMatchApply grava a escolha manual.
func (s *Server) handleMatchApply(w http.ResponseWriter, r *http.Request) {
	titleID := atoi64(r.PathValue("id"))

	var req struct {
		TMDBID int `json:"tmdb_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.TMDBID <= 0 {
		writeError(w, http.StatusBadRequest, "informe tmdb_id")
		return
	}

	enricher, err := s.enricher()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := enricher.ApplyManual(r.Context(), titleID, req.TMDBID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "título não encontrado")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
