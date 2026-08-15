package meta

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"O Poderoso Chefão": "o poderoso chefao",
		"Duna: Parte Dois":  "duna parte dois",
		"WALL·E":            "wall e",
		"  Alien   (1979) ": "alien 1979",
		"Amélie":            "amelie",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, quero %q", in, got, want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	// Pares que precisam casar bem alto.
	high := [][2]string{
		{"Duna Parte Dois", "Duna: Parte Dois"},
		{"O Poderoso Chefao", "O Poderoso Chefão"},
		{"Interestelar", "Interestelar"},
		{"Breaking Bad", "breaking bad"},
	}
	for _, pair := range high {
		if score := Similarity(pair[0], pair[1]); score < 0.85 {
			t.Errorf("Similarity(%q, %q) = %.2f, esperava ≥ 0.85", pair[0], pair[1], score)
		}
	}

	// Pares que não podem ser confundidos.
	low := [][2]string{
		{"Duna", "Duna Parte Dois"},
		{"Interestelar", "Star Wars"},
		{"Breaking Bad", "Better Call Saul"},
	}
	for _, pair := range low {
		if score := Similarity(pair[0], pair[1]); score >= 0.85 {
			t.Errorf("Similarity(%q, %q) = %.2f, esperava < 0.85", pair[0], pair[1], score)
		}
	}
}
