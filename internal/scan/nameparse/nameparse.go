// Package nameparse extrai título, ano e numeração de episódio a partir de
// nomes de arquivo de mídia.
//
// É o ponto mais frágil de qualquer indexador: o nome do arquivo é a única
// pista disponível e vem cheio de ruído de release (resolução, codec, grupo).
// Por isso o pacote é isolado e coberto por testes de tabela.
package nameparse

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Result é o que conseguimos deduzir de um nome de arquivo.
type Result struct {
	Title       string // título limpo, pronto para busca no TMDB
	Year        int    // 0 quando não há ano no nome
	Season      int
	Episode     int
	EpisodeName string // texto após o marcador SxxExx, quando existe
	IsEpisode   bool
}

var episodePatterns = []*regexp.Regexp{
	// O sufixo opcional cobre arquivos multi-episódio (S01E01E02): fica o primeiro.
	regexp.MustCompile(`(?i)\bs(\d{1,2})\s*e(\d{1,3})(?:\s*e\d{1,3})*\b`),
	regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{1,3})\b`),
	regexp.MustCompile(`(?i)\bseason\s*(\d{1,2})\s*episode\s*(\d{1,3})\b`),
	regexp.MustCompile(`(?i)\btemporada\s*(\d{1,2})\s*epis[oó]dio\s*(\d{1,3})\b`),
}

var (
	yearRe        = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	seasonDirRe   = regexp.MustCompile(`(?i)^(season|temporada|s)\s*(\d{1,2})$`)
	multiSpaceRe  = regexp.MustCompile(`\s+`)
	separatorsRe  = regexp.MustCompile(`[._\-\[\]{}()]+`)
	resolutionRe  = regexp.MustCompile(`(?i)^\d{3,4}[pi]$`)
	channelsRe    = regexp.MustCompile(`^\d\.\d$`)
	trailingJunkR = regexp.MustCompile(`(?i)\b(parte|part|cd|disc)\s*\d+$`)
)

// releaseTags são tokens que nunca fazem parte de um título e marcam o começo
// do ruído. O primeiro deles corta o nome.
var releaseTags = map[string]bool{
	"4k": true, "8k": true, "uhd": true, "hd": true, "sd": true, "fullhd": true,
	"x264": true, "x265": true, "h264": true, "h265": true, "avc": true, "hevc": true,
	"av1": true, "xvid": true, "divx": true, "10bit": true, "8bit": true,
	"web": true, "webdl": true, "webrip": true, "dl": true, "hdrip": true,
	"bluray": true, "blueray": true, "bdrip": true, "brrip": true, "bdremux": true,
	"remux": true, "hdtv": true, "dvdrip": true, "dvd": true, "cam": true,
	"telesync": true, "ts": true, "tc": true, "r5": true, "hdcam": true,
	"hdr": true, "hdr10": true, "dv": true, "dolby": true, "vision": true,
	"aac": true, "ac3": true, "eac3": true, "dts": true, "dtshd": true, "truehd": true,
	"atmos": true, "flac": true, "mp3": true, "opus": true,
	"dual": true, "dublado": true, "dubbed": true, "legendado": true, "leg": true,
	"nacional": true, "multi": true, "ptbr": true, "pt": true, "br": true,
	"eng": true, "ita": true, "esp": true, "lat": true, "subbed": true,
	"extended": true, "unrated": true, "uncut": true, "imax": true, "remastered": true,
	"proper": true, "repack": true, "internal": true, "limited": true,
	"complete": true, "completa": true, "final": true,
	"amzn": true, "nf": true, "netflix": true, "hulu": true, "dsnp": true,
	"hmax": true, "atvp": true, "pcok": true, "stan": true, "yts": true, "rarbg": true,
}

// Parse interpreta o nome de um único arquivo.
func Parse(filename string) Result {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	cleaned := normalize(base)

	res := Result{}

	// 1. Marcador de episódio — quando existe, corta o título ali.
	cut := len(cleaned)
	if loc, season, episode, ok := findEpisode(cleaned); ok {
		res.IsEpisode = true
		res.Season = season
		res.Episode = episode
		res.EpisodeName = cleanTitle(stripTags(cleaned[loc[1]:]))
		cut = loc[0]
	}

	// 2. Ano — último ano que não seja a primeira palavra do nome.
	head := cleaned[:cut]
	if idx, year, ok := findYear(head); ok {
		res.Year = year
		cut = idx
	}

	// 3. Primeiro token de release dentro do que sobrou.
	if idx, ok := findFirstTag(cleaned[:cut]); ok {
		cut = idx
	}

	title := cleanTitle(cleaned[:cut])
	if stripTags(title) == "" {
		title = ""
		if !res.IsEpisode {
			// Nome composto só de ruído: recupera o que der do nome inteiro.
			title = cleanTitle(stripTags(cleaned))
		}
		// Em episódio, título vazio é informação: o arquivo se chama apenas
		// "S01E02.mkv" e o nome da série tem que vir da pasta (ver ParsePath).
	}
	res.Title = title
	return res
}

// ParsePath usa também as pastas acima do arquivo. Estruturas comuns:
//
//	Filmes/Duna (2021)/Duna.2021.1080p.mkv
//	Series/Breaking Bad/Season 01/S01E02.mkv
//
// No segundo caso o nome do arquivo não traz a série, então ela vem da pasta.
func ParsePath(relPath string) Result {
	res := Parse(relPath)

	dirs := splitDirs(relPath)
	if len(dirs) == 0 {
		return res
	}

	parent := dirs[len(dirs)-1]

	// "Season 01" / "Temporada 2" não é nome de série: sobe mais um nível.
	showDir := parent
	if m := seasonDirRe.FindStringSubmatch(normalize(parent)); m != nil {
		if res.IsEpisode && res.Season == 0 {
			res.Season, _ = strconv.Atoi(m[2])
		}
		if len(dirs) >= 2 {
			showDir = dirs[len(dirs)-2]
		} else {
			showDir = ""
		}
	}

	if showDir != "" {
		fromDir := Parse(showDir)
		// A pasta só entra quando o arquivo não tem título próprio
		// (ex: "S01E02.mkv") — o nome do arquivo, quando existe, é mais
		// específico do que uma pasta que pode ser "Downloads".
		if res.Title == "" && fromDir.Title != "" {
			res.Title = fromDir.Title
		}
		if res.Year == 0 && fromDir.Year != 0 {
			res.Year = fromDir.Year
		}
	}
	return res
}

func splitDirs(relPath string) []string {
	dir := filepath.Dir(filepath.Clean(relPath))
	if dir == "." || dir == string(filepath.Separator) {
		return nil
	}
	parts := strings.Split(dir, string(filepath.Separator))
	out := parts[:0]
	for _, p := range parts {
		if p != "" && p != "." {
			out = append(out, p)
		}
	}
	return out
}

// normalize troca separadores de release por espaço e colapsa o resultado.
func normalize(s string) string {
	s = separatorsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(multiSpaceRe.ReplaceAllString(s, " "))
}

func findEpisode(s string) (loc []int, season, episode int, ok bool) {
	best := -1
	for _, re := range episodePatterns {
		m := re.FindStringSubmatchIndex(s)
		if m == nil {
			continue
		}
		if best == -1 || m[0] < best {
			best = m[0]
			loc = []int{m[0], m[1]}
			season, _ = strconv.Atoi(s[m[2]:m[3]])
			episode, _ = strconv.Atoi(s[m[4]:m[5]])
			ok = true
		}
	}
	return loc, season, episode, ok
}

// findYear devolve o último ano do texto, ignorando um ano que seja a primeira
// palavra (títulos como "2001 A Space Odyssey").
func findYear(s string) (idx, year int, ok bool) {
	matches := yearRe.FindAllStringIndex(s, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		if m[0] == 0 {
			continue
		}
		y, err := strconv.Atoi(s[m[0]:m[1]])
		if err != nil {
			continue
		}
		return m[0], y, true
	}
	return 0, 0, false
}

func isTag(token string) bool {
	t := strings.ToLower(token)
	return releaseTags[t] || resolutionRe.MatchString(t) || channelsRe.MatchString(t)
}

// findFirstTag devolve o índice, em runas do texto original, do primeiro token
// de release. Índice 0 não conta: um nome que começa com tag não tem título.
func findFirstTag(s string) (int, bool) {
	pos := 0
	for _, token := range strings.Split(s, " ") {
		if pos > 0 && isTag(token) {
			return pos, true
		}
		pos += len(token) + 1
	}
	return 0, false
}

func stripTags(s string) string {
	var kept []string
	for _, token := range strings.Split(s, " ") {
		if token == "" || isTag(token) {
			continue
		}
		kept = append(kept, token)
	}
	return strings.Join(kept, " ")
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(multiSpaceRe.ReplaceAllString(s, " "))
	s = strings.Trim(s, " -_.")
	s = trailingJunkR.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	return capitalizeWords(s)
}

// capitalizeWords deixa apresentável um nome escrito todo em minúsculas
// ("duna parte dois" → "Duna Parte Dois"). Se o nome já tem qualquer
// maiúscula, quem escreveu sabia o que queria — não mexemos, para não
// transformar "Cidade dos Homens" em "Cidade Dos Homens".
func capitalizeWords(s string) string {
	if s != strings.ToLower(s) {
		return s
	}
	words := strings.Split(s, " ")
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// SortName normaliza para ordenação e comparação: minúsculas, sem artigo
// inicial e sem pontuação.
func SortName(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	for _, article := range []string{"the ", "a ", "an ", "o ", "a ", "os ", "as ", "um ", "uma "} {
		if strings.HasPrefix(s, article) {
			s = s[len(article):]
			break
		}
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(multiSpaceRe.ReplaceAllString(b.String(), " "))
}
