package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Um salto maior que isso entre dois avisos de progresso é seek ou pulo, não
// tempo assistido. O player avisa a cada ~10 s; 5 min dá folga para aba em
// segundo plano e rede ruim.
const saltoMaximo = 5 * 60

// SomaHistorico acrescenta ao dia de hoje o que foi assistido entre a posição
// anterior e a nova. Devolve os segundos somados (0 quando não conta).
func (d *DB) SomaHistorico(ctx context.Context, userID, fileID int64, anterior, nova float64, anteriorEm, agora time.Time) float64 {
	delta := nova - anterior
	passou := agora.Sub(anteriorEm).Seconds()
	// Andar para trás, pular longe ou avançar mais do que o relógio andou
	// (seek para a frente) não é tempo assistido.
	if delta <= 0 || delta > saltoMaximo || delta > passou+30 {
		return 0
	}
	d.RegistraHistorico(ctx, userID, fileID, agora, delta)
	return delta
}

// RegistraHistorico grava segundos assistidos num dia/hora. Usado também pela
// federação, quando o outro nó conta o que foi visto por lá.
func (d *DB) RegistraHistorico(ctx context.Context, userID, fileID int64, quando time.Time, segundos float64) {
	local := quando.Local()
	_, _ = d.ExecContext(ctx, `
		INSERT INTO historico (user_id, media_file_id, dia, segundos) VALUES (?, ?, ?, ?)
		ON CONFLICT DO UPDATE SET segundos = segundos + excluded.segundos`,
		userID, fileID, local.Format("2006-01-02"), segundos)
	_, _ = d.ExecContext(ctx, `
		INSERT INTO historico_horas (user_id, ano, hora, segundos) VALUES (?, ?, ?, ?)
		ON CONFLICT DO UPDATE SET segundos = segundos + excluded.segundos`,
		userID, local.Year(), local.Hour(), segundos)
}

// ProgressoAnterior é a posição salva e quando, para calcular o delta.
func (d *DB) ProgressoAnterior(ctx context.Context, userID, fileID int64) (float64, time.Time, bool) {
	var pos float64
	var em int64
	err := d.QueryRowContext(ctx, `SELECT position_sec, updated_at FROM progress WHERE user_id = ? AND media_file_id = ?`,
		userID, fileID).Scan(&pos, &em)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return 0, time.Time{}, false
	}
	return pos, time.Unix(em, 0), true
}
