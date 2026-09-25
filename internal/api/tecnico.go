package api

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"nas/internal/cloud"
	"nas/internal/db"
)

// A tela técnica só lê. As consultas à AWS custam (ListObjects paginado) e
// ficam em cache por um minuto; ?atualizar=1 força.
const cacheDoTecnico = time.Minute

type pulso struct {
	estadoModo
	Conexoes         int64      `json:"conexoes"`
	CertificadoVence *time.Time `json:"certificado_vence,omitempty"`
	CertificadoCN    string     `json:"certificado_cn,omitempty"`
	EventosPendentes int64      `json:"eventos_pendentes"`
	UltimaReconc     string     `json:"ultima_reconciliacao,omitempty"`
	WorkerVisto      string     `json:"worker_visto,omitempty"`
}

type filaTecnica struct {
	Papel string `json:"papel"` // jobs | eventos | dlq
	cloud.EstadoDaFila
	Erro string `json:"erro,omitempty"`
}

type daNuvem struct {
	Indisponivel bool            `json:"indisponivel"`
	Motivo       string          `json:"motivo,omitempty"`
	Bucket       *cloud.Inspecao `json:"bucket,omitempty"`
	Filas        []filaTecnica   `json:"filas"`
	Em           time.Time       `json:"em"`
}

type cacheTecnico struct {
	mu    sync.Mutex
	valor *daNuvem
}

var tecnico cacheTecnico

func (s *Server) handleTecnico(w http.ResponseWriter, r *http.Request) {
	est := s.sincro.Estado(r.Context())
	p := pulso{
		estadoModo:       s.estadoModo(),
		Conexoes:         s.nuvem.Conexoes(),
		EventosPendentes: est.EventosPendentes,
		UltimaReconc:     est.UltimaReconc,
		WorkerVisto:      s.db.EstadoNuvem(r.Context(), "worker_visto"),
	}
	if vence, cn, err := certificadoDoMac(); err == nil {
		p.CertificadoVence, p.CertificadoCN = &vence, cn
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pulso": p,
		"nuvem": s.inspecionar(r.Context(), r.URL.Query().Get("atualizar") == "1"),
	})
}

// certificadoDoMac lê o certificado do Roles Anywhere (scripts/roles-anywhere.sh).
func certificadoDoMac() (time.Time, string, error) {
	dir := os.Getenv("NAS_PKI")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return time.Time{}, "", err
		}
		dir = filepath.Join(home, ".nas", "pki")
	}
	dados, err := os.ReadFile(filepath.Join(dir, "mac.pem"))
	if err != nil {
		return time.Time{}, "", err
	}
	bloco, _ := pem.Decode(dados)
	if bloco == nil {
		return time.Time{}, "", fmt.Errorf("mac.pem sem bloco PEM")
	}
	cert, err := x509.ParseCertificate(bloco.Bytes)
	if err != nil {
		return time.Time{}, "", err
	}
	return cert.NotAfter, cert.Subject.CommonName, nil
}

// inspecionar consulta bucket e filas. No modo local não faz chamada nenhuma
// (nem tentaria: o guard recusaria e o contador de bloqueadas subiria à toa).
func (s *Server) inspecionar(ctx context.Context, forcar bool) daNuvem {
	arm, nctx, ok := s.nuvem.Hibrido()
	if !ok {
		return daNuvem{Indisponivel: true, Motivo: "modo local", Filas: []filaTecnica{}, Em: time.Now()}
	}
	tecnico.mu.Lock()
	defer tecnico.mu.Unlock()
	if !forcar && tecnico.valor != nil && time.Since(tecnico.valor.Em) < cacheDoTecnico {
		return *tecnico.valor
	}

	cctx, cancel := juntos(ctx, nctx)
	defer cancel()
	cctx, pare := context.WithTimeout(cctx, 45*time.Second)
	defer pare()

	out := daNuvem{Filas: []filaTecnica{}, Em: time.Now()}
	insp, ok := arm.(cloud.Inspetor)
	if !ok {
		out.Indisponivel, out.Motivo = true, "este adapter não sabe se inspecionar"
		return out
	}
	b := insp.Inspecionar(cctx)
	out.Bucket = &b

	cfg := s.ConfigNuvem()
	for _, f := range []struct{ papel, url string }{{"jobs", cfg.FilaJobs}, {"eventos", cfg.FilaEventos}} {
		if f.url == "" {
			continue
		}
		e, err := insp.EstadoDaFila(cctx, f.url)
		ft := filaTecnica{Papel: f.papel, EstadoDaFila: e}
		if err != nil {
			ft.Nome, ft.Erro = filepath.Base(f.url), err.Error()
		}
		out.Filas = append(out.Filas, ft)
		if e.DLQ != "" {
			d, err := insp.EstadoDaFila(cctx, e.DLQ)
			fd := filaTecnica{Papel: "dlq de " + f.papel, EstadoDaFila: d}
			if err != nil {
				fd.Nome, fd.Erro = filepath.Base(e.DLQ), err.Error()
			}
			out.Filas = append(out.Filas, fd)
		}
	}
	// Um corte no meio da consulta não deve ficar em cache como verdade.
	if s.nuvem.Modo() == cloud.ModoHibrido {
		tecnico.valor = &out
	}
	return out
}

func (s *Server) handleDiario(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	up, _ := strconv.ParseInt(q.Get("upload"), 10, 64)
	antes, _ := strconv.ParseInt(q.Get("antes"), 10, 64)
	limite, _ := strconv.Atoi(q.Get("limite"))
	notas, err := s.db.Diario(r.Context(), db.FiltroDoDiario{Tipo: q.Get("tipo"), UploadID: up, Antes: antes, Limite: limite})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notas)
}

// handleDiarioEventos manda as notas novas ao vivo, no mesmo formato do SSE do modo.
func (s *Server) handleDiarioEventos(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, ": ouvindo\n\n")
	flusher.Flush()

	ch, sair := db.AssinarDiario()
	defer sair()
	batida := time.NewTicker(25 * time.Second)
	defer batida.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case n := <-ch:
			if payload, err := json.Marshal(n); err == nil {
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		case <-batida.C:
			fmt.Fprint(w, ": pulso\n\n")
			flusher.Flush()
		}
	}
}

// diarioDoModo anota cada virada do kill switch e, a cada 10 s, as chamadas
// que o guard recusou (em lote: um corte com o motor rodando recusa muitas).
func (s *Server) diarioDoModo(ctx context.Context) {
	if n, err := s.db.PodaDiario(ctx, 30*24*time.Hour); err == nil && n > 0 {
		s.db.Anota(ctx, "diario.poda", 0, 0, "", map[string]any{"apagadas": n})
	}
	ch, sair := s.nuvem.Assinar()
	defer sair()
	visto := s.nuvem.Bloqueadas()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
			dados := map[string]any{"conexoes": s.nuvem.Conexoes()}
			if e.Erro != "" {
				dados["erro"] = e.Erro
			}
			s.db.Anota(ctx, "modo."+string(e.Modo), 0, 0, "", dados)
		case <-t.C:
			if agora := s.nuvem.Bloqueadas(); agora > visto {
				s.db.Anota(ctx, "modo.bloqueadas", 0, 0, "", map[string]any{"n": agora - visto, "total": agora})
				visto = agora
			}
		}
	}
}
