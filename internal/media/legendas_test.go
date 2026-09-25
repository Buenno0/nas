package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDual gera um MKV com dois áudios (por/eng) e duas legendas, que é o
// arranjo que o acervo brasileiro tem de verdade.
func fixtureDual(t *testing.T) string {
	t.Helper()
	bin, err := FFmpegPath()
	if err != nil {
		t.Skip("sem ffmpeg")
	}
	dir := t.TempDir()

	pt := filepath.Join(dir, "pt.srt")
	os.WriteFile(pt, []byte("1\n00:00:01,000 --> 00:00:03,000\nlegenda em portugues\n\n"), 0o644)
	en := filepath.Join(dir, "en.srt")
	os.WriteFile(en, []byte("1\n00:00:01,000 --> 00:00:03,000\nsubtitle in english\n\n"), 0o644)

	saida := filepath.Join(dir, "dual.mkv")
	cmd := exec.Command(bin, "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=duration=5:size=320x180:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=5",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=5",
		"-i", pt, "-i", en,
		"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3", "-map", "4",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-c:s", "srt",
		"-metadata:s:a:0", "language=por", "-metadata:s:a:0", "title=Dublado",
		"-metadata:s:a:1", "language=eng", "-metadata:s:a:1", "title=Original",
		"-metadata:s:s:0", "language=por", "-metadata:s:s:1", "language=eng",
		saida)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("não foi possível gerar o fixture: %v — %s", err, out)
	}
	return saida
}

// A legenda embutida tem de virar WebVTT de verdade, com o cabeçalho que o
// <track> exige e o texto original preservado.
func TestLegendaEmbutidaViraWebVTT(t *testing.T) {
	origem := fixtureDual(t)
	p := NovoPreparador(t.TempDir(), 500<<20, 1<<30, 2)

	// Stream 3 é a primeira legenda (0=vídeo, 1 e 2=áudios).
	caminho, err := p.LegendaVTT(context.Background(), PedidoLegenda{
		Origem: origem, MTime: 1, Codec: "subrip", Indice: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	dados, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatal(err)
	}
	texto := string(dados)
	if !strings.HasPrefix(texto, "WEBVTT") {
		t.Fatalf("sem cabeçalho WEBVTT, o <track> ignora o arquivo. Veio: %.40q", texto)
	}
	if !strings.Contains(texto, "legenda em portugues") {
		t.Fatalf("o texto da legenda se perdeu na conversão: %q", texto)
	}
	// A segunda chamada tem de bater no cache, não converter de novo.
	segundo, err := p.LegendaVTT(context.Background(), PedidoLegenda{
		Origem: origem, MTime: 1, Codec: "subrip", Indice: 3,
	})
	if err != nil || segundo != caminho {
		t.Fatalf("o cache não pegou: %v, %q vs %q", err, segundo, caminho)
	}
}

// Cada legenda do arquivo é uma legenda diferente — o índice tem de entrar na
// chave do cache, senão a segunda devolveria a primeira.
func TestLegendasNaoColidemNoCache(t *testing.T) {
	origem := fixtureDual(t)
	p := NovoPreparador(t.TempDir(), 500<<20, 1<<30, 2)
	ctx := context.Background()

	ptCaminho, err := p.LegendaVTT(ctx, PedidoLegenda{Origem: origem, MTime: 1, Codec: "subrip", Indice: 3})
	if err != nil {
		t.Fatal(err)
	}
	enCaminho, err := p.LegendaVTT(ctx, PedidoLegenda{Origem: origem, MTime: 1, Codec: "subrip", Indice: 4})
	if err != nil {
		t.Fatal(err)
	}
	if ptCaminho == enCaminho {
		t.Fatal("as duas legendas foram para o mesmo arquivo de cache")
	}

	ptTexto, _ := os.ReadFile(ptCaminho)
	enTexto, _ := os.ReadFile(enCaminho)
	if !strings.Contains(string(ptTexto), "portugues") {
		t.Errorf("legenda 3 devia ser a portuguesa: %q", ptTexto)
	}
	if !strings.Contains(string(enTexto), "english") {
		t.Errorf("legenda 4 devia ser a inglesa: %q", enTexto)
	}
}

// Legenda de Blu-ray é figura, não texto. Melhor recusar com motivo do que
// gastar um ffmpeg para falhar.
func TestLegendaDeImagemERecusadaComMotivo(t *testing.T) {
	p := NovoPreparador(t.TempDir(), 500<<20, 1<<30, 2)
	_, err := p.LegendaVTT(context.Background(), PedidoLegenda{
		Origem: "/nao/importa.mkv", Codec: "hdmv_pgs_subtitle", Indice: 2,
	})
	var imagem ErrLegendaDeImagem
	if !errors.As(err, &imagem) {
		t.Fatalf("erro = %v, queria ErrLegendaDeImagem", err)
	}
	if !strings.Contains(err.Error(), "OCR") {
		t.Errorf("o motivo devia explicar o porquê: %q", err)
	}
}

// A faixa escolhida tem de estar no arquivo preparado — é o ponto de tudo isto.
func TestPreparoRespeitaAFaixaEscolhida(t *testing.T) {
	origem := fixtureDual(t)
	p := NovoPreparador(t.TempDir(), 500<<20, 1<<30, 2)
	ctx := context.Background()

	// Stream 2 é o segundo áudio ("Original", eng, 880 Hz).
	pedido := Pedido{FileID: 1, Origem: origem, MTime: 1, Duracao: 5, Receita: "remux", Audio: 2}
	if _, err := p.Pedir(ctx, pedido); err != nil {
		t.Fatal(err)
	}
	final, err := p.Esperar(ctx, pedido)
	if err != nil {
		t.Fatal(err)
	}
	if final.Estado != EstadoPronto {
		t.Fatalf("estado = %q, erro = %q", final.Estado, final.Erro)
	}

	// O preparado tem de ter UMA faixa de áudio, e ela tem de ser a inglesa.
	bin, _ := FFprobeParaTeste()
	saida, err := exec.Command(bin, "-v", "quiet", "-print_format", "json",
		"-show_streams", "-select_streams", "a", final.Arquivo).Output()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(saida), `"codec_type"`); n != 1 {
		t.Fatalf("o preparado tem %d faixas de áudio, quero exatamente 1", n)
	}
	if !strings.Contains(string(saida), "eng") {
		t.Fatalf("a faixa do preparado não é a escolhida (eng): %s", saida)
	}

	// E a chave do cache tem de separar as faixas.
	outro := pedido
	outro.Audio = 1
	if pedido.Chave() == outro.Chave() {
		t.Fatal("faixas diferentes com a mesma chave: o cache serviria o áudio errado")
	}
}
