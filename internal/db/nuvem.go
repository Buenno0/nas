package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
