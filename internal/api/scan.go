package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"nas/internal/meta"
	"nas/internal/scan"
)

// scanState acompanha o trabalho de fundo (indexação + metadados) para a
// interface mostrar progresso sem disparar outro por cima.
type scanState struct {
	mu        sync.Mutex
	running   bool
	stage     string // "arquivos" | "metadados"
	startedAt time.Time
	progress  scan.Progress
	lastStats string
	lastRun   time.Time
	lastErr   string
}

type scanStatus struct {
	Running   bool   `json:"running"`
	Stage     string `json:"stage,omitempty"`
	Library   string `json:"library,omitempty"`
	Current   int    `json:"current,omitempty"`
	Total     int    `json:"total,omitempty"`
	File      string `json:"file,omitempty"`
	LastStats string `json:"last_stats,omitempty"`
	LastRun   string `json:"last_run,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// begin marca o início de um trabalho; devolve false se já houver um rodando.
func (s *Server) begin(stage string) bool {
	s.scan.mu.Lock()
	defer s.scan.mu.Unlock()
	if s.scan.running {
		return false
	}
	s.scan.running = true
	s.scan.stage = stage
	s.scan.startedAt = time.Now()
	s.scan.lastErr = ""
	return true
}

func (s *Server) finish(stats string, err error) {
	s.scan.mu.Lock()
	defer s.scan.mu.Unlock()
	s.scan.running = false
	s.scan.stage = ""
	s.scan.progress = scan.Progress{}
	s.scan.lastRun = time.Now()
	if err != nil {
		s.scan.lastErr = err.Error()
		return
	}
	s.scan.lastStats = stats
}

func (s *Server) handleScanStart(w http.ResponseWriter, r *http.Request) {
	if !s.begin("arquivos") {
		writeError(w, http.StatusConflict, "já existe um scan em andamento")
		return
	}

	// O trabalho continua depois que a resposta HTTP termina, mas ainda morre
	// junto com o servidor.
	go s.runScan(context.WithoutCancel(r.Context()))
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

// runScan indexa os arquivos e, em seguida, busca os metadados do que é novo.
func (s *Server) runScan(ctx context.Context) {
	scanner := scan.New(s.db)
	stats, err := scanner.ScanAll(ctx, func(p scan.Progress) {
		s.scan.mu.Lock()
		s.scan.progress = p
		s.scan.mu.Unlock()
	})
	if err != nil {
		log.Printf("scan falhou: %v", err)
		s.finish("", err)
		return
	}

	metaStats := s.runEnrichment(ctx, false)
	s.finish(stats.String()+" · "+metaStats, nil)
}

// runEnrichment busca capas e sinopses; devolve o resumo para o status.
func (s *Server) runEnrichment(ctx context.Context, all bool) string {
	s.scan.mu.Lock()
	s.scan.stage = "metadados"
	s.scan.progress = scan.Progress{}
	s.scan.mu.Unlock()

	enricher, err := s.enricher()
	if err != nil {
		log.Printf("metadados: %v", err)
		return "metadados falharam"
	}

	stats, err := enricher.Run(ctx, all, func(p meta.Progress) {
		s.scan.mu.Lock()
		s.scan.progress = scan.Progress{Library: "metadados", Current: p.Current, Total: p.Total, File: p.Title}
		s.scan.mu.Unlock()
	})
	if err != nil {
		log.Printf("metadados: %v", err)
		return "metadados: " + err.Error()
	}
	return stats.String()
}

func (s *Server) handleMetadataStart(w http.ResponseWriter, r *http.Request) {
	if !s.begin("metadados") {
		writeError(w, http.StatusConflict, "já existe um trabalho em andamento")
		return
	}

	// ?all=1 refaz tudo — útil depois de colar a chave do TMDB.
	all := r.URL.Query().Get("all") != ""
	ctx := context.WithoutCancel(r.Context())
	go func() {
		s.finish(s.runEnrichment(ctx, all), nil)
	}()

	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

// StartBackgroundJobs sobe o watcher das pastas e o scan periódico. Ambos
// passam pelo mesmo caminho do botão "escanear agora", então nunca há dois
// scans ao mesmo tempo.
func (s *Server) StartBackgroundJobs(ctx context.Context) {
	// Um scan logo depois de subir: enquanto o NAS esteve desligado, arquivos
	// podem ter sido movidos ou apagados, e o watcher não viu nada disso.
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		if s.begin("arquivos") {
			s.runScan(ctx)
		}
	}()

	go func() {
		watcher := scan.NewWatcher(s.db, func() {
			if s.begin("arquivos") {
				log.Println("mudança detectada nas bibliotecas: reindexando")
				s.runScan(ctx)
			}
		})
		watcher.Run(ctx)
	}()

	s.cfgMu.RLock()
	interval := s.cfg.ScanInterval()
	s.cfgMu.RUnlock()
	if interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.begin("arquivos") {
					s.runScan(ctx)
				}
			}
		}
	}()
}

// handleScanEvents transmite o progresso por SSE, para a interface mostrar a
// barra sem ficar perguntando de segundo em segundo.
func (s *Server) handleScanEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var last string
	for {
		payload, err := json.Marshal(s.status())
		if err == nil && string(payload) != last {
			last = string(payload)
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) status() scanStatus {
	s.scan.mu.Lock()
	defer s.scan.mu.Unlock()

	st := scanStatus{
		Running:   s.scan.running,
		Stage:     s.scan.stage,
		Library:   s.scan.progress.Library,
		Current:   s.scan.progress.Current,
		Total:     s.scan.progress.Total,
		File:      s.scan.progress.File,
		LastStats: s.scan.lastStats,
		LastError: s.scan.lastErr,
	}
	if !s.scan.lastRun.IsZero() {
		st.LastRun = s.scan.lastRun.Format(time.RFC3339)
	}
	return st
}

func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.status())
}
