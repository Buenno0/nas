package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// Nota é uma linha do diário técnico.
type Nota struct {
	ID       int64           `json:"id"`
	Em       int64           `json:"em"` // ms
	Tipo     string          `json:"tipo"`
	UploadID *int64          `json:"upload_id,omitempty"`
	FileID   *int64          `json:"file_id,omitempty"`
	Nome     string          `json:"nome,omitempty"`
	Dados    json.RawMessage `json:"dados"`
}

// Ouvintes do diário (o SSE da tela técnica). Global ao processo: há um banco
// por servidor, e perder uma nota num ouvinte lento não quebra nada.
var (
	diarioMu       sync.Mutex
	diarioOuvintes = map[chan Nota]struct{}{}
)

// AssinarDiario entrega cada nota nova.
func AssinarDiario() (<-chan Nota, func()) {
	ch := make(chan Nota, 64)
	diarioMu.Lock()
	diarioOuvintes[ch] = struct{}{}
	diarioMu.Unlock()
	return ch, func() {
		diarioMu.Lock()
		delete(diarioOuvintes, ch)
		diarioMu.Unlock()
	}
}

func nulo(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// Anota grava no diário. Nunca falha para quem chama: o diário é para
// diagnóstico e não pode derrubar um envio.
func (d *DB) Anota(ctx context.Context, tipo string, uploadID, fileID int64, nome string, dados any) {
	if dados == nil {
		dados = map[string]any{}
	}
	corpo, _ := json.Marshal(dados)
	n := Nota{Em: time.Now().UnixMilli(), Tipo: tipo, Nome: nome, Dados: corpo}
	res, err := d.ExecContext(context.WithoutCancel(ctx),
		`INSERT INTO diario (em, tipo, upload_id, file_id, nome, dados) VALUES (?, ?, ?, ?, ?, ?)`,
		n.Em, tipo, nulo(uploadID), nulo(fileID), nome, string(corpo))
	if err != nil {
		log.Printf("diário (%s): %v", tipo, err)
		return
	}
	n.ID, _ = res.LastInsertId()
	if uploadID != 0 {
		n.UploadID = &uploadID
	}
	if fileID != 0 {
		n.FileID = &fileID
	}
	diarioMu.Lock()
	for ch := range diarioOuvintes {
		select {
		case ch <- n:
		default:
		}
	}
	diarioMu.Unlock()
}

// FiltroDoDiario: tipo por prefixo ("envio." pega todos os de envio), antes
// para paginar para trás.
type FiltroDoDiario struct {
	Tipo     string
	UploadID int64
	Antes    int64
	Limite   int
}

func (d *DB) Diario(ctx context.Context, f FiltroDoDiario) ([]Nota, error) {
	if f.Limite <= 0 || f.Limite > 500 {
		f.Limite = 200
	}
	q := `SELECT id, em, tipo, upload_id, file_id, nome, dados FROM diario WHERE 1=1`
	var args []any
	if f.Tipo != "" {
		q += ` AND tipo LIKE ? || '%'`
		args = append(args, f.Tipo)
	}
	if f.UploadID != 0 {
		q += ` AND upload_id = ?`
		args = append(args, f.UploadID)
	}
	if f.Antes != 0 {
		q += ` AND id < ?`
		args = append(args, f.Antes)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, f.Limite)
	rows, err := d.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Nota{}
	for rows.Next() {
		var (
			n        Nota
			up, file sql.NullInt64
			dados    string
		)
		if err := rows.Scan(&n.ID, &n.Em, &n.Tipo, &up, &file, &n.Nome, &dados); err != nil {
			return nil, err
		}
		if up.Valid {
			n.UploadID = &up.Int64
		}
		if file.Valid {
			n.FileID = &file.Int64
		}
		n.Dados = json.RawMessage(dados)
		out = append(out, n)
	}
	return out, rows.Err()
}

// PodaDiario apaga as notas mais velhas que a idade dada.
func (d *DB) PodaDiario(ctx context.Context, idade time.Duration) (int64, error) {
	res, err := d.ExecContext(ctx, `DELETE FROM diario WHERE em < ?`, time.Now().Add(-idade).UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
