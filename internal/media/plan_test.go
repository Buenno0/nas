package media

import "testing"

// A tabela cobre os quatro fixtures gerados para o pipeline e os casos que a
// heurística antiga do frontend errava.
func TestDecide(t *testing.T) {
	casos := []struct {
		nome     string
		ext      string
		vcodec   string
		pixFmt   string
		vprofile string
		acodec   string
		trocou   bool
		caps     Caps
		quero    Mode
	}{
		// Fixtures reais (ids 3 a 6 no banco de teste).
		{"mp4 h264/aac toca direto", ".mp4", "h264", "yuv420p", "High", "aac", false, Caps{}, ModeDirect},
		{"mkv h264/aac só troca container", ".mkv", "h264", "yuv420p", "High", "aac", false, Caps{}, ModeRemux},
		{"mkv h264/ac3 recodifica só o som", ".mkv", "h264", "yuv420p", "High", "ac3", false, Caps{}, ModeAudio},
		{"mp4 hevc/ac3 recodifica a imagem", ".mp4", "hevc", "yuv420p", "Main", "ac3", false, Caps{}, ModeVideo},

		// O buraco que a heurística do frontend deixava passar: h264 legítimo
		// que navegador nenhum decodifica.
		{"h264 High 10 é tela preta", ".mp4", "h264", "yuv420p10le", "High 10", "aac", false, Caps{}, ModeVideo},
		{"h264 4:2:2 é tela preta", ".mp4", "h264", "yuv422p", "High 4:2:2", "aac", false, Caps{}, ModeVideo},

		// Negociação de capacidade: Safari toca HEVC por hardware; transcodificar
		// para ele seria queimar CPU de graça.
		{"hevc com cliente capaz toca direto", ".mp4", "hevc", "yuv420p", "Main", "aac", false, Caps{HEVC: true}, ModeDirect},
		{"hevc em mkv com cliente capaz só remuxa", ".mkv", "hevc", "yuv420p", "Main", "aac", false, Caps{HEVC: true}, ModeRemux},
		{"av1 sem suporte recodifica", ".mp4", "av1", "yuv420p", "Main", "opus", false, Caps{}, ModeVideo},
		{"av1 com suporte toca direto", ".mp4", "av1", "yuv420p", "Main", "opus", false, Caps{AV1: true}, ModeDirect},

		// A imagem manda: áudio ruim junto de vídeo ruim resolve os dois de uma vez.
		{"vídeo e áudio ruins viram um job só", ".mkv", "vc1", "yuv420p", "", "dts", false, Caps{}, ModeVideo},

		// Áudio puro não deve cair no caminho de vídeo.
		{"mp3 toca direto", ".mp3", "", "", "", "mp3", false, Caps{}, ModeDirect},
		{"flac em container próprio toca direto", ".m4a", "", "", "", "flac", false, Caps{}, ModeDirect},

		// Arquivo indexado antes da migration 0005, sem pix_fmt: não condenar por
		// falta de dado.
		{"sem probe detalhado não é condenado", ".mp4", "h264", "", "", "aac", false, Caps{}, ModeDirect},

		// Trocar a faixa de áudio impede o direct play mesmo com tudo
		// compatível: <video> não troca faixa fora do Safari, então quem
		// escolhe é o servidor, remontando o arquivo.
		{"trocar de faixa força remux", ".mp4", "h264", "yuv420p", "High", "aac", true, Caps{}, ModeRemux},
		{"trocar de faixa em mkv continua remux", ".mkv", "h264", "yuv420p", "High", "aac", true, Caps{}, ModeRemux},
		{"trocar de faixa com áudio ruim continua no áudio", ".mkv", "h264", "yuv420p", "High", "ac3", true, Caps{}, ModeAudio},
		{"trocar de faixa com vídeo ruim continua no vídeo", ".mp4", "hevc", "yuv420p", "Main", "aac", true, Caps{}, ModeVideo},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := Decide(c.ext, c.vcodec, c.pixFmt, c.vprofile, c.acodec, c.trocou, c.caps)
			if got.Mode != c.quero {
				t.Errorf("modo = %q (motivo: %s), quero %q", got.Mode, got.Reason, c.quero)
			}
			if got.Mode != ModeDirect && got.Recipe == "" {
				t.Error("modo que exige ffmpeg ficou sem receita")
			}
			if got.Reason == "" {
				t.Error("todo veredito precisa de motivo legível")
			}
		})
	}
}

func TestDecideWithSizePreparaVideoAcimaDoLimiteDaTV(t *testing.T) {
	plan := DecideWithSize(
		"mkv", "h264", "yuv420p", "High", "aac",
		3840, 2160, false,
		Caps{
			Video: map[string]bool{"h264": true}, Audio: map[string]bool{"aac": true},
			Containers: map[string]bool{"mkv": true}, MaxWidth: 1920, MaxHeight: 1080,
		},
	)
	if plan.Mode != ModeVideo || plan.Recipe != "video1080" {
		t.Fatalf("plano = %+v, quero transcodificação video1080", plan)
	}
}

// perfilIOS é o que um cliente AVFoundation declara: nada de WebM, nada de
// Opus, nada de VP9 — três coisas que todo navegador faz e que faziam o
// servidor mandar um arquivo intocável para o aparelho.
var perfilIOS = Caps{
	Video:      Conjunto([]string{"h264", "hevc"}, NormalizaVCodec),
	Audio:      Conjunto([]string{"aac", "mp3", "alac", "flac", "ac3", "eac3"}, func(s string) string { return s }),
	Containers: Conjunto([]string{"mp4", "m4v", "mov", "m4a", "mp3", "flac"}, NormalizaExt),
}

// O mesmo acervo, dois aparelhos, vereditos diferentes. Cada caso aqui é um
// arquivo que o navegador toca direto e o iPhone não — ou o contrário.
func TestDecidePerfilNativo(t *testing.T) {
	casos := []struct {
		nome     string
		ext      string
		vcodec   string
		pixFmt   string
		vprofile string
		acodec   string
		caps     Caps
		quero    Mode
	}{
		// O buraco que motivou tudo: o navegador abre, o AVPlayer não.
		{"webm vp9/opus toca no navegador", ".webm", "vp9", "yuv420p", "", "opus", Caps{VP9: true}, ModeDirect},
		{"webm vp9/opus não existe para o iPhone", ".webm", "vp9", "yuv420p", "", "opus", perfilIOS, ModeVideo},

		// Opus em container aceito: a imagem está boa, só o som não passa.
		{"mp4 h264/opus toca no navegador", ".mp4", "h264", "yuv420p", "High", "opus", Caps{}, ModeDirect},
		{"mp4 h264/opus recodifica só o som no iPhone", ".mp4", "h264", "yuv420p", "High", "opus", perfilIOS, ModeAudio},

		// E o contrário: o iPhone decodifica HEVC por hardware sem ninguém
		// perguntar, e transcodificar para ele seria queimar CPU de graça.
		{"mp4 hevc/aac toca direto no iPhone", ".mp4", "hevc", "yuv420p", "Main", "aac", perfilIOS, ModeDirect},
		{"mp4 hevc/aac sem declaração vira o caso caro", ".mp4", "hevc", "yuv420p", "Main", "aac", Caps{}, ModeVideo},

		// AC3 é aceito pelo perfil, então o que sobra é a embalagem: remux, e
		// não o caminho mais caro do áudio. A ordem de custo tem de valer aqui
		// também.
		{"mkv h264/ac3 no iPhone é só a embalagem", ".mkv", "h264", "yuv420p", "High", "ac3", perfilIOS, ModeRemux},
		{"mkv h264/ac3 no navegador ainda recodifica o som", ".mkv", "h264", "yuv420p", "High", "ac3", Caps{}, ModeAudio},

		// Áudio puro: o iPhone abre .flac cru, o navegador precisa de remux.
		{"flac cru toca direto no iPhone", ".flac", "", "", "", "flac", perfilIOS, ModeDirect},
		{"flac cru precisa de embalagem no navegador", ".flac", "", "", "", "flac", Caps{}, ModeRemux},

		// Declarar áudio não pode afrouxar a imagem: os três conjuntos são
		// independentes.
		{"declarar áudio não perdoa AV1", ".mp4", "av1", "yuv420p", "Main", "aac", perfilIOS, ModeVideo},

		// 10 bits continua sendo tela preta em todo lugar, inclusive no
		// VideoToolbox — nenhum perfil compra isso declarando "h264".
		{"h264 10 bits não passa nem declarado", ".mp4", "h264", "yuv420p10le", "High 10", "aac", perfilIOS, ModeVideo},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := Decide(c.ext, c.vcodec, c.pixFmt, c.vprofile, c.acodec, false, c.caps)
			if got.Mode != c.quero {
				t.Errorf("modo = %q (motivo: %s), quero %q", got.Mode, got.Reason, c.quero)
			}
		})
	}
}

// A extensão vem do disco com ponto e caixa qualquer; o cliente a declara sem
// ponto e em minúscula. Os dois lados têm de bater, ou um .MKV vazaria como se
// fosse um container conhecido.
func TestNormalizacaoDeEntrada(t *testing.T) {
	caps := Caps{
		Video:      Conjunto([]string{"H264", " h265 "}, NormalizaVCodec),
		Audio:      Conjunto([]string{"aac"}, func(s string) string { return s }),
		Containers: Conjunto([]string{".MP4", "mov "}, NormalizaExt),
	}

	if p := Decide(".MP4", "H.264", "yuv420p", "High", "aac", false, caps); p.Mode != ModeDirect {
		t.Errorf("extensão em caixa alta: modo = %q (%s), quero direct", p.Mode, p.Reason)
	}
	// "h265" declarado tem de valer para o "hevc" que o ffprobe grava.
	if p := Decide(".mp4", "hevc", "yuv420p", "Main", "aac", false, caps); p.Mode != ModeDirect {
		t.Errorf("apelido h265/hevc: modo = %q (%s), quero direct", p.Mode, p.Reason)
	}
	if p := Decide(".mkv", "h264", "yuv420p", "High", "aac", false, caps); p.Mode != ModeRemux {
		t.Errorf("container fora do conjunto: modo = %q (%s), quero remux", p.Mode, p.Reason)
	}
}

// Conjunto vazio significa "usa o padrão", e é o que mantém o SPA funcionando
// exatamente como antes de existir cliente nativo.
func TestConjuntoVazioCaiNoPadrao(t *testing.T) {
	if m := Conjunto(nil, NormalizaExt); m != nil {
		t.Errorf("lista nula virou %v, quero nil", m)
	}
	if m := Conjunto([]string{"", "  "}, NormalizaExt); m != nil {
		t.Errorf("lista só de lixo virou %v, quero nil", m)
	}
}

func TestAudioAlternativoPrefereAACDoMesmoIdioma(t *testing.T) {
	faixas := []FaixaDeAudio{
		{Index: 1, Codec: "eac3", Lang: "por", Default: true},
		{Index: 2, Codec: "aac", Lang: "eng"},
		{Index: 3, Codec: "aac", Lang: "por"},
	}
	f, ok := AudioAlternativo(faixas, Caps{})
	if !ok || f.Index != 3 {
		t.Fatalf("queria a faixa 3 (aac por), veio %+v %v", f, ok)
	}
	// Só outro idioma: não troca a língua de quem assiste.
	if _, ok := AudioAlternativo(faixas[:2], Caps{}); ok {
		t.Fatal("trocou de idioma para achar um codec compatível")
	}
	// A padrão já toca: nada muda.
	if _, ok := AudioAlternativo([]FaixaDeAudio{{Index: 1, Codec: "aac", Default: true}, {Index: 2, Codec: "aac"}}, Caps{}); ok {
		t.Fatal("trocou uma faixa que já tocava")
	}
}
