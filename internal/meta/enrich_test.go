package meta

import (
	"testing"

	"nas/internal/meta/tmdb"
)

func TestBestMatch(t *testing.T) {
	candidates := []tmdb.Result{
		{ID: 1, Title: "Duna", ReleaseDate: "2021-09-15", Popularity: 90},
		{ID: 2, Title: "Duna: Parte Dois", ReleaseDate: "2024-02-27", Popularity: 120},
		{ID: 3, Title: "Duna: Parte Três", ReleaseDate: "2026-12-18", Popularity: 60},
	}

	t.Run("o ano desempata entre títulos parecidos", func(t *testing.T) {
		best, score := bestMatch("Duna Parte Dois", 2024, candidates)
		if best.ID != 2 {
			t.Errorf("escolheu %q (id %d), queria Parte Dois", best.DisplayName(), best.ID)
		}
		if score < MinScore {
			t.Errorf("score %.2f abaixo do mínimo %.2f", score, MinScore)
		}
	})

	t.Run("sem ano ainda casa pelo nome", func(t *testing.T) {
		best, score := bestMatch("Duna Parte Dois", 0, candidates)
		if best.ID != 2 || score < MinScore {
			t.Errorf("escolheu id %d com score %.2f", best.ID, score)
		}
	})

	t.Run("ano errado derruba o candidato", func(t *testing.T) {
		// "Duna" (2021) contra um arquivo marcado como 2024: a penalidade de
		// ano tem que impedir que ele seja aceito.
		only := []tmdb.Result{{ID: 1, Title: "Duna", ReleaseDate: "2021-09-15"}}
		_, score := bestMatch("Duna", 2024, only)
		if score >= MinScore {
			t.Errorf("score %.2f deveria ficar abaixo de %.2f", score, MinScore)
		}
	})

	t.Run("nada parecido não vira match", func(t *testing.T) {
		_, score := bestMatch("Aniversário da Vovó 2019", 2019, candidates)
		if score >= MinScore {
			t.Errorf("score %.2f deveria ficar abaixo de %.2f", score, MinScore)
		}
	})

	t.Run("lista vazia", func(t *testing.T) {
		if _, score := bestMatch("Qualquer Coisa", 2020, nil); score != 0 {
			t.Errorf("score %.2f, queria 0", score)
		}
	})
}
