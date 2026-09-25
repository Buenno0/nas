package aws

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"nas/internal/cloud"
)

// CustoDoMes custa US$ 0,01 por chamada ao Cost Explorer (quem chama guarda
// em cache); o Budgets é gratuito. Os dois só respondem em us-east-1.
func (b *bucket) CustoDoMes(ctx context.Context, orcamento string) cloud.Custo {
	agora := time.Now().UTC()
	inicio := time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, time.UTC)
	// O fim é exclusivo; no dia 1 o intervalo seria vazio, e o CE recusa.
	fim := agora.AddDate(0, 0, 1)
	c := cloud.Custo{Mes: inicio.Format("2006-01"), Moeda: "USD", Em: time.Now(),
		PorServico: []cloud.CustoDoServico{}, PorDia: []cloud.CustoDoDia{}, Erros: map[string]string{}}

	regional := func(o *costexplorer.Options) { o.Region = "us-east-1" }
	out, err := costexplorer.NewFromConfig(b.awsCfg, regional).GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
		TimePeriod:  &cetypes.DateInterval{Start: aws.String(inicio.Format("2006-01-02")), End: aws.String(fim.Format("2006-01-02"))},
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		GroupBy:     []cetypes.GroupDefinition{{Type: cetypes.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")}},
	})
	if err != nil {
		c.Erros["cost_explorer"] = err.Error()
	} else {
		servicos := map[string]float64{}
		for _, r := range out.ResultsByTime {
			dia := cloud.CustoDoDia{Dia: aws.ToString(r.TimePeriod.Start)}
			for _, g := range r.Groups {
				m := g.Metrics["UnblendedCost"]
				v, _ := strconv.ParseFloat(aws.ToString(m.Amount), 64)
				if u := aws.ToString(m.Unit); u != "" {
					c.Moeda = u
				}
				if len(g.Keys) > 0 {
					servicos[g.Keys[0]] += v
				}
				dia.Valor += v
			}
			c.Total += dia.Valor
			c.PorDia = append(c.PorDia, dia)
		}
		for s, v := range servicos {
			c.PorServico = append(c.PorServico, cloud.CustoDoServico{Servico: s, Valor: v})
		}
		sort.Slice(c.PorServico, func(i, j int) bool { return c.PorServico[i].Valor > c.PorServico[j].Valor })
	}

	if orcamento != "" {
		id, err := sts.NewFromConfig(b.awsCfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			c.Erros["budgets"] = err.Error()
			return c
		}
		bo, err := budgets.NewFromConfig(b.awsCfg, func(o *budgets.Options) { o.Region = "us-east-1" }).
			DescribeBudget(ctx, &budgets.DescribeBudgetInput{AccountId: id.Account, BudgetName: aws.String(orcamento)})
		if err != nil {
			c.Erros["budgets"] = err.Error()
			return c
		}
		num := func(s *string) *float64 {
			if s == nil {
				return nil
			}
			v, err := strconv.ParseFloat(*s, 64)
			if err != nil {
				return nil
			}
			return &v
		}
		if bo.Budget.BudgetLimit != nil {
			c.Limite = num(bo.Budget.BudgetLimit.Amount)
		}
		if cs := bo.Budget.CalculatedSpend; cs != nil && cs.ForecastedSpend != nil {
			c.Previsao = num(cs.ForecastedSpend.Amount)
		}
	}
	return c
}
