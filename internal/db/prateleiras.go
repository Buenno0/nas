package db

import (
	"context"
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
