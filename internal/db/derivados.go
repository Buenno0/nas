package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Derivado é um arquivo que o worker gerou a partir de um item da nuvem.
type Derivado struct {
	Tipo    string
	Indice  int
	Key     string
	Receita string
}

// GravaDerivados troca o conjunto inteiro: reprocessar não deixa sobras.
func (d *DB) GravaDerivados(ctx context.Context, fileID int64, ds []Derivado) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM derivados WHERE media_file_id = ?`, fileID); err != nil {
		return err
	}
	for _, x := range ds {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO derivados (media_file_id, tipo, indice, nuvem_key, receita) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT DO UPDATE SET nuvem_key = excluded.nuvem_key, receita = excluded.receita`,
			fileID, x.Tipo, x.Indice, x.Key, x.Receita); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO processamento (media_file_id, estado, erro, atualizado) VALUES (?, 'concluido', '', ?)
		ON CONFLICT DO UPDATE SET estado = 'concluido', erro = '', atualizado = excluded.atualizado`,
		fileID, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// DerivadoDe devolve a chave de um derivado, ou ErrNotFound.
func (d *DB) DerivadoDe(ctx context.Context, fileID int64, tipo string, indice int) (string, error) {
	var key string
	err := d.QueryRowContext(ctx,
		`SELECT nuvem_key FROM derivados WHERE media_file_id = ? AND tipo = ? AND indice = ?`,
		fileID, tipo, indice).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}

// Processamento diz se o Mac já pediu, e como terminou. Vazio: nunca pediu.
func (d *DB) Processamento(ctx context.Context, fileID int64) (estado, erro string) {
	_ = d.QueryRowContext(ctx,
		`SELECT estado, erro FROM processamento WHERE media_file_id = ?`, fileID).Scan(&estado, &erro)
	return estado, erro
}

func (d *DB) MarcaProcessamento(ctx context.Context, fileID int64, estado, erro string) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO processamento (media_file_id, estado, erro, atualizado) VALUES (?, ?, ?, ?)
		ON CONFLICT DO UPDATE SET estado = excluded.estado, erro = excluded.erro, atualizado = excluded.atualizado`,
		fileID, estado, erro, time.Now().Unix())
	return err
}

// FileIDPorNuvemKey acha o arquivo de uma chave do bucket.
func (d *DB) FileIDPorNuvemKey(ctx context.Context, key string) (int64, error) {
	var id int64
	err := d.QueryRowContext(ctx, `SELECT id FROM media_files WHERE nuvem_key = ?`, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}
