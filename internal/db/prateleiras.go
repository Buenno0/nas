package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Prateleiras da home que não são "recentes" nem "favoritos".
//
// Antes disto a home era fixa no código: adicionados recentemente, favoritos e
// uma fileira por biblioteca. Um acervo parado ficava com a mesma tela para
// sempre, e o que estava no fundo do acervo nunca reaparecia.

// diasParaEsquecer separa "continuar assistindo" de "esquecido". Um mês é o
// ponto em que a pessoa deixou de lembrar do enredo: continuar a oferecer como
// se fosse a sessão de ontem é o que faz a lista de continuar virar um cemitério.
const diasParaEsquecer = 30

// Esquecidos são as coisas começadas e abandonadas — o complemento exato de
// ContinueWatching, que agora só olha os últimos 30 dias. A união das duas
// continua sendo tudo que foi começado e não terminado: nada some da interface.
func (d *DB) Esquecidos(ctx context.Context, userID int64, limit int) ([]ContinueItem, error) {
	corte := time.Now().AddDate(0, 0, -diasParaEsquecer).Unix()
	return d.continuarQuery(ctx, `p.updated_at < ?`, `p.updated_at DESC`, userID, corte, limit)
}

// TitulosAoAcaso traz o que a pessoa nunca abriu. É o antídoto do acervo grande:
// sem isso, o que entrou há dois anos e nunca foi clicado nunca mais aparece.
//
// Sem seed: a ordem muda a cada carregamento de propósito, porque a graça é a
// prateleira ser diferente quando você volta.
func (d *DB) TitulosAoAcaso(ctx context.Context, userID int64, limit int) ([]TitleCard, error) {
	rows, err := d.QueryContext(ctx, titleCardSelect+`
	 WHERE t.kind IN ('movie', 'tv')
	   AND NOT EXISTS (
	         SELECT 1 FROM progress p
	           JOIN media_files f ON f.id = p.media_file_id
	          WHERE f.title_id = t.id AND p.user_id = ?)
	 ORDER BY RANDOM()
	 LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("sorteando títulos: %w", err)
	}
	defer rows.Close()
	return coletarCards(rows)
}

// FotoDoDia é uma foto da prateleira "neste dia", já com o álbum a que
// pertence: sem isso o card não teria para onde levar.
type FotoDoDia struct {
	FileInfo
	TitleID int64 `json:"title_id"`
}

// FotosDoDia são as fotos tiradas neste mesmo dia do calendário, em anos
// anteriores. Depende da data de captura (migration 0008); o mtime serve de
// reserva, como no resto da galeria.
func (d *DB) FotosDoDia(ctx context.Context, limit int) ([]FotoDoDia, error) {
	hoje := time.Now()
	// strftime do SQLite trabalha em UTC por padrão; 'localtime' faz a
	// comparação no fuso de quem tirou a foto, que é o dia que a pessoa lembra.
	rows, err := d.QueryContext(ctx, `
		SELECT f.id, f.rel_path, f.ext, f.media_type, f.size,
		       IFNULL(f.width, 0), IFNULL(f.height, 0), f.thumb,
		       IFNULL(NULLIF(f.taken_at, 0), f.mtime) AS quando,
		       IFNULL(f.title_id, 0)
		  FROM media_files f
		 WHERE f.media_type = 'photo'
		   AND f.title_id IS NOT NULL
		   AND strftime('%m-%d', quando, 'unixepoch', 'localtime') = ?
		   AND strftime('%Y', quando, 'unixepoch', 'localtime') < ?
		 ORDER BY quando DESC
		 LIMIT ?`,
		hoje.Format("01-02"), hoje.Format("2006"), limit)
	if err != nil {
		return nil, fmt.Errorf("fotos do dia: %w", err)
	}
	defer rows.Close()

	fotos := []FotoDoDia{}
	for rows.Next() {
		var f FotoDoDia
		var mtype string
		if err := rows.Scan(&f.ID, &f.RelPath, &f.Ext, &mtype, &f.Size,
			&f.Width, &f.Height, &f.Thumb, &f.Quando, &f.TitleID); err != nil {
			return nil, err
		}
		f.Type = MediaType(mtype)
		f.Name = displayName(f.FileInfo)
		fotos = append(fotos, f)
	}
	return fotos, rows.Err()
}

// CabeEm são filmes nunca começados que cabem no tempo dado ("Cabe em
// 1h30"). Abaixo de 40 min é curta ou extra, não filme para a noite.
func (d *DB) CabeEm(ctx context.Context, userID int64, maxSegundos float64, limit int) ([]TitleCard, error) {
	rows, err := d.QueryContext(ctx, titleCardSelect+`
	 WHERE t.kind = 'movie'
	   AND (SELECT MAX(f.duration) FROM media_files f WHERE f.title_id = t.id) BETWEEN 2400 AND ?
	   AND NOT EXISTS (
	         SELECT 1 FROM progress p JOIN media_files f ON f.id = p.media_file_id
	          WHERE f.title_id = t.id AND p.user_id = ?)
	 ORDER BY RANDOM()
	 LIMIT ?`, maxSegundos, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("filmes curtos: %w", err)
	}
	defer rows.Close()
	return coletarCards(rows)
}

// ParaTerminar são séries com algum episódio terminado e outros por ver,
// da mais recente para a mais antiga. "Continuar assistindo" é por arquivo;
// isto é a série inteira que ficou pela metade.
func (d *DB) ParaTerminar(ctx context.Context, userID int64, limit int) ([]TitleCard, error) {
	rows, err := d.QueryContext(ctx, titleCardSelect+`
	 WHERE t.kind = 'tv'
	   AND EXISTS (SELECT 1 FROM progress p JOIN media_files f ON f.id = p.media_file_id
	                WHERE f.title_id = t.id AND p.user_id = ? AND p.finished = 1)
	   AND EXISTS (SELECT 1 FROM media_files f
	                WHERE f.title_id = t.id AND NOT EXISTS (
	                      SELECT 1 FROM progress p WHERE p.media_file_id = f.id AND p.user_id = ? AND p.finished = 1))
	 ORDER BY (SELECT MAX(p.updated_at) FROM progress p JOIN media_files f ON f.id = p.media_file_id
	            WHERE f.title_id = t.id AND p.user_id = ?) DESC
	 LIMIT ?`, userID, userID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("séries pela metade: %w", err)
	}
	defer rows.Close()
	return coletarCards(rows)
}

// UltimoTerminado é o último filme ou série que a pessoa terminou e que tem
// id do TMDB: a semente do "Porque você viu".
func (d *DB) UltimoTerminado(ctx context.Context, userID int64) (id int64, nome, kind string, tmdbID int, err error) {
	err = d.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.kind, t.tmdb_id
		  FROM progress p JOIN media_files f ON f.id = p.media_file_id JOIN titles t ON t.id = f.title_id
		 WHERE p.user_id = ? AND p.finished = 1 AND t.tmdb_id > 0 AND t.kind IN ('movie', 'tv')
		 ORDER BY p.updated_at DESC LIMIT 1`, userID).Scan(&id, &nome, &kind, &tmdbID)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

// Recomendacoes devolve os ids do TMDB guardados para o título e há quanto
// tempo. ok=false quando nunca foram buscados.
func (d *DB) Recomendacoes(ctx context.Context, titleID int64) (ids []int, em time.Time, ok bool) {
	var bruto string
	var quando int64
	if err := d.QueryRowContext(ctx, `SELECT tmdb_ids, em FROM recomendacoes WHERE title_id = ?`, titleID).Scan(&bruto, &quando); err != nil {
		return nil, time.Time{}, false
	}
	_ = json.Unmarshal([]byte(bruto), &ids)
	return ids, time.Unix(quando, 0), true
}

func (d *DB) GravaRecomendacoes(ctx context.Context, titleID int64, ids []int) error {
	corpo, _ := json.Marshal(ids)
	_, err := d.ExecContext(ctx, `
		INSERT INTO recomendacoes (title_id, tmdb_ids, em) VALUES (?, ?, ?)
		ON CONFLICT DO UPDATE SET tmdb_ids = excluded.tmdb_ids, em = excluded.em`,
		titleID, string(corpo), time.Now().Unix())
	return err
}

// TitulosPorTMDB são os títulos do acervo com esses ids do TMDB, na ordem da
// recomendação, tirando os que a pessoa já terminou.
func (d *DB) TitulosPorTMDB(ctx context.Context, userID int64, ids []int, limit int) ([]TitleCard, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	lista, _ := json.Marshal(ids)
	rows, err := d.QueryContext(ctx, titleCardSelect+`
	  JOIN json_each(?) j ON j.value = t.tmdb_id
	 WHERE NOT EXISTS (SELECT 1 FROM progress p JOIN media_files f ON f.id = p.media_file_id
	                    WHERE f.title_id = t.id AND p.user_id = ? AND p.finished = 1)
	 ORDER BY j.key
	 LIMIT ?`, string(lista), userID, limit)
	if err != nil {
		return nil, fmt.Errorf("recomendações no acervo: %w", err)
	}
	defer rows.Close()
	return coletarCards(rows)
}
