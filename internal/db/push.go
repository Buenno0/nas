package db

import (
	"context"
	"strings"
	"time"
)

// InscricaoPush é um aparelho que aceitou receber avisos.
type InscricaoPush struct {
	ID       int64
	UserID   int64
	Endpoint string
	P256dh   string
	Auth     string
	Tipos    []string
}

func (d *DB) SalvaInscricao(ctx context.Context, i InscricaoPush) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO push_inscricoes (user_id, endpoint, p256dh, auth, tipos, criado) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh,
		       auth = excluded.auth, tipos = excluded.tipos, falhas = 0`,
		i.UserID, i.Endpoint, i.P256dh, i.Auth, strings.Join(i.Tipos, ","), time.Now().Unix())
	return err
}

// RemoveInscricao apaga pelo endpoint; userID 0 = de qualquer um (quando o
// serviço de push diz que ela morreu).
func (d *DB) RemoveInscricao(ctx context.Context, endpoint string, userID int64) error {
	if userID == 0 {
		_, err := d.ExecContext(ctx, `DELETE FROM push_inscricoes WHERE endpoint = ?`, endpoint)
		return err
	}
	_, err := d.ExecContext(ctx, `DELETE FROM push_inscricoes WHERE endpoint = ? AND user_id = ?`, endpoint, userID)
	return err
}

// InscricaoDo diz se o endpoint está inscrito, e em quais tipos.
func (d *DB) InscricaoDo(ctx context.Context, userID int64, endpoint string) ([]string, bool) {
	var tipos string
	if err := d.QueryRowContext(ctx, `SELECT tipos FROM push_inscricoes WHERE user_id = ? AND endpoint = ?`, userID, endpoint).Scan(&tipos); err != nil {
		return nil, false
	}
	return strings.Split(tipos, ","), true
}

// Inscricoes para um tipo de aviso. paraAdmins manda a todos os admins;
// senão, aos userIDs dados (vazio = todo mundo).
func (d *DB) Inscricoes(ctx context.Context, tipo string, paraAdmins bool, userIDs []int64) ([]InscricaoPush, error) {
	q := `SELECT i.id, i.user_id, i.endpoint, i.p256dh, i.auth, i.tipos
	        FROM push_inscricoes i JOIN users u ON u.id = i.user_id
	       WHERE (',' || i.tipos || ',') LIKE '%,' || ? || ',%'`
	args := []any{tipo}
	switch {
	case paraAdmins:
		q += ` AND u.is_admin = 1`
	case len(userIDs) > 0:
		q += ` AND i.user_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",") + `)`
		for _, id := range userIDs {
			args = append(args, id)
		}
	}
	rows, err := d.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InscricaoPush
	for rows.Next() {
		var i InscricaoPush
		var tipos string
		if err := rows.Scan(&i.ID, &i.UserID, &i.Endpoint, &i.P256dh, &i.Auth, &tipos); err != nil {
			return nil, err
		}
		i.Tipos = strings.Split(tipos, ",")
		out = append(out, i)
	}
	return out, rows.Err()
}

// FalhaDaInscricao conta uma entrega que falhou; depois de 10 seguidas, a
// inscrição é descartada (aparelho sumido, navegador reinstalado).
func (d *DB) FalhaDaInscricao(ctx context.Context, id int64) {
	_, _ = d.ExecContext(ctx, `UPDATE push_inscricoes SET falhas = falhas + 1 WHERE id = ?`, id)
	_, _ = d.ExecContext(ctx, `DELETE FROM push_inscricoes WHERE id = ? AND falhas >= 10`, id)
}

func (d *DB) EntregaDaInscricao(ctx context.Context, id int64) {
	_, _ = d.ExecContext(ctx, `UPDATE push_inscricoes SET falhas = 0 WHERE id = ? AND falhas > 0`, id)
}
