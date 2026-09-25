package media

import (
	"fmt"
	"strings"
)

// Modo de entrega de um arquivo de vídeo.
type Mode string

const (
	// ModeDirect: o arquivo já toca como está. É o caminho de 99% do acervo.
	ModeDirect Mode = "direct"
	// ModeRemux: streams compatíveis no container errado. Trocar a embalagem
	// custa quase nada — medido em 830x tempo real nesta máquina.
	ModeRemux Mode = "remux"
	// ModeAudio: imagem boa, áudio recusado (AC3, DTS). Recodifica só o som,
	// 137x tempo real.
	ModeAudio Mode = "audio"
	// ModeVideo: imagem incompatível. O caso caro, 9x tempo real.
	ModeVideo Mode = "video"
)

// Caps é o que o cliente declarou saber tocar. Vem dele, não de farejar
// User-Agent: Safari toca HEVC com aceleração de hardware, e mandá-lo para a
// transcodificação queimaria CPU à toa.
//
// Há duas maneiras de preencher isto, e a diferença é quem manda:
//
//   - O SPA responde só as três dúvidas de navegador (HEVC, VP9, AV1) e deixa
//     o resto em branco. Os padrões abaixo — containers, áudios e codecs que
//     todo navegador abre — valem, e é o comportamento que existia antes de
//     haver qualquer cliente nativo.
//   - Um cliente nativo preenche os conjuntos por inteiro. Aí o que ele
//     declarou é a verdade, e nenhum padrão de navegador se aplica: o AVPlayer
//     do iOS não abre WebM nem toca Opus, coisas que todo navegador faz.
//
// Conjunto vazio (nil) significa "usa o padrão"; conjunto preenchido substitui
// o padrão inteiro. Não há mistura — declarar meia lista seria pedir para o
// servidor adivinhar a outra metade.
type Caps struct {
	// As três dúvidas do navegador. Ignoradas quando Video está preenchido.
	HEVC bool
	VP9  bool
	AV1  bool

	// Conjuntos exaustivos de um cliente nativo. Chaves em minúsculas; as
	// extensões vêm sem ponto e são normalizadas na entrada.
	Video      map[string]bool
	Audio      map[string]bool
	Containers map[string]bool
	MaxWidth   int
	MaxHeight  int
}

// Plan é o veredito para um arquivo, com o motivo em português para a interface
// poder explicar em vez de só falhar.
type Plan struct {
	Mode   Mode   `json:"mode"`
	Reason string `json:"reason"`
	// Recipe é a chave da receita de ffmpeg; vazio em ModeDirect.
	Recipe string `json:"recipe,omitempty"`
}

// Containers que os navegadores abrem. O resto precisa de remux, mesmo com
// streams perfeitos dentro.
var containersPadrao = map[string]bool{
	"mp4": true, "m4v": true, "mov": true, "webm": true, "m4a": true, "mp3": true,
}

// Formatos de pixel que os decodificadores aceitam. Fora dessa lista (10 bits,
// 4:2:2, 4:4:4) é tela preta, mesmo em h264 — e isso não é uma limitação de
// navegador: o VideoToolbox do iOS também não decodifica H.264 de 10 bits.
// Por isso a lista é global e não entra na negociação de capacidade.
var pixFmtOK = map[string]bool{
	"":        true, // arquivo antigo, sem probe detalhado: não condena por falta de dado
	"yuv420p": true, "yuvj420p": true, "nv12": true,
}

// Perfis de H.264 decodificados por hardware em qualquer lugar que nos
// interessa. High 10, High 4:2:2 e High 4:4:4 Predictive ficam de fora, pelo
// mesmo motivo de pixFmtOK.
var h264ProfileOK = map[string]bool{
	"":         true,
	"Baseline": true, "Constrained Baseline": true, "Main": true,
	"High": true, "Progressive High": true,
}

// Áudio que os navegadores tocam.
var audioPadrao = map[string]bool{
	"aac": true, "mp3": true, "opus": true, "vorbis": true, "flac": true,
}

// Nomes de codec como as pessoas os escrevem, para o motivo não sair dizendo
// "mpeg2video" a quem só queria assistir um filme.
var nomesDeCodec = map[string]string{
	"h264": "H.264", "hevc": "HEVC", "vp8": "VP8", "vp9": "VP9",
	"av1": "AV1", "vc1": "VC-1", "mpeg2video": "MPEG-2", "mpeg4": "MPEG-4",
	"msmpeg4v3": "DivX", "wmv3": "WMV", "vp6": "VP6", "theora": "Theora",
}

func nomeDeCodec(codec string) string {
	if nome, ok := nomesDeCodec[codec]; ok {
		return nome
	}
	return codec
}

// NormalizaExt tira o ponto e baixa a caixa. O banco guarda ".MKV", o cliente
// manda "mkv", e os dois precisam bater.
func NormalizaExt(ext string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
}

// NormalizaVCodec unifica os apelidos que o ffprobe e os clientes usam para a
// mesma coisa.
func NormalizaVCodec(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	switch c {
	case "h265", "x265":
		return "hevc"
	case "h.264", "x264", "avc", "avc1":
		return "h264"
	case "av01":
		return "av1"
	}
	return c
}

// Conjunto monta um dos mapas de Caps a partir da lista que o cliente mandou.
// Devolve nil para lista vazia — que é o sinal de "usa o padrão".
func Conjunto(itens []string, normaliza func(string) string) map[string]bool {
	if len(itens) == 0 {
		return nil
	}
	m := make(map[string]bool, len(itens))
	for _, item := range itens {
		if chave := normaliza(item); chave != "" {
			m[chave] = true
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// decodifica diz se a imagem passa pelo nome do codec. O pixel e o perfil são
// checados depois, por videoIncompativel.
func (c Caps) decodifica(vcodec string) bool {
	if len(c.Video) > 0 {
		return c.Video[vcodec]
	}
	switch vcodec {
	case "h264", "vp8", "theora":
		return true
	case "hevc":
		return c.HEVC
	case "vp9":
		return c.VP9
	case "av1":
		return c.AV1
	default:
		return false
	}
}

func (c Caps) toca(acodec string) bool {
	if len(c.Audio) > 0 {
		return c.Audio[acodec]
	}
	return audioPadrao[acodec]
}

func (c Caps) abre(ext string) bool {
	if len(c.Containers) > 0 {
		return c.Containers[ext]
	}
	return containersPadrao[ext]
}

// Decide devolve o que fazer com o arquivo, na ordem de custo: primeiro checa a
// imagem (mais caro de consertar), depois o som, depois a embalagem.
//
// trocouAudio diz que o espectador escolheu uma faixa que não é a padrão do
// arquivo. Isso sozinho já impede o direct play: <video> não expõe troca de
// faixa de áudio fora do Safari, então quem tem de escolher a faixa é o
// servidor, remontando o arquivo com aquela faixa como a única.
func Decide(ext, vcodec, pixFmt, vprofile, acodec string, trocouAudio bool, caps Caps) Plan {
	return DecideWithSize(ext, vcodec, pixFmt, vprofile, acodec, 0, 0, trocouAudio, caps)
}

// DecideWithSize inclui o limite físico declarado por clientes de TV. Os
// clientes antigos omitem maxw/maxh e continuam passando zeros, preservando o
// comportamento anterior.
func DecideWithSize(ext, vcodec, pixFmt, vprofile, acodec string, width, height int, trocouAudio bool, caps Caps) Plan {
	ext = NormalizaExt(ext)
	vcodec = NormalizaVCodec(vcodec)
	acodec = strings.ToLower(strings.TrimSpace(acodec))

	if (caps.MaxWidth > 0 && width > caps.MaxWidth) || (caps.MaxHeight > 0 && height > caps.MaxHeight) {
		return Plan{
			Mode: ModeVideo, Recipe: "video1080",
			Reason: fmt.Sprintf("o vídeo em %dx%d excede o limite de %dx%d deste aparelho", width, height, caps.MaxWidth, caps.MaxHeight),
		}
	}
	if motivo, ok := videoIncompativel(vcodec, pixFmt, vprofile, caps); !ok {
		return Plan{Mode: ModeVideo, Recipe: "video1080", Reason: motivo}
	}
	if acodec != "" && !caps.toca(acodec) {
		return Plan{
			Mode:   ModeAudio,
			Recipe: "audio",
			Reason: fmt.Sprintf("a imagem é compatível, mas o áudio em %s não toca neste aparelho", acodec),
		}
	}
	if !caps.abre(ext) {
		return Plan{
			Mode:   ModeRemux,
			Recipe: "remux",
			Reason: fmt.Sprintf("%s e %s tocam aqui, mas o container %s não abre", nomeDeCodec(vcodec), acodec, ext),
		}
	}
	if trocouAudio {
		return Plan{
			Mode:   ModeRemux,
			Recipe: "remux",
			Reason: "o arquivo tocaria direto, mas trocar a faixa de áudio exige reembalá-lo",
		}
	}
	return Plan{Mode: ModeDirect, Reason: "o arquivo toca como está"}
}

// videoIncompativel devolve (motivo, ok). ok=true significa que a imagem passa.
func videoIncompativel(vcodec, pixFmt, vprofile string, caps Caps) (string, bool) {
	if vcodec == "" {
		return "", true // só áudio; a imagem não é o problema
	}
	if !caps.decodifica(vcodec) {
		return fmt.Sprintf("este aparelho não decodifica %s", nomeDeCodec(vcodec)), false
	}
	// Passar pelo nome não basta: h264 é o codec mais compatível que existe e
	// mesmo assim tem variantes que nenhum decodificador de hardware abre.
	if vcodec == "h264" {
		if !pixFmtOK[strings.ToLower(pixFmt)] {
			return fmt.Sprintf("h264 em %s (10 bits ou croma 4:2:2/4:4:4) não é decodificado por hardware", pixFmt), false
		}
		if !h264ProfileOK[vprofile] {
			return fmt.Sprintf("o perfil h264 %q não é decodificado por hardware", vprofile), false
		}
	}
	return "", true
}

// FaixaDeAudio é o mínimo de uma faixa para escolher qual tocar.
type FaixaDeAudio struct {
	Index   int
	Codec   string
	Lang    string
	Default bool
}

// AudioAlternativo acha, quando a faixa padrão não toca neste aparelho, outra
// faixa do MESMO idioma que toque (o AAC estéreo que muitos releases trazem
// ao lado do eac3/dts). Com ela, basta reembalar em vez de recodificar o
// áudio: mais rápido e sem perda. ok=false quando a padrão já toca ou não há
// alternativa — aí vale a decisão de sempre.
func AudioAlternativo(faixas []FaixaDeAudio, caps Caps) (FaixaDeAudio, bool) {
	var padrao *FaixaDeAudio
	for i := range faixas {
		if faixas[i].Default {
			padrao = &faixas[i]
			break
		}
	}
	if padrao == nil && len(faixas) > 0 {
		padrao = &faixas[0] // sem disposição marcada, o ffmpeg usa a primeira
	}
	if padrao == nil || caps.toca(strings.ToLower(padrao.Codec)) {
		return FaixaDeAudio{}, false
	}
	idioma := strings.ToLower(padrao.Lang)
	for _, f := range faixas {
		if f.Index == padrao.Index || !caps.toca(strings.ToLower(f.Codec)) {
			continue
		}
		if strings.ToLower(f.Lang) == idioma {
			return f, true
		}
	}
	return FaixaDeAudio{}, false
}
