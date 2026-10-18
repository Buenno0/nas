package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"nas/internal/estante"
)

// A Estante: o PS5 e a TV da sala, vistos e comandados pela rede do Mac. A
// instância cloud não enxerga a LAN, então lá tudo responde "indisponível".

type estadoEstante struct {
	Disponivel bool `json:"disponivel"`
	estante.Estado
}

// alvosDaEstante lê a config atual: `nas config set estante.*` vale depois de
// reiniciar, como o resto do config.json.
func (s *Server) alvosDaEstante() estante.Alvos {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return estante.Alvos{PS5: s.cfg.Estante.PS5, TV: s.cfg.Estante.TV}
}

func (s *Server) handleEstante(w http.ResponseWriter, r *http.Request) {
	if s.opts.NaNuvem {
		writeJSON(w, http.StatusOK, estadoEstante{})
		return
	}
	writeJSON(w, http.StatusOK, estadoEstante{true, s.estante.Estado(r.Context())})
}

// handleEstanteEventos manda o estado por SSE. Enquanto houver uma conexão
// destas aberta a Central sonda a rede; fechadas todas, ela para.
func (s *Server) handleEstanteEventos(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	envia := func(e estante.Estado) {
		if payload, err := json.Marshal(estadoEstante{true, e}); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}

	ch, sair := s.estante.Assinar()
	defer sair()
	envia(s.estante.Estado(r.Context()))

	pulso := time.NewTicker(25 * time.Second)
	defer pulso.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			envia(e)
		case <-pulso.C:
			fmt.Fprint(w, ": pulso\n\n")
			flusher.Flush()
		}
	}
}

// handleEstanteVolume aceita {"valor": n} (o slider) ou {"passo": ±n} (os
// botões − e +).
func (s *Server) handleEstanteVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Valor *int `json:"valor"`
		Passo *int `json:"passo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || (body.Valor == nil) == (body.Passo == nil) {
		writeError(w, http.StatusBadRequest, `mande "valor" ou "passo"`)
		return
	}
	var e estante.Estado
	var err error
	if body.Valor != nil {
		e, err = s.estante.DefinirVolume(r.Context(), *body.Valor)
	} else {
		e, err = s.estante.Passo(r.Context(), *body.Passo)
	}
	s.respostaDaEstante(w, e, err)
}

func (s *Server) handleEstanteMudo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mudo *bool `json:"mudo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Mudo == nil {
		writeError(w, http.StatusBadRequest, `mande "mudo": true ou false`)
		return
	}
	e, err := s.estante.DefinirMudo(r.Context(), *body.Mudo)
	s.respostaDaEstante(w, e, err)
}

func (s *Server) respostaDaEstante(w http.ResponseWriter, e estante.Estado, err error) {
	switch {
	case errors.Is(err, estante.ErrTVDesligada):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusBadGateway, "a TV não aceitou o comando: "+err.Error())
	default:
		writeJSON(w, http.StatusOK, estadoEstante{true, e})
	}
}
