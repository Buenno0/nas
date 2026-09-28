package db

import (
	"context"
	"strconv"
	"time"
)

// RegistraJuntos soma, para cada par de quem estava na sala, os segundos
// vistos juntos hoje. Nomes que não são usuários daqui são ignorados.
func (d *DB) RegistraJuntos(ctx context.Context, nomes []string, quando time.Time, segundos float64) {
	dia := quando.Local().Format("2006-01-02")
	for _, eu := range nomes {
		u, err := d.UserByName(ctx, eu)
		if err != nil {
			continue
		}
		for _, com := range nomes {
			if com == eu {
				continue
			}
			_, _ = d.ExecContext(ctx, `
				INSERT INTO juntos_historico (user_id, com, dia, segundos) VALUES (?, ?, ?, ?)
				ON CONFLICT DO UPDATE SET segundos = segundos + excluded.segundos`,
				u.ID, com, dia, segundos)
		}
	}
}

// Companhia é alguém com quem se assistiu junto e por quanto tempo.
type Companhia struct {
	Nome     string  `json:"nome"`
	Segundos float64 `json:"segundos"`
}

// CompanhiasDoAno lista com quem o usuário mais assistiu junto no ano.
func (d *DB) CompanhiasDoAno(ctx context.Context, userID int64, ano int) ([]Companhia, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT com, SUM(segundos) AS s FROM juntos_historico
		 WHERE user_id = ? AND substr(dia, 1, 4) = ?
		 GROUP BY com ORDER BY s DESC LIMIT 5`, userID, strconv.Itoa(ano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Companhia{}
	for rows.Next() {
		var c Companhia
		if err := rows.Scan(&c.Nome, &c.Segundos); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
