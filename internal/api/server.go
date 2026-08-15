// Package api monta o servidor HTTP: API JSON, streaming e o SPA embutido.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"nas/internal/auth"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/media"
	"nas/internal/web"
)

// Options são as diferenças de comportamento entre os modos local e tunnel.
type Options struct {
	SecureCookies bool // cookie só por HTTPS (tunnel)
	TrustProxy    bool // confiar no CF-Connecting-IP vindo do loopback
}

// Server agrupa as dependências dos handlers.
type Server struct {
	// cfg muda em tempo de execução (a chave do TMDB é colada pela interface),
	// então é protegida por mutex.
	cfgMu sync.RWMutex
	cfg   config.Config

	db     *db.DB
	auth   *auth.Service
	opts   Options
	scan   scanState
	ruinas ruinas
}

// ffmpegAvailable diz à interface se dá para gerar capas e miniaturas.
func (s *Server) ffmpegAvailable() bool {
	_, err := media.FFmpegPath()
	return err == nil
}

func New(cfg config.Config, database *db.DB, opts Options) *Server {
	return &Server{
		cfg:  cfg,
		db:   database,
		auth: auth.NewService(database),
		opts: opts,
	}
}

// Auth expõe o serviço de autenticação para a CLI (criar usuário inicial).
func (s *Server) Auth() *auth.Service { return s.auth }

// Handler devolve o roteador completo já com os middlewares aplicados.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	return logRequests(mux)
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)

	// Público: o login e as ruínas (erros de propósito, para ver as telas).
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /ruinas/{codigo}", s.handleRuina)
	mux.HandleFunc("GET /api/ruinas", s.handleRuinasPlacar)

	// Protegido.
	mux.Handle("GET /api/auth/me", s.protected(s.handleMe))
	mux.Handle("POST /api/auth/password", s.protected(s.handleChangePassword))

	mux.Handle("GET /api/home", s.protected(s.handleHome))
	mux.Handle("GET /api/libraries", s.protected(s.handleLibraries))
	mux.Handle("GET /api/titles", s.protected(s.handleTitles))
	mux.Handle("GET /api/titles/{id}", s.protected(s.handleTitle))
	mux.Handle("GET /api/files/{id}", s.protected(s.handleFile))
	mux.Handle("GET /api/files/{id}/next", s.protected(s.handleNext))
	mux.Handle("PUT /api/progress/{id}", s.protected(s.handleProgress))
	mux.Handle("POST /api/progress/{id}", s.protected(s.handleProgress)) // sendBeacon
	mux.Handle("POST /api/favorites/{id}", s.protected(s.handleFavorite))
	mux.Handle("DELETE /api/favorites/{id}", s.protected(s.handleFavorite))
	// Ver o andamento é de todos; disparar trabalho no servidor é do admin.
	mux.Handle("GET /api/scan/status", s.protected(s.handleScanStatus))
	mux.Handle("GET /api/scan/events", s.protected(s.handleScanEvents))
	mux.Handle("POST /api/scan", s.adminOnly(s.handleScanStart))
	mux.Handle("POST /api/metadata", s.adminOnly(s.handleMetadataStart))

	mux.Handle("GET /api/settings", s.protected(s.handleGetSettings))
	mux.Handle("PUT /api/settings", s.adminOnly(s.handlePutSettings))

	// Corrigir a capa de um título melhora o acervo para todo mundo, então
	// qualquer conta pode fazer.
	mux.Handle("GET /api/titles/{id}/matches", s.protected(s.handleMatchSearch))
	mux.Handle("POST /api/titles/{id}/match", s.protected(s.handleMatchApply))

	mux.Handle("GET /stream/{id}", s.protected(s.handleStream))
	mux.Handle("GET /img/file/{id}", s.protected(s.handleFileThumb))
	mux.Handle("GET /img/{kind}/{name}", s.protected(s.handleImage))

	// O SPA embutido responde por todo o resto.
	if web.Available() {
		mux.Handle("GET /", web.Handler())
	} else {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "NAS no ar, mas sem frontend embutido. Rode `make web` e recompile.")
		})
	}
}

// protected embrulha um handler com a exigência de sessão válida.
func (s *Server) protected(h http.HandlerFunc) http.Handler {
	return s.auth.Require(h)
}

// adminOnly exige sessão válida de um administrador.
func (s *Server) adminOnly(h http.HandlerFunc) http.Handler {
	return s.auth.RequireAdmin(h)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Serve sobe o servidor em addr e desliga graciosamente quando ctx é cancelado.
func (s *Server) Serve(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Sem WriteTimeout: streaming de vídeo mantém respostas longas abertas.
		IdleTimeout: 120 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("ouvindo em %s: %w", addr, err)
	}

	go s.cleanupLoop(ctx)
	s.StartBackgroundJobs(ctx)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return srv.Close()
		}
		return nil
	}
}

// cleanupLoop apaga sessões expiradas de tempos em tempos.
func (s *Server) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()
	for {
		if _, err := s.auth.CleanupExpired(ctx); err != nil {
			log.Printf("limpeza de sessões: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("erro escrevendo JSON: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush e ReadFrom precisam ser repassados: o SSE do scan depende do Flusher e
// o io.Copy do streaming fica bem mais rápido com o ReadFrom do socket.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(r.ResponseWriter, src)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}
