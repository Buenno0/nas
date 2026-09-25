package api

import (
	"net/http"
	"strconv"
	"sync"

	"nas/internal/web"
)

// As ruínas: rotas que devolvem erro de propósito, para as telas de 401, 404,
// 500 e as da nuvem (502, 503) poderem ser vistas sem quebrar nada de verdade. São públicas — a graça é
// justamente mandar o link para alguém.
//
// O contador é de memória, some quando o servidor reinicia, e não guarda nada
// sobre quem clicou.
type ruinas struct {
	mu     sync.Mutex
	visits map[int]int
}

var codigosPermitidos = map[int]bool{401: true, 404: true, 500: true, 502: true, 503: true}

func (s *Server) handleRuina(w http.ResponseWriter, r *http.Request) {
	// A tela da instância cloud divide o 503 com a do modo local.
	if r.PathValue("codigo") == "no-mac" {
		s.registraRuina(503)
		web.ServeTela(w, r, "503-no-mac", http.StatusServiceUnavailable)
		return
	}
	code, err := strconv.Atoi(r.PathValue("codigo"))
	if err != nil || !codigosPermitidos[code] {
		// Pedir uma ruína que não existe é, ele mesmo, um 404.
		s.registraRuina(404)
		web.ServeError(w, r, http.StatusNotFound)
		return
	}

	s.registraRuina(code)
	web.ServeError(w, r, code)
}

func (s *Server) registraRuina(code int) {
	s.ruinas.mu.Lock()
	defer s.ruinas.mu.Unlock()
	if s.ruinas.visits == nil {
		s.ruinas.visits = make(map[int]int, 3)
	}
	s.ruinas.visits[code]++
}

// handleRuinasPlacar alimenta o "quantos caíram" da página de iscas.
func (s *Server) handleRuinasPlacar(w http.ResponseWriter, r *http.Request) {
	s.ruinas.mu.Lock()
	placar := map[string]int{}
	total := 0
	for code, n := range s.ruinas.visits {
		placar[strconv.Itoa(code)] = n
		total += n
	}
	s.ruinas.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"por_codigo": placar, "total": total})
}
