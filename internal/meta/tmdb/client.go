// Package tmdb fala com a API do The Movie Database.
//
// Nada aqui é obrigatório: sem chave configurada, o Client fica desligado e o
// resto do NAS funciona igual, apenas sem sinopse, nota e capa oficial.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase   = "https://api.themoviedb.org/3"
	imageBase = "https://image.tmdb.org/t/p"

	PosterSize   = "w500"
	BackdropSize = "w1280"
	StillSize    = "w300"
)

var ErrDisabled = errors.New("TMDB sem chave configurada")

type Client struct {
	key      string
	lang     string
	http     *http.Client
	throttle <-chan time.Time
}

// New cria o cliente. Chave vazia devolve um cliente desligado (Enabled falso).
func New(key, lang string) *Client {
	if lang == "" {
		lang = "pt-BR"
	}
	return &Client{
		key:  strings.TrimSpace(key),
		lang: lang,
		http: &http.Client{Timeout: 20 * time.Second},
		// ~10 requisições por segundo: bem abaixo do limite do TMDB e
		// gentil com a rede de casa.
		throttle: time.Tick(100 * time.Millisecond),
	}
}

func (c *Client) Enabled() bool { return c != nil && c.key != "" }

// Result é um candidato de busca.
type Result struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Name         string  `json:"name"` // séries usam "name"
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	Overview     string  `json:"overview"`
	Rating       float64 `json:"vote_average"`
	Popularity   float64 `json:"popularity"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`

	// A busca devolve ids de gênero; o endpoint de detalhe devolve os nomes.
	GenreIDs []int `json:"genre_ids"`
	Genres   []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"genres"`
}

// DisplayName resolve a diferença entre filmes (title) e séries (name).
func (r Result) DisplayName() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}

// Year extrai o ano da data de lançamento/estreia.
func (r Result) Year() int {
	date := r.ReleaseDate
	if date == "" {
		date = r.FirstAirDate
	}
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

type searchResponse struct {
	Results []Result `json:"results"`
}

// SearchMovie e SearchTV buscam candidatos por título e, opcionalmente, ano.
func (c *Client) SearchMovie(ctx context.Context, title string, year int) ([]Result, error) {
	params := url.Values{"query": {title}}
	if year > 0 {
		params.Set("year", strconv.Itoa(year))
	}
	return c.search(ctx, "/search/movie", params)
}

func (c *Client) SearchTV(ctx context.Context, title string, year int) ([]Result, error) {
	params := url.Values{"query": {title}}
	if year > 0 {
		params.Set("first_air_date_year", strconv.Itoa(year))
	}
	return c.search(ctx, "/search/tv", params)
}

func (c *Client) search(ctx context.Context, path string, params url.Values) ([]Result, error) {
	var resp searchResponse
	if err := c.get(ctx, path, params, &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// Genres traduz os ids de gênero em nomes, com cache por idioma.
type genreCache struct {
	names map[int]string
}

var genresByLang = map[string]*genreCache{}

func (c *Client) Genres(ctx context.Context) (map[int]string, error) {
	if cached, ok := genresByLang[c.lang]; ok {
		return cached.names, nil
	}
	names := map[int]string{}
	for _, path := range []string{"/genre/movie/list", "/genre/tv/list"} {
		var resp struct {
			Genres []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"genres"`
		}
		if err := c.get(ctx, path, nil, &resp); err != nil {
			return nil, err
		}
		for _, g := range resp.Genres {
			names[g.ID] = g.Name
		}
	}
	genresByLang[c.lang] = &genreCache{names: names}
	return names, nil
}

// Episode é um episódio de uma temporada.
type Episode struct {
	Number    int     `json:"episode_number"`
	Name      string  `json:"name"`
	Overview  string  `json:"overview"`
	StillPath string  `json:"still_path"`
	AirDate   string  `json:"air_date"`
	Rating    float64 `json:"vote_average"`
}

// Season devolve os episódios de uma temporada de série.
func (c *Client) Season(ctx context.Context, tvID, season int) ([]Episode, error) {
	var resp struct {
		Episodes []Episode `json:"episodes"`
	}
	path := fmt.Sprintf("/tv/%d/season/%d", tvID, season)
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Episodes, nil
}

// Detail busca um item específico por id (usado na correção manual de match).
func (c *Client) Detail(ctx context.Context, kind string, id int) (Result, error) {
	var r Result
	path := fmt.Sprintf("/%s/%d", kind, id)
	if err := c.get(ctx, path, nil, &r); err != nil {
		return Result{}, err
	}
	return r, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", c.key)
	params.Set("language", c.lang)

	endpoint := apiBase + path + "?" + params.Encode()

	var lastErr error
	for attempt := range 3 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.throttle:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			wait := 2 * time.Second
			if v := resp.Header.Get("Retry-After"); v != "" {
				if secs, err := strconv.Atoi(v); err == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			resp.Body.Close()
			lastErr = errors.New("TMDB pediu para esperar (429)")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue

		case resp.StatusCode == http.StatusUnauthorized:
			resp.Body.Close()
			return errors.New("chave do TMDB inválida")

		case resp.StatusCode != http.StatusOK:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return fmt.Errorf("TMDB %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return err
	}
	return fmt.Errorf("TMDB indisponível: %w", lastErr)
}

// DownloadImage baixa uma imagem para o cache local. O front nunca busca no
// TMDB: além de privacidade, o acervo continua com capas se a internet cair.
func (c *Client) DownloadImage(ctx context.Context, imagePath, size, destDir string) (string, error) {
	if imagePath == "" {
		return "", nil
	}
	name := size + "_" + strings.TrimPrefix(imagePath, "/")
	dest := filepath.Join(destDir, name)

	if _, err := os.Stat(dest); err == nil {
		return name, nil // já em cache
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-c.throttle:
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageBase+"/"+size+imagePath, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("baixando imagem: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imagem %s: %s", imagePath, resp.Status)
	}

	// Escreve em temporário e renomeia: nunca deixa um arquivo pela metade
	// no cache se o download for interrompido.
	tmp, err := os.CreateTemp(destDir, "img-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, 12<<20)); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return "", err
	}
	return name, nil
}
