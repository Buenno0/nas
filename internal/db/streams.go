package db

import (
	"context"
	"fmt"
)

// StreamKind separa faixa de áudio de legenda.
type StreamKind string

const (
	StreamAudio    StreamKind = "audio"
	StreamSubtitle StreamKind = "subtitle"
)

// Stream é uma faixa de áudio ou uma legenda de um arquivo.
type Stream struct {
	Index    int        `json:"idx"`
	Kind     StreamKind `json:"kind"`
	Codec    string     `json:"codec,omitempty"`
	Lang     string     `json:"lang,omitempty"`
	Title    string     `json:"title,omitempty"`
	Channels int        `json:"channels,omitempty"`
	Default  bool       `json:"default,omitempty"`
	Forced   bool       `json:"forced,omitempty"`
	// ExtPath é o caminho da legenda em arquivo separado; vazio para as
	// embutidas. Nunca vai para o cliente: caminho de disco é do dono da
	// máquina, como o das bibliotecas.
	ExtPath string `json:"-"`
}

// ReplaceStreams troca as faixas de um arquivo pelas recém-lidas.
//
// Troca em vez de acrescentar porque o arquivo pode ter sido substituído no
// disco pelo mesmo nome — uma versão dublada no lugar da original, por exemplo.
// Manter as faixas antigas ofereceria ao player um índice que não existe mais.
func (d *DB) ReplaceStreams(ctx context.Context, fileID int64, streams []Stream) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM media_streams WHERE media_file_id = ?`, fileID); err != nil {
		return fmt.Errorf("limpando faixas: %w", err)
	}
	for _, st := range streams {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_streams
				(media_file_id, idx, kind, codec, lang, title, channels, is_default, forced, ext_path)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fileID, st.Index, string(st.Kind), st.Codec, st.Lang, st.Title,
			st.Channels, st.Default, st.Forced, st.ExtPath); err != nil {
			return fmt.Errorf("gravando faixa %d: %w", st.Index, err)
		}
	}
	return tx.Commit()
}

// Streams devolve as faixas de um arquivo, áudio antes de legenda e em ordem
// de índice — a mesma ordem em que aparecem no arquivo.
func (d *DB) Streams(ctx context.Context, fileID int64) ([]Stream, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT idx, kind, codec, lang, title, channels, is_default, forced, ext_path
		  FROM media_streams
		 WHERE media_file_id = ?
		 ORDER BY kind DESC, idx`, fileID)
	if err != nil {
		return nil, fmt.Errorf("lendo faixas: %w", err)
	}
	defer rows.Close()

	streams := []Stream{}
	for rows.Next() {
		var st Stream
		var kind string
		if err := rows.Scan(&st.Index, &kind, &st.Codec, &st.Lang, &st.Title,
			&st.Channels, &st.Default, &st.Forced, &st.ExtPath); err != nil {
			return nil, err
		}
		st.Kind = StreamKind(kind)
		streams = append(streams, st)
	}
	return streams, rows.Err()
}

// StreamByIndex busca uma faixa específica. É o que valida o índice vindo da
// URL: sem isso, um `?audio=` qualquer viraria argumento de -map do ffmpeg.
func (d *DB) StreamByIndex(ctx context.Context, fileID int64, idx int) (Stream, error) {
	var st Stream
	var kind string
	err := d.QueryRowContext(ctx, `
		SELECT idx, kind, codec, lang, title, channels, is_default, forced, ext_path
		  FROM media_streams
		 WHERE media_file_id = ? AND idx = ?`, fileID, idx).
		Scan(&st.Index, &kind, &st.Codec, &st.Lang, &st.Title,
			&st.Channels, &st.Default, &st.Forced, &st.ExtPath)
	if err != nil {
		return Stream{}, ErrNotFound
	}
	st.Kind = StreamKind(kind)
	return st, nil
}

// ReplaceExternalSubtitles troca só as legendas em arquivo separado (índice
// negativo), sem tocar no que veio de dentro do container.
//
// Existe porque largar um "filme.pt.srt" ao lado de um filme já indexado não
// muda o vídeo em nada: tamanho e data continuam iguais, o scan o considera
// inalterado e nunca o reexaminaria. Exigir `nas scan --force` para uma legenda
// nova seria transformar o caso mais comum do acervo em manutenção manual.
func (d *DB) ReplaceExternalSubtitles(ctx context.Context, fileID int64, subs []Stream) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM media_streams WHERE media_file_id = ? AND idx < 0`, fileID); err != nil {
		return err
	}
	for _, st := range subs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_streams
				(media_file_id, idx, kind, codec, lang, title, channels, is_default, forced, ext_path)
			VALUES (?, ?, 'subtitle', ?, ?, ?, 0, 0, ?, ?)`,
			fileID, st.Index, st.Codec, st.Lang, st.Title, st.Forced, st.ExtPath); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ContaLegendasExternas diz quantas legendas em arquivo já estão registradas,
// para o scan só escrever quando o conjunto realmente mudou.
func (d *DB) ContaLegendasExternas(ctx context.Context, fileID int64) (int, error) {
	var n int
	err := d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_streams WHERE media_file_id = ? AND idx < 0`, fileID).Scan(&n)
	return n, err
}
