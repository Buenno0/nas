package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
)

// Localização de um arquivo (migration 0010).
const (
	LocalLocal    = "local"
	LocalEnviando = "enviando"
	LocalAmbos    = "ambos"
	LocalBaixando = "baixando"
	LocalNuvem    = "nuvem"
)

// SoNaNuvem diz se o arquivo não tem cópia no disco do Mac.
func SoNaNuvem(localizacao string) bool {
	return localizacao == LocalNuvem || localizacao == LocalBaixando
}

// Localizacao devolve onde o arquivo mora e a chave dele no bucket.
func (d *DB) Localizacao(ctx context.Context, fileID int64) (localizacao, key string, err error) {
	err = d.QueryRowContext(ctx,
		`SELECT localizacao, nuvem_key FROM media_files WHERE id = ?`, fileID).Scan(&localizacao, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return localizacao, key, err
}

// MarcaNaNuvem registra que o arquivo agora também existe no bucket.
func (d *DB) MarcaNaNuvem(ctx context.Context, fileID int64, localizacao, key string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE media_files SET localizacao = ?, nuvem_key = ? WHERE id = ?`, localizacao, key, fileID)
	if err == nil {
		d.RegistraEvento(ctx, "localizacao.alterada", map[string]any{
			"file_id": fileID, "localizacao": localizacao, "key": key,
		})
	}
	return err
}

// FileIDPorCaminho acha um arquivo já indexado pelo caminho absoluto.
func (d *DB) FileIDPorCaminho(ctx context.Context, path string) (int64, error) {
	var id int64
	err := d.QueryRowContext(ctx, `SELECT id FROM media_files WHERE path = ?`, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// LocalizacoesDosTitulos resume, por título, se existe alguma cópia no Mac.
// Um título com um episódio local e outro só na nuvem continua tocável.
func (d *DB) TitulosSoNaNuvem(ctx context.Context) (map[int64]bool, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT title_id FROM media_files
		 WHERE title_id IS NOT NULL
		 GROUP BY title_id
		HAVING SUM(localizacao NOT IN ('nuvem', 'baixando')) = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// LocalizacoesDoTitulo devolve a localização de cada arquivo de um título.
func (d *DB) LocalizacoesDoTitulo(ctx context.Context, titleID int64) (map[int64]string, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, localizacao FROM media_files WHERE title_id = ?`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var (
			id  int64
			loc string
		)
		if err := rows.Scan(&id, &loc); err != nil {
			return nil, err
		}
		out[id] = loc
	}
	return out, rows.Err()
}

// Upload é uma linha da tabela uploads.
type Upload struct {
	ID           int64  `json:"id"`
	LibraryID    int64  `json:"library_id"`
	UserID       *int64 `json:"-"`
	Key          string `json:"key"`
	UploadID     string `json:"-"`
	Nome         string `json:"nome"`
	Tamanho      int64  `json:"tamanho"`
	ParteTamanho int64  `json:"parte_tamanho"`
	ContentType  string `json:"content_type"`
	Origem       string `json:"-"`
	MediaFileID  *int64 `json:"media_file_id,omitempty"`
	Estado       string `json:"estado"`
	CreatedAt    int64  `json:"created_at"`
}

func (d *DB) CriaUpload(ctx context.Context, u Upload) (int64, error) {
	agora := time.Now().Unix()
	res, err := d.ExecContext(ctx, `
		INSERT INTO uploads (library_id, user_id, nuvem_key, upload_id, nome, tamanho,
		                     parte_tamanho, content_type, origem, media_file_id,
		                     estado, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'enviando', ?, ?)`,
		u.LibraryID, u.UserID, u.Key, u.UploadID, u.Nome, u.Tamanho, u.ParteTamanho,
		u.ContentType, u.Origem, u.MediaFileID, agora, agora)
	if err != nil {
		return 0, fmt.Errorf("registrando upload: %w", err)
	}
	return res.LastInsertId()
}

const colunasUpload = `id, library_id, user_id, nuvem_key, upload_id, nome, tamanho,
	parte_tamanho, content_type, origem, media_file_id, estado, created_at`

func scanUpload(row interface{ Scan(...any) error }) (Upload, error) {
	var (
		u      Upload
		userID sql.NullInt64
		fileID sql.NullInt64
	)
	err := row.Scan(&u.ID, &u.LibraryID, &userID, &u.Key, &u.UploadID, &u.Nome, &u.Tamanho,
		&u.ParteTamanho, &u.ContentType, &u.Origem, &fileID, &u.Estado, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Upload{}, ErrNotFound
	}
	if userID.Valid {
		u.UserID = &userID.Int64
	}
	if fileID.Valid {
		u.MediaFileID = &fileID.Int64
	}
	return u, err
}

func (d *DB) UploadPorID(ctx context.Context, id int64) (Upload, error) {
	return scanUpload(d.QueryRowContext(ctx, `SELECT `+colunasUpload+` FROM uploads WHERE id = ?`, id))
}

// UploadPendenteDe acha um push interrompido do mesmo arquivo de origem.
func (d *DB) UploadPendenteDe(ctx context.Context, origem string) (Upload, error) {
	return scanUpload(d.QueryRowContext(ctx, `SELECT `+colunasUpload+`
		FROM uploads WHERE origem = ? AND estado = 'enviando' ORDER BY id DESC LIMIT 1`, origem))
}

// UploadAbertoDoNavegador acha um envio do navegador ainda aberto para o
// mesmo arquivo (biblioteca, nome e tamanho): soltar o arquivo de novo
// continua esse envio em vez de abrir outro e deixar o primeiro órfão.
func (d *DB) UploadAbertoDoNavegador(ctx context.Context, libID int64, nome string, tamanho int64) (Upload, error) {
	return scanUpload(d.QueryRowContext(ctx, `SELECT `+colunasUpload+`
		FROM uploads WHERE origem = '' AND estado = 'enviando'
		  AND library_id = ? AND nome = ? AND tamanho = ?
		ORDER BY id DESC LIMIT 1`, libID, nome, tamanho))
}

// UploadsPendentes lista os uploads que ainda não terminaram.
func (d *DB) UploadsPendentes(ctx context.Context) ([]Upload, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+colunasUpload+`
		FROM uploads WHERE estado = 'enviando' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Upload
	for rows.Next() {
		u, err := scanUpload(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (d *DB) EstadoDoUpload(ctx context.Context, id int64, estado string) error {
	_, err := d.ExecContext(ctx, `UPDATE uploads SET estado = ?, updated_at = ? WHERE id = ?`,
		estado, time.Now().Unix(), id)
	return err
}

// VersaoDosEventos é o schema_version do contrato entre Mac e nuvem.
const VersaoDosEventos = 1

// Evento é uma linha do outbox.
type Evento struct {
	ID      int64           `json:"id"`
	Tipo    string          `json:"tipo"`
	Payload json.RawMessage `json:"payload"`
	Criado  int64           `json:"criado"`
}

// RegistraEvento grava no outbox. Falhar aqui nunca derruba a ação que gerou
// o evento: o outbox é para a nuvem saber, não para o Mac funcionar.
func (d *DB) RegistraEvento(ctx context.Context, tipo string, payload any) {
	if semEventos(ctx) {
		return
	}
	corpo, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO outbox (tipo, payload, criado) VALUES (?, ?, ?)`,
		tipo, string(corpo), time.Now().UnixMilli()); err != nil {
		log.Printf("outbox: %v", err)
	}
}

// EventosPendentes devolve até limite eventos, do mais antigo.
func (d *DB) EventosPendentes(ctx context.Context, limite int) ([]Evento, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, tipo, payload, criado FROM outbox ORDER BY id LIMIT ?`, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Evento
	for rows.Next() {
		var (
			e       Evento
			payload string
		)
		if err := rows.Scan(&e.ID, &e.Tipo, &payload, &e.Criado); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ContaEventos diz quantos eventos esperam o híbrido.
func (d *DB) ContaEventos(ctx context.Context) (int64, error) {
	var n int64
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox`).Scan(&n)
	return n, err
}

// ConfirmaEventos apaga do outbox o que já está no journal do bucket.
func (d *DB) ConfirmaEventos(ctx context.Context, ateID int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM outbox WHERE id <= ?`, ateID)
	return err
}

func (d *DB) EstadoNuvem(ctx context.Context, chave string) string {
	var v string
	_ = d.QueryRowContext(ctx, `SELECT valor FROM nuvem_estado WHERE chave = ?`, chave).Scan(&v)
	return v
}

func (d *DB) GravaEstadoNuvem(ctx context.Context, chave, valor string) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO nuvem_estado (chave, valor) VALUES (?, ?)
		ON CONFLICT (chave) DO UPDATE SET valor = excluded.valor`, chave, valor)
	return err
}

// SetEspelhada liga ou desliga o espelhamento de uma biblioteca.
func (d *DB) SetEspelhada(ctx context.Context, libID int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := d.ExecContext(ctx, `UPDATE libraries SET espelhada = ? WHERE id = ?`, v, libID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ChavesNaNuvem devolve todas as chaves que o catálogo e os uploads abertos
// já conhecem, para a reconciliação só importar o que é novo.
func (d *DB) ChavesNaNuvem(ctx context.Context) (map[string]bool, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT nuvem_key FROM media_files WHERE nuvem_key != ''
		UNION SELECT nuvem_key FROM uploads WHERE estado = 'enviando'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

// ArquivoParaSincronizar é o mínimo que fixar, liberar e espelhar precisam.
type ArquivoParaSincronizar struct {
	ID          int64
	LibraryID   int64
	Path        string
	RelPath     string
	Size        int64
	Localizacao string
	NuvemKey    string
}

// ArquivosPorLocalizacao lista os arquivos num estado, opcionalmente só das
// bibliotecas espelhadas.
func (d *DB) ArquivosPorLocalizacao(ctx context.Context, localizacao string, soEspelhadas bool) ([]ArquivoParaSincronizar, error) {
	q := `SELECT f.id, f.library_id, f.path, f.rel_path, f.size, f.localizacao, f.nuvem_key
	        FROM media_files f JOIN libraries l ON l.id = f.library_id
	       WHERE f.localizacao = ?`
	if soEspelhadas {
		q += ` AND l.espelhada = 1 AND l.enabled = 1`
	}
	rows, err := d.QueryContext(ctx, q+` ORDER BY f.id`, localizacao)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArquivoParaSincronizar
	for rows.Next() {
		var a ArquivoParaSincronizar
		if err := rows.Scan(&a.ID, &a.LibraryID, &a.Path, &a.RelPath, &a.Size, &a.Localizacao, &a.NuvemKey); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MudaCaminho troca o caminho e a localização juntos: é o que acontece quando
// um item é fixado (ganha arquivo no disco) ou tem o espaço liberado (perde).
func (d *DB) MudaCaminho(ctx context.Context, fileID int64, path, localizacao, hash string, mtime int64) error {
	_, err := d.ExecContext(ctx, `
		UPDATE media_files SET path = ?, localizacao = ?, content_hash = ?, mtime = ?
		 WHERE id = ?`, path, localizacao, hash, mtime, fileID)
	return err
}

// ArquivoComChave é um item do catálogo que tem (ou deveria ter) cópia no bucket.
type ArquivoComChave struct {
	ID          int64
	LibraryID   int64
	Key         string
	Localizacao string
}

func (d *DB) ArquivosComChave(ctx context.Context) ([]ArquivoComChave, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, library_id, nuvem_key, localizacao FROM media_files WHERE nuvem_key != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArquivoComChave
	for rows.Next() {
		var a ArquivoComChave
		if err := rows.Scan(&a.ID, &a.LibraryID, &a.Key, &a.Localizacao); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ApagaSoDaNuvem tira do catálogo um item cuja única cópia era o bucket e
// que sumiu de lá. Nunca toca num item com cópia no disco.
func (d *DB) ApagaSoDaNuvem(ctx context.Context, id int64) (bool, error) {
	res, err := d.ExecContext(ctx, `DELETE FROM media_files WHERE id = ? AND localizacao = 'nuvem'`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
