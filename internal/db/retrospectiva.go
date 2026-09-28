package db

import (
	"context"
	"sort"
	"strings"
	"time"
)

// TituloNaRetrospectiva é um título com o quanto foi assistido.
type TituloNaRetrospectiva struct {
	ID       int64   `json:"id"`
	Nome     string  `json:"nome"`
	Kind     string  `json:"kind"`
	Poster   string  `json:"poster,omitempty"`
	Backdrop string  `json:"backdrop,omitempty"`
	Segundos float64 `json:"segundos"`
	Dia      string  `json:"dia,omitempty"`
}

// Retrospectiva resume um ano de uma pessoa.
type Retrospectiva struct {
	Ano            int                     `json:"ano"`
	Segundos       float64                 `json:"segundos"`
	Dias           int                     `json:"dias"`
	MaiorSequencia int                     `json:"maior_sequencia"`
	Titulos        int                     `json:"titulos"`
	Filmes         int                     `json:"filmes"`
	Episodios      int                     `json:"episodios"`
	PorMes         [12]float64             `json:"por_mes"`
	Horas          [24]float64             `json:"horas"`
	HoraPreferida  int                     `json:"hora_preferida"`
	Generos        []GeneroNaRetrospectiva `json:"generos"`
	MaisVistos     []TituloNaRetrospectiva `json:"mais_vistos"`
	Maratona       *Maratona               `json:"maratona,omitempty"`
	Primeiro       *TituloNaRetrospectiva  `json:"primeiro,omitempty"`
	Ultimo         *TituloNaRetrospectiva  `json:"ultimo,omitempty"`
	Juntos         []Companhia             `json:"juntos"`
}

type GeneroNaRetrospectiva struct {
	Nome     string  `json:"nome"`
	Segundos float64 `json:"segundos"`
}

// Maratona é o dia com mais episódios diferentes de uma mesma série.
type Maratona struct {
	Dia       string                `json:"dia"`
	Episodios int                   `json:"episodios"`
	Titulo    TituloNaRetrospectiva `json:"titulo"`
}

// AnoDaRetrospectiva monta o ano de userID a partir do histórico. São umas
// poucas consultas pequenas: nada que pese no Mac.
func (d *DB) AnoDaRetrospectiva(ctx context.Context, userID int64, ano int) (Retrospectiva, error) {
	r := Retrospectiva{Ano: ano, Generos: []GeneroNaRetrospectiva{}, MaisVistos: []TituloNaRetrospectiva{}}
	prefixo := time.Date(ano, 1, 1, 0, 0, 0, 0, time.Local).Format("2006") + "-%"

	rows, err := d.QueryContext(ctx, `
		SELECT h.dia, h.segundos, f.media_type, IFNULL(t.id, 0), IFNULL(t.name, f.display_name), IFNULL(t.kind, ''),
		       IFNULL(t.poster, ''), IFNULL(t.backdrop, ''), IFNULL(t.genres, ''), f.id
		  FROM historico h
		  JOIN media_files f ON f.id = h.media_file_id
		  LEFT JOIN titles t ON t.id = f.title_id
		 WHERE h.user_id = ? AND h.dia LIKE ?
		 ORDER BY h.dia`, userID, prefixo)
	if err != nil {
		return r, err
	}
	defer rows.Close()

	type chave struct {
		dia    string
		titulo int64
	}
	dias := map[string]bool{}
	porTitulo := map[int64]*TituloNaRetrospectiva{}
	generos := map[string]float64{}
	arquivos := map[int64]string{} // arquivo → kind do título
	episodiosNoDia := map[chave]map[int64]bool{}
	for rows.Next() {
		var (
			dia, tipo, nome, kind, poster, backdrop, gens string
			seg                                           float64
			tid, fid                                      int64
		)
		if err := rows.Scan(&dia, &seg, &tipo, &tid, &nome, &kind, &poster, &backdrop, &gens, &fid); err != nil {
			return r, err
		}
		if tipo != string(TypeVideo) && tipo != string(TypeAudio) {
			continue
		}
		r.Segundos += seg
		dias[dia] = true
		if m, err := time.Parse("2006-01-02", dia); err == nil {
			r.PorMes[m.Month()-1] += seg
		}
		t := porTitulo[tid]
		if t == nil {
			t = &TituloNaRetrospectiva{ID: tid, Nome: nome, Kind: kind, Poster: poster, Backdrop: backdrop}
			porTitulo[tid] = t
		}
		t.Segundos += seg
		if r.Primeiro == nil {
			p := *t
			p.Dia = dia
			r.Primeiro = &p
		}
		u := *t
		u.Dia = dia
		r.Ultimo = &u
		for _, g := range strings.Split(gens, ",") {
			if g = strings.TrimSpace(g); g != "" {
				generos[g] += seg
			}
		}
		arquivos[fid] = kind
		if kind == "tv" {
			k := chave{dia, tid}
			if episodiosNoDia[k] == nil {
				episodiosNoDia[k] = map[int64]bool{}
			}
			episodiosNoDia[k][fid] = true
		}
	}
	if err := rows.Err(); err != nil {
		return r, err
	}

	r.Dias = len(dias)
	r.Titulos = len(porTitulo)
	for _, kind := range arquivos {
		if kind == "tv" {
			r.Episodios++
		} else if kind == "movie" {
			r.Filmes++
		}
	}
	r.MaiorSequencia = maiorSequencia(dias)

	for nome, seg := range generos {
		r.Generos = append(r.Generos, GeneroNaRetrospectiva{Nome: nome, Segundos: seg})
	}
	sort.Slice(r.Generos, func(i, j int) bool { return r.Generos[i].Segundos > r.Generos[j].Segundos })
	if len(r.Generos) > 5 {
		r.Generos = r.Generos[:5]
	}
	for _, t := range porTitulo {
		r.MaisVistos = append(r.MaisVistos, *t)
	}
	sort.Slice(r.MaisVistos, func(i, j int) bool { return r.MaisVistos[i].Segundos > r.MaisVistos[j].Segundos })
	if len(r.MaisVistos) > 5 {
		r.MaisVistos = r.MaisVistos[:5]
	}
	for k, eps := range episodiosNoDia {
		if len(eps) >= 2 && (r.Maratona == nil || len(eps) > r.Maratona.Episodios) {
			r.Maratona = &Maratona{Dia: k.dia, Episodios: len(eps), Titulo: *porTitulo[k.titulo]}
		}
	}

	hrows, err := d.QueryContext(ctx, `SELECT hora, segundos FROM historico_horas WHERE user_id = ? AND ano = ?`, userID, ano)
	if err != nil {
		return r, err
	}
	defer hrows.Close()
	for hrows.Next() {
		var h int
		var seg float64
		if err := hrows.Scan(&h, &seg); err == nil && h >= 0 && h < 24 {
			r.Horas[h] = seg
		}
	}
	melhor := 0.0
	r.HoraPreferida = -1
	for h, seg := range r.Horas {
		if seg > melhor {
			melhor, r.HoraPreferida = seg, h
		}
	}
	return r, hrows.Err()
}

func maiorSequencia(dias map[string]bool) int {
	lista := make([]time.Time, 0, len(dias))
	for d := range dias {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			lista = append(lista, t)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Before(lista[j]) })
	maior, atual := 0, 0
	for i, t := range lista {
		if i > 0 && t.Sub(lista[i-1]) == 24*time.Hour {
			atual++
		} else {
			atual = 1
		}
		maior = max(maior, atual)
	}
	return maior
}
