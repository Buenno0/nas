package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"nas/internal/cloud"
)

// Cada consulta ao Cost Explorer custa US$ 0,01 e os números só mudam umas
// poucas vezes por dia: guardado no banco por 6 h (sobrevive a reinícios), e
// "atualizar" respeita um intervalo mínimo de 10 min.
const (
	validadeDoCusto  = 6 * time.Hour
	intervaloDoCusto = 10 * time.Minute
	orcamentoPadrao  = "ozymandias-mensal"
)

var custoMu sync.Mutex

type respostaCusto struct {
	Indisponivel bool         `json:"indisponivel"`
	Motivo       string       `json:"motivo,omitempty"`
	Custo        *cloud.Custo `json:"custo,omitempty"`
	// Do cache: a consulta é antiga, mas a nuvem está desligada agora.
	DoCache bool `json:"do_cache,omitempty"`
}

func (s *Server) custoGuardado(ctx context.Context) *cloud.Custo {
	var c cloud.Custo
	if v := s.db.EstadoNuvem(ctx, "custo"); v != "" && json.Unmarshal([]byte(v), &c) == nil {
		return &c
	}
	return nil
}

func (s *Server) handleCusto(w http.ResponseWriter, r *http.Request) {
	custoMu.Lock()
	defer custoMu.Unlock()
	ctx := r.Context()
	guardado := s.custoGuardado(ctx)

	arm, nctx, ok := s.nuvem.Hibrido()
	if !ok {
		// Mostrar o último número conhecido não chama a AWS: é só o banco.
		writeJSON(w, http.StatusOK, respostaCusto{Indisponivel: guardado == nil, Motivo: "modo local", Custo: guardado, DoCache: guardado != nil})
		return
	}
	idade := time.Duration(1 << 62)
	if guardado != nil {
		idade = time.Since(guardado.Em)
	}
	forcar := r.URL.Query().Get("atualizar") == "1" && idade > intervaloDoCusto
	if guardado != nil && idade < validadeDoCusto && !forcar {
		writeJSON(w, http.StatusOK, respostaCusto{Custo: guardado})
		return
	}
	contador, ok := arm.(cloud.Contador)
	if !ok {
		writeJSON(w, http.StatusOK, respostaCusto{Indisponivel: true, Motivo: "este adapter não lê custos"})
		return
	}
	cctx, cancel := juntos(ctx, nctx)
	defer cancel()
	cctx, pare := context.WithTimeout(cctx, 30*time.Second)
	defer pare()

	orcamento := s.ConfigNuvem().Orcamento
	if orcamento == "" {
		orcamento = orcamentoPadrao
	}
	c := contador.CustoDoMes(cctx, orcamento)
	// Só guarda o que veio de fato do Cost Explorer; um erro de permissão não
	// deve apagar o último número bom.
	if _, falhou := c.Erros["cost_explorer"]; !falhou {
		if corpo, err := json.Marshal(c); err == nil {
			_ = s.db.GravaEstadoNuvem(ctx, "custo", string(corpo))
		}
	}
	writeJSON(w, http.StatusOK, respostaCusto{Custo: &c})
}
