package media

import (
	"os/exec"
	"strings"
	"sync"
)

var (
	encodersOnce sync.Once
	encoders     map[string]bool
)

// disponiveis lista os encoders do ffmpeg local, uma vez por processo.
func disponiveis() map[string]bool {
	encodersOnce.Do(func() {
		encoders = map[string]bool{}
		bin, err := FFmpegPath()
		if err != nil {
			return
		}
		saida, err := exec.Command(bin, "-hide_banner", "-encoders").Output()
		if err != nil {
			return
		}
		for _, linha := range strings.Split(string(saida), "\n") {
			campos := strings.Fields(linha)
			if len(campos) >= 2 && strings.HasPrefix(campos[0], "V") || len(campos) >= 2 && strings.HasPrefix(campos[0], "A") {
				encoders[campos[1]] = true
			}
		}
	})
	return encoders
}

// TemEncoder diz se o ffmpeg local sabe usar aquele encoder.
func TemEncoder(nome string) bool { return disponiveis()[nome] }

// encoderAAC prefere o AudioToolbox da Apple, medido em 137x tempo real contra
// 119x do nativo, com o mesmo tamanho de saída. Cai para o nativo em qualquer
// máquina sem ele.
func encoderAAC() string {
	if TemEncoder("aac_at") {
		return "aac_at"
	}
	return "aac"
}

// EncoderDeVideo devolve o encoder de imagem em uso, para a interface poder
// dizer se vai usar hardware.
func EncoderDeVideo() string {
	if TemEncoder("h264_videotoolbox") {
		return "h264_videotoolbox"
	}
	return "libx264"
}
