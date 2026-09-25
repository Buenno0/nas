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
	"strings"
	"sync"
	"time"

	"nas/internal/auth"
	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/energia"
	"nas/internal/media"
	"nas/internal/metrics"
	"nas/internal/sincro"
	"nas/internal/web"
)

// Options são as diferenças de comportamento entre os modos local e tunnel.
type Options struct {
	SecureCookies bool // cookie só por HTTPS (tunnel)
	TrustProxy    bool // confiar no CF-Connecting-IP vindo do loopback
	// Nuvem é o kill switch do modo híbrido. Nil vira uma chave travada em
	// LOCAL, que é exatamente o Ozymandias de antes do híbrido.
	Nuvem *cloud.Chave
	// NaNuvem: esta é a instância cloud (nas serve --nuvem). Ela não tem
	// disco de mídia nem é dona de usuários; o que só o Mac faz é recusado.
	NaNuvem bool
	// Energia informa bateria e temperatura do Mac para o bursting. Nil = um
	// Mac sempre na tomada e frio, que nunca manda preparo para a nuvem.
	Energia *energia.Leitor
}

// Server agrupa as dependências dos handlers.
type Server struct {
	// cfg muda em tempo de execução (a chave do TMDB é colada pela interface),
	// então é protegida por mutex.
	cfgMu sync.RWMutex
	cfg   config.Config

	db      *db.DB
	auth    *auth.Service
	devices *devicePairings
	opts    Options
	scan    scanState
	ruinas  ruinas
	nuvem   *cloud.Chave
	sincro  *sincro.Motor

	// preparador cuida da transcodificação sob demanda; fundo é o contexto do
	// servidor, para um preparo sobreviver à requisição que o pediu mas morrer
	// junto com o processo.
	preparador *media.Preparador
	fundo      context.Context

	// Telemetria: tudo em memória, reseta a cada execução.
	//
	// O amostrador de processo guarda estado entre leituras (o delta de
	// CPU-time), então um único goroutine o consulta; os handlers só leem a
	// última amostra pronta em processoAtual.
	coletor     *metrics.Coletor
	amostrador  *metrics.AmostradorProcesso
	processoMu  sync.RWMutex
	processoUlt metrics.ProcessoAmostra
	startedAt   time.Time
}

// ffmpegAvailable diz à interface se dá para gerar capas e miniaturas.
func (s *Server) ffmpegAvailable() bool {
	_, err := media.FFmpegPath()
	return err == nil
}

func New(cfg config.Config, database *db.DB, opts Options) *Server {
	dir, err := config.PrepareDir()
	if err != nil {
		log.Printf("cache de preparo indisponível: %v", err)
	}
	if opts.Energia == nil {
		opts.Energia = energia.Fixo(energia.Estado{})
	}
	chave := opts.Nuvem
	if chave == nil {
		chave = cloud.Nova(func() config.Nuvem { return config.Nuvem{} }, true)
	}
	srv := &Server{}
	motor := sincro.Novo(database, chave, func() int64 { return int64(cfg.ReservaGB * 1e9) }, func() config.Nuvem {
		return srv.ConfigNuvem()
	})
	*srv = Server{
		nuvem:   chave,
		sincro:  motor,
		cfg:     cfg,
		db:      database,
		auth:    auth.NewService(database),
		devices: newDevicePairings(),
		opts:    opts,
		preparador: media.NovoPreparador(
			dir,
			int64(cfg.CacheGB*1e9),
			int64(cfg.ReservaGB*1e9),
			cfg.Trabalhos,
		),
		coletor:    metrics.NovoColetor(),
		amostrador: metrics.NovoAmostradorProcesso(),
		// Substituído pelo contexto real em Serve; até lá, nada roda.
		fundo: context.Background(),
	}
	return srv
}

// Auth expõe o serviço de autenticação para a CLI (criar usuário inicial).
func (s *Server) Auth() *auth.Service { return s.auth }

// Handler devolve o roteador completo já com os middlewares aplicados.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	return s.logRequests(mux)
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)

	// Público: o login e as ruínas (erros de propósito, para ver as telas).
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("POST /api/auth/device/start", s.handleDeviceStart)
	mux.HandleFunc("POST /api/auth/device/token", s.handleDeviceToken)
	mux.HandleFunc("GET /ruinas/{codigo}", s.handleRuina)
	mux.HandleFunc("GET /api/ruinas", s.handleRuinasPlacar)

	// Protegido.
	mux.Handle("GET /api/auth/me", s.protected(s.handleMe))
	mux.Handle("POST /api/auth/password", s.soNoMac(s.protected(s.handleChangePassword)))
	mux.Handle("POST /api/auth/device/approve", s.protected(s.handleDeviceApprove))
	// Credencial curta para os players que só sabem abrir uma URL.
	mux.Handle("POST /api/auth/media-token", s.protected(s.handleMediaToken))

	mux.Handle("GET /api/home", s.protected(s.handleHome))
	mux.Handle("GET /api/libraries", s.protected(s.handleLibraries))
	mux.Handle("GET /api/titles", s.protected(s.handleTitles))
	mux.Handle("GET /api/artistas", s.protected(s.handleArtistas))
	mux.Handle("GET /api/artistas/{nome}", s.protected(s.handleArtista))
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
	mux.Handle("POST /api/scan", s.soNoMac(s.adminOnly(s.handleScanStart)))
	mux.Handle("POST /api/metadata", s.soNoMac(s.adminOnly(s.handleMetadataStart)))

	// Telemetria do servidor: só o administrador.
	mux.Handle("GET /api/metrics/status", s.adminOnly(s.handleMetricsStatus))
	mux.Handle("GET /api/metrics/events", s.adminOnly(s.handleMetricsEvents))

	// Coleções: listas montadas à mão, por usuário.
	mux.Handle("GET /api/colecoes", s.protected(s.handleColecoes))
	mux.Handle("POST /api/colecoes", s.protected(s.handleCriarColecao))
	mux.Handle("GET /api/colecoes/{id}", s.protected(s.handleColecao))
	mux.Handle("PUT /api/colecoes/{id}", s.protected(s.handleRenomearColecao))
	mux.Handle("DELETE /api/colecoes/{id}", s.protected(s.handleApagarColecao))
	mux.Handle("POST /api/colecoes/{id}/itens/{titulo}", s.protected(s.handleItemDaColecao))
	mux.Handle("DELETE /api/colecoes/{id}/itens/{titulo}", s.protected(s.handleItemDaColecao))
	mux.Handle("PUT /api/colecoes/{id}/ordem", s.protected(s.handleReordenarColecao))
	mux.Handle("GET /api/titles/{id}/colecoes", s.protected(s.handleColecoesDoTitulo))

	// Modo de nuvem: ver é de todos (a interface mostra o selo), alternar é do
	// admin. O kill switch é o PUT com "local".
	mux.Handle("GET /api/modo", s.protected(s.handleGetModo))
	mux.Handle("PUT /api/modo", s.soNoMac(s.adminOnly(s.handlePutModo)))
	mux.Handle("GET /api/modo/events", s.protected(s.handleModoEvents))

	// Upload direto para o bucket: o navegador envia as partes ao S3 com URLs
	// assinadas, sem passar os bytes pelo Mac.
	mux.Handle("GET /api/uploads", s.adminOnly(s.handleUploads))
	mux.Handle("POST /api/uploads", s.adminOnly(s.handleCriarUpload))
	mux.Handle("POST /api/uploads/{id}/urls", s.adminOnly(s.handleURLsDoUpload))
	mux.Handle("GET /api/uploads/{id}/partes", s.adminOnly(s.handlePartesDoUpload))
	mux.Handle("POST /api/uploads/{id}/concluir", s.adminOnly(s.handleConcluirUpload))
	mux.Handle("DELETE /api/uploads/{id}", s.adminOnly(s.handleAbortarUpload))
	mux.Handle("POST /api/uploads/{id}/diario", s.adminOnly(s.handleDiarioDoUpload))

	// Tela técnica: só leitura, para diagnóstico.
	mux.Handle("GET /api/tecnico", s.adminOnly(s.handleTecnico))
	mux.Handle("GET /api/tecnico/custo", s.adminOnly(s.handleCusto))
	mux.Handle("GET /api/tecnico/diario", s.adminOnly(s.handleDiario))
	mux.Handle("GET /api/tecnico/eventos", s.adminOnly(s.handleDiarioEventos))

	// Localização: fixar, liberar espaço, enviar, remover da nuvem. As duas
	// destrutivas (liberar, remover) só nascem daqui, nunca de um evento.
	mux.Handle("GET /api/sincronizacao", s.adminOnly(s.handleSincronizacao))
	mux.Handle("POST /api/sincronizacao", s.adminOnly(s.handleReconciliar))
	mux.Handle("POST /api/files/{id}/nuvem/{acao}", s.soNoMac(s.adminOnly(s.handleAcaoDeNuvem)))
	mux.Handle("PUT /api/libraries/{id}/espelhada", s.soNoMac(s.adminOnly(s.handleEspelhada)))

	mux.Handle("GET /api/settings", s.protected(s.handleGetSettings))
	mux.Handle("PUT /api/settings", s.soNoMac(s.adminOnly(s.handlePutSettings)))

	// Corrigir a capa de um título melhora o acervo para todo mundo, então
	// qualquer conta pode fazer.
	mux.Handle("GET /api/titles/{id}/matches", s.protected(s.handleMatchSearch))
	mux.Handle("POST /api/titles/{id}/match", s.protected(s.handleMatchApply))

	mux.Handle("GET /api/files/{id}/faixas", s.protected(s.handleFaixas))
	mux.Handle("GET /api/files/{id}/legenda/{idx}", s.midia(s.handleLegenda))
	mux.Handle("GET /api/files/{id}/playback", s.protected(s.handlePlayback))
	mux.Handle("POST /api/files/{id}/prepare", s.protected(s.handlePrepareStart))
	mux.Handle("GET /api/files/{id}/prepare/events", s.protected(s.handlePrepareEvents))

	// Tudo o que um player ou uma <img> abre por URL: sessão normal, ou o
	// token de mídia em ?t= para quem não consegue mandar cookie nem cabeçalho.
	mux.Handle("GET /stream/{id}", s.midia(s.handleStream))
	mux.Handle("GET /preparado/{id}", s.midia(s.handlePreparado))
	mux.Handle("GET /img/file/{id}", s.midia(s.handleFileThumb))
	mux.Handle("GET /img/{kind}/{name}", s.midia(s.handleImage))

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

// soNoMac recusa, na instância cloud, o que só o Mac pode fazer: ele é o dono
// de usuários, bibliotecas e disco, e o snapshot seguinte desfaria a mudança.
func (s *Server) soNoMac(h http.Handler) http.Handler {
	if !s.opts.NaNuvem {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusConflict, "isto só se faz no Mac; a instância cloud recebe pelo snapshot")
	})
}

// protected embrulha um handler com a exigência de sessão válida.
func (s *Server) protected(h http.HandlerFunc) http.Handler {
	return s.auth.Require(h)
}

// adminOnly exige sessão válida de um administrador.
func (s *Server) adminOnly(h http.HandlerFunc) http.Handler {
	return s.auth.RequireAdmin(h)
}

// midia embrulha o que é aberto por URL solta — vídeo, capa, legenda. Aceita a
// sessão como qualquer rota protegida, e além dela o token de mídia.
func (s *Server) midia(h http.HandlerFunc) http.Handler {
	return s.auth.RequireMidia(h)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"time":        time.Now().Format(time.RFC3339),
		"api_version": 2,
		"features":    []string{"device_pairing", "playback_caps_v2"},
		"papel":       s.papel(),
	})
}

func (s *Server) papel() string {
	if s.opts.NaNuvem {
		return "nuvem"
	}
	return "mac"
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

	// A partir daqui os preparos nascem deste contexto: cancelá-lo no shutdown
	// interrompe o ffmpeg em vez de deixá-lo órfão queimando CPU.
	s.fundo = ctx
	s.startedAt = time.Now()
	s.preparador.LimparParciais()

	go s.cleanupLoop(ctx)
	go s.sincro.Rodar(ctx)
	go s.diarioDoModo(ctx)
	go s.amostraLoop(ctx)
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

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duracao := time.Since(start)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, duracao.Round(time.Millisecond))

		// r.Pattern é preenchido pelo ServeMux durante o roteamento e continua
		// legível aqui, depois do ServeHTTP — é por isso que o middleware pode
		// continuar envolvendo o mux inteiro em vez de ser reestruturado.
		// Agregar por padrão ("GET /api/titles/{id}") e não pelo caminho
		// resolvido evita cardinalidade infinita vinda dos IDs.
		s.coletor.Observa(r.Pattern, rec.status, duracao, respostaLonga(rec.Header().Get("Content-Type")))
	})
}

// respostaLonga reconhece as respostas que ficam abertas de propósito —
// streaming de mídia e SSE. Detectar pelo Content-Type, e não por uma lista de
// rotas, faz uma rota de streaming futura já nascer excluída.
func respostaLonga(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "video/") ||
		strings.HasPrefix(ct, "audio/") ||
		strings.HasPrefix(ct, "text/event-stream")
}
