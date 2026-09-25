package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nas/internal/metrics"
)

// metricsSnapshot é o painel inteiro em uma resposta: recursos do processo,
// disco e tráfego HTTP.
type metricsSnapshot struct {
	UptimeSegundos int64                   `json:"uptime_segundos"`
	Modo           string                  `json:"modo"` // "local" | "tunnel"
	Processo       metrics.ProcessoAmostra `json:"processo"`

	DiscoLivre   int64 `json:"disco_livre_bytes"`
	DiscoTotal   int64 `json:"disco_total_bytes"`
	DiscoReserva int64 `json:"disco_reserva_bytes"`
	CacheUsado   int64 `json:"cache_usado_bytes"`
	CacheLimite  int64 `json:"cache_limite_bytes"`

	Trafego metrics.Resumo `json:"trafego"`
}

// amostraLoop é o ÚNICO chamador de Amostra no processo. O amostrador calcula a
// CPU pela diferença entre duas leituras, então vários leitores concorrentes
// repartiriam a mesma janela e cada um veria uma fração do valor real.
func (s *Server) amostraLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		am := s.amostrador.Amostra()
		s.processoMu.Lock()
		s.processoUlt = am
		s.processoMu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) metricsSnapshot() metricsSnapshot {
	s.processoMu.RLock()
	processo := s.processoUlt
	s.processoMu.RUnlock()

	// O modo é um rótulo, não uma quebra de tráfego: o NAS roda em local OU em
	// tunnel (a exclusividade do lockfile), nunca nos dois, então 100% das
	// requisições de uma execução chegam pelo mesmo caminho.
	modo := "local"
	if s.opts.TrustProxy {
		modo = "tunnel"
	}

	livre, total := s.preparador.Disco()
	usado, limite := s.preparador.Uso()

	snap := metricsSnapshot{
		Modo:         modo,
		Processo:     processo,
		DiscoLivre:   livre,
		DiscoTotal:   total,
		DiscoReserva: s.preparador.Reserva(),
		CacheUsado:   usado,
		CacheLimite:  limite,
		Trafego:      s.coletor.Snapshot(),
	}
	// startedAt só é preenchido em Serve; fora dele (testes) o uptime é zero em
	// vez de "desde 1970".
	if !s.startedAt.IsZero() {
		snap.UptimeSegundos = int64(time.Since(s.startedAt).Seconds())
	}
	return snap
}

func (s *Server) handleMetricsStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metricsSnapshot())
}

// handleMetricsEvents transmite o painel por SSE — uma conexão em vez de uma
// pergunta por segundo. Mesmo formato do SSE do scan.
func (s *Server) handleMetricsEvents(w http.ResponseWriter, r *http.Request) {
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
		payload, err := json.Marshal(s.metricsSnapshot())
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
