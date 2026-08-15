package meta

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/media"
	"nas/internal/meta/tmdb"
)

// MinScore é a confiança mínima para aceitar um match do TMDB. Abaixo disso o
// título fica marcado como "unmatched": é melhor mostrar uma capa gerada do
// que jurar que o vídeo da formatura é um filme de 1998.
const MinScore = 0.72

// Enricher completa os títulos com metadados e capas.
type Enricher struct {
	db        *db.DB
	client    *tmdb.Client
	posterDir string
	thumbDir  string
	MinScore  float64
}

type Stats struct {
	Matched   int
	Unmatched int
	Local     int // capa gerada localmente
	Failed    int
}

func (s Stats) String() string {
	return fmt.Sprintf("%d casados, %d sem match, %d capas locais, %d falhas",
		s.Matched, s.Unmatched, s.Local, s.Failed)
}

type Progress struct {
	Title   string `json:"title"`
	Current int    `json:"current"`
	Total   int    `json:"total"`
}

type ProgressFunc func(Progress)

func NewEnricher(database *db.DB, cfg config.Config) (*Enricher, error) {
	posterDir, err := config.PosterDir()
	if err != nil {
		return nil, err
	}
	thumbDir, err := config.ThumbDir()
	if err != nil {
		return nil, err
	}
	return &Enricher{
		db:        database,
		client:    tmdb.New(cfg.TMDBKey, cfg.TMDBLang),
		posterDir: posterDir,
		thumbDir:  thumbDir,
		MinScore:  MinScore,
	}, nil
}

// TMDBEnabled diz se há chave configurada.
func (e *Enricher) TMDBEnabled() bool { return e.client.Enabled() }

// Run enriquece os títulos pendentes (ou todos, com all=true).
func (e *Enricher) Run(ctx context.Context, all bool, onProgress ProgressFunc) (Stats, error) {
	var stats Stats

	titles, err := e.db.PendingTitles(ctx, all, 0)
	if err != nil {
		return stats, err
	}

	for i, title := range titles {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if onProgress != nil {
			onProgress(Progress{Title: title.Name, Current: i + 1, Total: len(titles)})
		}

		matched, err := e.enrichTitle(ctx, title)
		switch {
		case err != nil:
			// Um título problemático não pode parar a fila inteira.
			log.Printf("metadados de %q: %v", title.Name, err)
			stats.Failed++
		case matched:
			stats.Matched++
		default:
			stats.Unmatched++
		}

		// A capa local entra tanto para quem não casou quanto para álbuns e
		// pastas de fotos, que nunca vão ao TMDB.
		if !matched {
			if ok := e.localArtwork(ctx, title); ok {
				stats.Local++
			}
			// Sem estado final o título voltaria à fila em todo scan. "local"
			// distingue quem nunca vai ao TMDB de quem foi e não casou.
			state := "unmatched"
			if !e.tmdbApplies(title) {
				state = "local"
			}
			if err := e.db.SetTitleState(ctx, title.ID, state); err != nil {
				log.Printf("estado de %q: %v", title.Name, err)
			}
		}
	}
	return stats, nil
}

// enrichTitle tenta o TMDB. Devolve matched=false quando não há chave, quando
// o tipo não se aplica (álbum, fotos) ou quando nenhum candidato convence.
func (e *Enricher) enrichTitle(ctx context.Context, title db.Title) (bool, error) {
	if !e.tmdbApplies(title) {
		return false, nil
	}

	candidates, err := e.search(ctx, title)
	if err != nil {
		return false, err
	}

	best, score := bestMatch(title.Name, title.Year, candidates)
	if score < e.MinScore {
		return false, nil
	}

	if err := e.apply(ctx, title, best, "matched"); err != nil {
		return false, err
	}
	return true, nil
}

// tmdbApplies: só filme e série vão ao TMDB, e só com chave configurada.
func (e *Enricher) tmdbApplies(title db.Title) bool {
	if !e.client.Enabled() {
		return false
	}
	return title.Kind == db.TitleMovie || title.Kind == db.TitleTV
}

func (e *Enricher) search(ctx context.Context, title db.Title) ([]tmdb.Result, error) {
	results, err := e.searchWithYear(ctx, title, title.Year)
	if err != nil {
		return nil, err
	}
	// O ano do nome do arquivo às vezes é o da codificação, não o do
	// lançamento. Se ele não achou nada, tenta sem o filtro.
	if len(results) == 0 && title.Year > 0 {
		return e.searchWithYear(ctx, title, 0)
	}
	return results, nil
}

func (e *Enricher) searchWithYear(ctx context.Context, title db.Title, year int) ([]tmdb.Result, error) {
	if title.Kind == db.TitleTV {
		return e.client.SearchTV(ctx, title.Name, year)
	}
	return e.client.SearchMovie(ctx, title.Name, year)
}

// bestMatch pontua os candidatos: semelhança do nome com bônus/penalidade de
// ano, desempatando por popularidade.
func bestMatch(name string, year int, candidates []tmdb.Result) (tmdb.Result, float64) {
	type scored struct {
		result tmdb.Result
		score  float64
	}

	ranked := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		score := Similarity(name, c.DisplayName())
		if year > 0 && c.Year() > 0 {
			switch diff := year - c.Year(); {
			case diff == 0:
				score += 0.15
			case diff == 1 || diff == -1:
				score += 0.05 // lançamento virou o ano entre países
			default:
				// Refilmagens repetem o título exato ("Duna" 1984 e 2021),
				// então o ano distante precisa pesar mais do que a semelhança
				// perfeita do nome. Na dúvida, fica sem match e o usuário
				// corrige à mão.
				score -= 0.35
			}
		}
		if score > 1 {
			score = 1
		}
		ranked = append(ranked, scored{result: c, score: score})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].result.Popularity > ranked[j].result.Popularity
	})

	if len(ranked) == 0 {
		return tmdb.Result{}, 0
	}
	return ranked[0].result, ranked[0].score
}

// apply grava os metadados e baixa as imagens para o cache local.
func (e *Enricher) apply(ctx context.Context, title db.Title, result tmdb.Result, state string) error {
	poster, err := e.client.DownloadImage(ctx, result.PosterPath, tmdb.PosterSize, e.posterDir)
	if err != nil {
		log.Printf("pôster de %q: %v", title.Name, err)
	}
	backdrop, err := e.client.DownloadImage(ctx, result.BackdropPath, tmdb.BackdropSize, e.posterDir)
	if err != nil {
		log.Printf("backdrop de %q: %v", title.Name, err)
	}

	var list []string
	if len(result.Genres) > 0 {
		// Veio do endpoint de detalhe: os nomes já estão aqui.
		for _, g := range result.Genres {
			list = append(list, g.Name)
		}
	} else if names, err := e.client.Genres(ctx); err == nil {
		for _, id := range result.GenreIDs {
			if n, ok := names[id]; ok {
				list = append(list, n)
			}
		}
	}
	genres := strings.Join(list, ", ")

	meta := db.TitleMeta{
		ID:       title.ID,
		Name:     result.DisplayName(),
		Year:     result.Year(),
		Overview: result.Overview,
		Rating:   result.Rating,
		Genres:   genres,
		TMDBID:   result.ID,
		Poster:   poster,
		Backdrop: backdrop,
		State:    state,
	}
	if err := e.db.UpdateTitleMeta(ctx, meta); err != nil {
		return err
	}

	if title.Kind == db.TitleTV {
		e.enrichSeasons(ctx, title.ID, result.ID)
	}
	return nil
}

// enrichSeasons completa os episódios que existem em disco. Falha de rede aqui
// não invalida o título: ele já tem capa e sinopse.
func (e *Enricher) enrichSeasons(ctx context.Context, titleID int64, tmdbID int) {
	seasons, err := e.db.TitleSeasons(ctx, titleID)
	if err != nil {
		log.Printf("temporadas do título %d: %v", titleID, err)
		return
	}

	for _, season := range seasons {
		episodes, err := e.client.Season(ctx, tmdbID, season)
		if err != nil {
			log.Printf("temporada %d do título %d: %v", season, titleID, err)
			continue
		}
		for _, ep := range episodes {
			still, err := e.client.DownloadImage(ctx, ep.StillPath, tmdb.StillSize, e.posterDir)
			if err != nil {
				still = ""
			}
			if err := e.db.UpdateEpisodeMeta(ctx, titleID, season, ep.Number,
				ep.Name, ep.Overview, still, ep.AirDate); err != nil {
				log.Printf("episódio S%02dE%02d: %v", season, ep.Number, err)
			}
		}
	}
}

// localArtwork gera a capa a partir do próprio arquivo quando o TMDB não
// serve: frame do vídeo, arte embutida do áudio ou miniatura da foto.
func (e *Enricher) localArtwork(ctx context.Context, title db.Title) bool {
	if title.Poster != "" {
		return false // já tem capa
	}

	file, err := e.db.FirstFileOfTitle(ctx, title.ID)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			log.Printf("arquivo de %q: %v", title.Name, err)
		}
		return false
	}

	var name string
	switch file.Type {
	case db.TypeVideo:
		name, err = media.VideoFrame(ctx, file.Path, file.MTime, file.Duration, e.posterDir)
	case db.TypeAudio:
		name, err = media.EmbeddedCover(ctx, file.Path, file.MTime, e.posterDir)
	case db.TypePhoto:
		name, err = media.ImageThumb(ctx, file.Path, file.MTime, e.posterDir, 500)
	}
	if err != nil || name == "" {
		// Sem ffmpeg ou sem capa embutida não é falha: é a vida como ela é.
		if err != nil && !errors.Is(err, media.ErrNoFFmpeg) && !errors.Is(err, media.ErrNoArtwork) {
			log.Printf("capa local de %q: %v", title.Name, err)
		}
		return false
	}

	if err := e.db.SetTitlePoster(ctx, title.ID, name); err != nil {
		log.Printf("gravando capa de %q: %v", title.Name, err)
		return false
	}
	return true
}

// SearchCandidates alimenta a correção manual de match na interface.
func (e *Enricher) SearchCandidates(ctx context.Context, kind db.TitleKind, query string, year int) ([]tmdb.Result, error) {
	if !e.client.Enabled() {
		return nil, tmdb.ErrDisabled
	}
	if kind == db.TitleTV {
		return e.client.SearchTV(ctx, query, year)
	}
	return e.client.SearchMovie(ctx, query, year)
}

// ApplyManual força o match escolhido à mão e trava o título como "manual",
// para que um novo scan não desfaça a correção.
func (e *Enricher) ApplyManual(ctx context.Context, titleID int64, tmdbID int) error {
	title, err := e.db.TitleByID(ctx, titleID)
	if err != nil {
		return err
	}
	kind := "movie"
	if title.Kind == db.TitleTV {
		kind = "tv"
	}
	result, err := e.client.Detail(ctx, kind, tmdbID)
	if err != nil {
		return err
	}
	return e.apply(ctx, title, result, "manual")
}
