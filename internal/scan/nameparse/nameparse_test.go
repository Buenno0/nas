package nameparse

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		title   string
		year    int
		season  int
		episode int
		isEp    bool
	}{
		{"filme com tags", "Duna.Parte.Dois.2024.1080p.WEB-DL.x265-GRUPO.mkv", "Duna Parte Dois", 2024, 0, 0, false},
		{"filme com parênteses", "Interestelar (2014).mkv", "Interestelar", 2014, 0, 0, false},
		{"filme sem ano", "Meu Video De Ferias.mp4", "Meu Video De Ferias", 0, 0, 0, false},
		{"ano no título", "Blade Runner 2049 2017 2160p HDR.mkv", "Blade Runner 2049", 2017, 0, 0, false},
		{"título que começa com ano", "2001 A Space Odyssey.mkv", "2001 A Space Odyssey", 0, 0, 0, false},
		{"pontos no título", "Mr.Robot.S01E01.1080p.mkv", "Mr Robot", 0, 1, 1, true},
		{"episódio com nome", "Serie S02E07 - Nome do Episodio.mkv", "Serie", 0, 2, 7, true},
		{"formato 1x02", "Chaves 1x02 720p.avi", "Chaves", 0, 1, 2, true},
		{"season/episode por extenso", "Show Season 3 Episode 12.mkv", "Show", 0, 3, 12, true},
		{"português", "Serie Temporada 2 Episodio 5.mkv", "Serie", 0, 2, 5, true},
		{"só tags", "1080p.x264-GRUPO.mkv", "GRUPO", 0, 0, 0, false},
		{"colchetes", "[GrupoFansub] Anime - 1080p [ABCD1234].mkv", "GrupoFansub Anime", 0, 0, 0, false},
		{"dublado e legendado", "O.Poderoso.Chefao.1972.Dublado.1080p.mkv", "O Poderoso Chefao", 1972, 0, 0, false},
		{"canais de áudio", "Filme.2020.1080p.DTS.5.1.mkv", "Filme", 2020, 0, 0, false},
		{"multi episódio pega o primeiro", "Serie.S01E01E02.mkv", "Serie", 0, 1, 1, true},
		// Título vazio é proposital: só a pasta sabe o nome da série.
		{"episódio sem nome de série", "S01E02 - Cat in the Bag.mkv", "", 0, 1, 2, true},
		{"episódio nu", "S03E10.mkv", "", 0, 3, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.in)
			if got.Title != tt.title {
				t.Errorf("Title = %q, quero %q", got.Title, tt.title)
			}
			if got.Year != tt.year {
				t.Errorf("Year = %d, quero %d", got.Year, tt.year)
			}
			if got.IsEpisode != tt.isEp {
				t.Errorf("IsEpisode = %v, quero %v", got.IsEpisode, tt.isEp)
			}
			if got.Season != tt.season || got.Episode != tt.episode {
				t.Errorf("S%02dE%02d, quero S%02dE%02d", got.Season, got.Episode, tt.season, tt.episode)
			}
		})
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		title   string
		year    int
		season  int
		episode int
	}{
		{
			"série com pasta de temporada",
			"Breaking Bad/Season 01/S01E02.mkv",
			"Breaking Bad", 0, 1, 2,
		},
		{
			"temporada em português",
			"Cidade dos Homens/Temporada 2/Cidade.dos.Homens.S02E03.mkv",
			"Cidade dos Homens", 0, 2, 3,
		},
		{
			"ano só na pasta",
			"Duna (2021)/duna.1080p.mkv",
			"Duna", 2021, 0, 0,
		},
		{
			"arquivo solto",
			"Interestelar.2014.mkv",
			"Interestelar", 2014, 0, 0,
		},
		{
			"episódio com nome próprio herda a série da pasta",
			"Breaking Bad/Season 01/S01E02 - Cat in the Bag.mkv",
			"Breaking Bad", 0, 1, 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePath(tt.in)
			if got.Title != tt.title {
				t.Errorf("Title = %q, quero %q", got.Title, tt.title)
			}
			if got.Year != tt.year {
				t.Errorf("Year = %d, quero %d", got.Year, tt.year)
			}
			if got.Season != tt.season || got.Episode != tt.episode {
				t.Errorf("S%02dE%02d, quero S%02dE%02d", got.Season, got.Episode, tt.season, tt.episode)
			}
		})
	}
}

func TestSortName(t *testing.T) {
	tests := map[string]string{
		"The Matrix":        "matrix",
		"O Poderoso Chefão": "poderoso chefão",
		"Duna: Parte Dois":  "duna parte dois",
		"  Alien  ":         "alien",
	}
	for in, want := range tests {
		if got := SortName(in); got != want {
			t.Errorf("SortName(%q) = %q, quero %q", in, got, want)
		}
	}
}
