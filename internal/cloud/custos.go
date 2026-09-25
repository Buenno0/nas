package cloud

import (
	"context"
	"time"
)

// Custo é o gasto da conta AWS no mês corrente, para o painel de custo.
type Custo struct {
	Mes        string           `json:"mes"` // 2026-09
	Moeda      string           `json:"moeda"`
	Total      float64          `json:"total"`
	PorServico []CustoDoServico `json:"por_servico"`
	PorDia     []CustoDoDia     `json:"por_dia"`
	// Do AWS Budgets: previsão até o fim do mês e o limite do alerta.
	Previsao *float64          `json:"previsao,omitempty"`
	Limite   *float64          `json:"limite,omitempty"`
	Em       time.Time         `json:"em"`
	Erros    map[string]string `json:"erros,omitempty"`
}

type CustoDoServico struct {
	Servico string  `json:"servico"`
	Valor   float64 `json:"valor"`
}

type CustoDoDia struct {
	Dia   string  `json:"dia"` // 2026-09-25
	Valor float64 `json:"valor"`
}

// Contador lê o Cost Explorer e o Budgets. Opcional no adapter.
type Contador interface {
	CustoDoMes(ctx context.Context, orcamento string) Custo
}
