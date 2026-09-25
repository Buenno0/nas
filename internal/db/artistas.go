package db

import (
	"context"
	"fmt"
)

// A coluna titles.artist existe desde o esquema inicial e nunca teve tela: dava
// para ver um álbum, nunca para ver um artista. Uma biblioteca de música em que
// não se navega por quem tocou é uma pasta com capas.

// Artista agrupa os álbuns de um mesmo nome.
type Artista struct {
	Nome    string  `json:"nome"`
	Albuns  int     `json:"albuns"`
	Faixas  int     `json:"faixas"`
	Duracao float64 `json:"duracao"`
	// Poster de um dos álbuns, para o card ter rosto.
	Poster string `json:"poster,omitempty"`
}

// Artistas lista quem tem música no acervo, em ordem alfabética.
func (d *DB) Artistas(ctx context.Context) ([]Artista, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT t.artist,
		       COUNT(*),
		       IFNULL(SUM((SELECT COUNT(*) FROM media_files f WHERE f.title_id = t.id)), 0),
		       IFNULL(SUM((SELECT IFNULL(SUM(f.duration), 0) FROM media_files f WHERE f.title_id = t.id)), 0),
		       IFNULL(MAX(NULLIF(t.poster, '')), '')
		  FROM titles t
		 WHERE t.kind = 'album' AND t.artist != ''
		 GROUP BY t.artist COLLATE NOCASE
		 ORDER BY t.artist COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("listando artistas: %w", err)
	}
	defer rows.Close()

	artistas := []Artista{}
	for rows.Next() {
		var a Artista
		if err := rows.Scan(&a.Nome, &a.Albuns, &a.Faixas, &a.Duracao, &a.Poster); err != nil {
			return nil, err
		}
		artistas = append(artistas, a)
	}
	return artistas, rows.Err()
}

// AlbunsDoArtista traz a discografia, do mais novo para o mais velho — que é
// como se olha a obra de alguém, e não em ordem alfabética de título.
func (d *DB) AlbunsDoArtista(ctx context.Context, nome string) ([]TitleCard, error) {
	rows, err := d.QueryContext(ctx, titleCardSelect+`
	 WHERE t.kind = 'album' AND t.artist = ? COLLATE NOCASE
	 ORDER BY IFNULL(t.year, 0) DESC, t.sort_name`, nome)
	if err != nil {
		return nil, fmt.Errorf("álbuns de %q: %w", nome, err)
	}
	defer rows.Close()
	return coletarCards(rows)
}

// FaixasDoArtista traz tudo que o artista tem no acervo, em ordem de álbum e
// faixa. É o que alimenta o "tocar tudo" e o modo aleatório.
func (d *DB) FaixasDoArtista(ctx context.Context, nome string, userID int64) ([]FileInfo, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT f.id, f.rel_path, f.ext, f.media_type, f.size, IFNULL(f.duration, 0),
		       IFNULL(f.width, 0), IFNULL(f.height, 0), f.vcodec, f.acodec,
		       IFNULL(f.track, 0), f.thumb, f.display_name,
		       IFNULL(p.position_sec, 0), IFNULL(p.finished, 0),
		       0, 0, '', 0
		  FROM media_files f
		  JOIN titles t ON t.id = f.title_id
		  LEFT JOIN progress p ON p.media_file_id = f.id AND p.user_id = ?
		 WHERE t.kind = 'album' AND t.artist = ? COLLATE NOCASE
		   AND f.media_type = 'audio'
		 ORDER BY IFNULL(t.year, 0) DESC, t.sort_name, IFNULL(f.track, 0), f.rel_path`,
		userID, nome)
	if err != nil {
		return nil, fmt.Errorf("faixas de %q: %w", nome, err)
	}
	defer rows.Close()

	faixas := []FileInfo{}
	for rows.Next() {
		var (
			fi       FileInfo
			mtype    string
			finished int
		)
		if err := rows.Scan(&fi.ID, &fi.RelPath, &fi.Ext, &mtype, &fi.Size, &fi.Duration,
			&fi.Width, &fi.Height, &fi.VCodec, &fi.ACodec, &fi.Track, &fi.Thumb, &fi.TagName,
			&fi.Position, &finished, &fi.Season, &fi.Episode, &fi.EpName, &fi.Quando); err != nil {
			return nil, err
		}
		fi.Type = MediaType(mtype)
		fi.Finished = finished != 0
		fi.Name = displayName(fi)
		faixas = append(faixas, fi)
	}
	return faixas, rows.Err()
}
