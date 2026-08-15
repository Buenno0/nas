// Package meta casa os títulos indexados com os metadados do TMDB e cuida das
// capas — oficiais quando há match, geradas localmente quando não há.
package meta

import (
	"strings"
	"unicode"
)

// Acentos mais comuns em títulos em português e espanhol. Um mapa resolve o
// caso sem trazer a dependência de normalização Unicode inteira.
var accents = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'õ': 'o', 'ô': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n', 'ý': 'y',
}

// Normalize deixa o título comparável: minúsculo, sem acento e sem pontuação.
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := true

	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if mapped, ok := accents[r]; ok {
			r = mapped
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// Similarity devolve 0–1 usando o coeficiente de Dice sobre bigramas. É
// tolerante a palavra faltando e a erro de digitação, que é exatamente o tipo
// de ruído que sobra depois de limpar um nome de arquivo.
func Similarity(a, b string) float64 {
	na, nb := Normalize(a), Normalize(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}

	bigramsA := bigrams(na)
	bigramsB := bigrams(nb)
	if len(bigramsA) == 0 || len(bigramsB) == 0 {
		return 0
	}

	counts := make(map[string]int, len(bigramsA))
	for _, bg := range bigramsA {
		counts[bg]++
	}

	matches := 0
	for _, bg := range bigramsB {
		if counts[bg] > 0 {
			counts[bg]--
			matches++
		}
	}
	return 2 * float64(matches) / float64(len(bigramsA)+len(bigramsB))
}

func bigrams(s string) []string {
	runes := []rune(s)
	if len(runes) < 2 {
		return []string{s}
	}
	out := make([]string, 0, len(runes)-1)
	for i := 0; i < len(runes)-1; i++ {
		out = append(out, string(runes[i:i+2]))
	}
	return out
}
