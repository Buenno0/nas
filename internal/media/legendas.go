package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Legendas do acervo chegam em SRT, ASS ou dentro do próprio MKV. O elemento
// <track> do navegador aceita um formato só, WebVTT — então tudo é convertido
// aqui, sob demanda e com cache.
//
// A conversão é separada do preparo do vídeo de propósito: legenda liga e
// desliga durante a reprodução, e embutir uma no MP4 significaria reembalar o
// filme inteiro a cada troca.

// codecsDeImagem são legendas que são figura, não texto: vêm de DVD e Blu-ray
// como bitmaps sobrepostos. Converter exigiria OCR, que é outro projeto — e
// falhar com motivo é melhor que oferecer uma opção que só dá erro.
var codecsDeImagem = map[string]bool{
	"hdmv_pgs_subtitle": true,
	"dvd_subtitle":      true,
	"dvb_subtitle":      true,
	"xsub":              true,
}

// ErrLegendaDeImagem é devolvido para legenda que não é texto.
type ErrLegendaDeImagem struct{ Codec string }

func (e ErrLegendaDeImagem) Error() string {
	return fmt.Sprintf("a legenda está em %s, que é imagem e não texto: converter exigiria OCR", e.Codec)
}

// PedidoLegenda identifica uma legenda a converter.
type PedidoLegenda struct {
	// Origem é o vídeo, para legenda embutida, ou o próprio .srt, para a que
	// mora em arquivo ao lado.
	Origem string
	MTime  int64
	Codec  string
	// Indice é o stream dentro do vídeo. Negativo significa arquivo separado:
	// aí a origem já É a legenda e não há o que mapear.
	Indice int
	// Identidade substitui Origem na chave do cache quando a origem é uma URL
	// assinada, que muda a cada pedido.
	Identidade string
}

const versaoDasLegendas = 1

func (p PedidoLegenda) chave() string {
	id := p.Origem
	if p.Identidade != "" {
		id = p.Identidade
	}
	return fmt.Sprintf("legenda_v%d_%s.vtt", versaoDasLegendas,
		hashCurto(fmt.Sprintf("%s|%d|s%d", id, p.MTime, p.Indice)))
}

// LegendaVTT devolve o caminho de um WebVTT pronto, convertendo na primeira vez.
//
// Sem voo único: a conversão custa menos de um segundo e produz poucos KB, e a
// gravação é em temporário com rename atômico — dois pedidos simultâneos no
// pior caso desperdiçam uma conversão, nunca corrompem o arquivo.
func (p *Preparador) LegendaVTT(ctx context.Context, pedido PedidoLegenda) (string, error) {
	if codecsDeImagem[strings.ToLower(pedido.Codec)] {
		return "", ErrLegendaDeImagem{Codec: pedido.Codec}
	}

	destino := filepath.Join(p.dir, pedido.chave())
	if info, err := os.Stat(destino); err == nil && info.Size() > 0 {
		p.tocar(destino, info)
		return destino, nil
	}

	bin, err := FFmpegPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return "", err
	}

	temp, err := os.CreateTemp(p.dir, "legenda-*.vtt")
	if err != nil {
		return "", err
	}
	tempNome := temp.Name()
	temp.Close()
	defer os.Remove(tempNome)

	args := []string{"-nostdin", "-y", "-v", "error", "-i", pedido.Origem}
	if pedido.Indice >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:%d", pedido.Indice))
	}
	args = append(args, "-f", "webvtt", tempNome)

	cmd := exec.CommandContext(ctx, bin, args...)
	if saida, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("convertendo legenda: %s", strings.TrimSpace(string(saida)))
	}

	info, err := os.Stat(tempNome)
	if err != nil || info.Size() == 0 {
		return "", fmt.Errorf("a conversão não produziu legenda")
	}
	if err := os.Rename(tempNome, destino); err != nil {
		return "", err
	}
	return destino, nil
}

// EhLegendaDeImagem diz se o codec é bitmap, para a API poder explicar em vez
// de oferecer uma legenda que só daria erro ao ser clicada.
func EhLegendaDeImagem(codec string) bool {
	return codecsDeImagem[strings.ToLower(codec)]
}

// FFprobeParaTeste expõe o binário do ffprobe para os testes conferirem o que
// o pipeline produziu. Não é usado em produção: o servidor lê metadados pelo
// pacote scan.
func FFprobeParaTeste() (string, error) { return exec.LookPath("ffprobe") }
