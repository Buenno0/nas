// Package media gera as imagens que a interface mostra quando não há capa
// oficial: um frame do vídeo, a arte embutida do MP3 ou a miniatura da foto.
package media

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var (
	ErrNoFFmpeg = errors.New("ffmpeg não encontrado no PATH")
	// ErrNoArtwork é o caso comum de um MP3 sem capa embutida — situação
	// normal, não falha que mereça log.
	ErrNoArtwork = errors.New("arquivo sem capa embutida")
)

var (
	ffmpegOnce sync.Once
	ffmpegPath string
)

func FFmpegPath() (string, error) {
	ffmpegOnce.Do(func() {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegPath = p
		}
	})
	if ffmpegPath == "" {
		return "", ErrNoFFmpeg
	}
	return ffmpegPath, nil
}

// cacheName é estável por (caminho, mtime): o arquivo mudou, a imagem muda de
// nome — e o cache do navegador não serve uma capa velha.
func cacheName(prefix, path string, mtime int64) string {
	sum := sha1.Sum([]byte(path + "|" + strconv.FormatInt(mtime, 10)))
	return prefix + "_" + hex.EncodeToString(sum[:10]) + ".jpg"
}

// VideoFrame extrai um quadro do vídeo como capa. Pega perto de 15% da
// duração para escapar de tela preta e logo de abertura.
func VideoFrame(ctx context.Context, srcPath string, mtime int64, duration float64, destDir string) (string, error) {
	// Cuidado com vídeos curtos: buscar além do fim faz o ffmpeg terminar sem
	// escrever nada. O ponto sempre fica dentro do arquivo.
	seek := 3.0
	if duration > 0 {
		seek = duration * 0.15
		if seek < 1 {
			seek = duration * 0.5
		}
		if seek > duration*0.9 {
			seek = duration * 0.5
		}
	}
	return runFFmpeg(ctx, destDir, cacheName("frame", srcPath, mtime),
		"-ss", strconv.FormatFloat(seek, 'f', 2, 64),
		"-i", srcPath,
		"-frames:v", "1",
		"-vf", "scale=500:-2",
		"-q:v", "4",
	)
}

// EmbeddedCover extrai a capa embutida em arquivos de áudio (tag ID3/FLAC).
func EmbeddedCover(ctx context.Context, srcPath string, mtime int64, destDir string) (string, error) {
	name, err := runFFmpeg(ctx, destDir, cacheName("cover", srcPath, mtime),
		"-i", srcPath,
		"-an",
		"-map", "0:v?",
		"-frames:v", "1",
		"-vf", "scale=500:-2",
		"-q:v", "4",
	)
	if err != nil && !errors.Is(err, ErrNoFFmpeg) {
		// Sem stream de vídeo o ffmpeg reclama; para nós é só "não tem capa".
		return "", ErrNoArtwork
	}
	return name, err
}

// ImageThumb reduz uma foto. Também resolve HEIC, que o Chrome não abre.
func ImageThumb(ctx context.Context, srcPath string, mtime int64, destDir string, width int) (string, error) {
	if width <= 0 {
		width = 500
	}
	// A largura entra no nome: sem isso, a primeira miniatura gerada seria
	// devolvida para todos os tamanhos pedidos depois.
	return runFFmpeg(ctx, destDir, cacheName(fmt.Sprintf("thumb%d", width), srcPath, mtime),
		"-i", srcPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-2", width),
		"-q:v", "4",
	)
}

// Duas proteções sobre a geração de miniaturas, ambas medidas nesta máquina
// (M4, 10 núcleos, um vídeo 1080p de 2h47):
//
//  1. Uma miniatura sozinha custa 199 ms. Dezesseis pedidos simultâneos da
//     MESMA miniatura custavam 2,47 s e saturavam 904% de CPU — dezesseis
//     processos ffmpeg produzindo byte por byte o mesmo JPEG. É o que acontece
//     quando uma página abre com o cache frio: o navegador pede tudo de uma vez.
//     Daí o "voo único": a primeira chamada gera, as outras esperam por ela.
//
//  2. Mesmo para miniaturas distintas, paralelismo não compra vazão aqui — o
//     trabalho é leitura de disco e decodificação, não cálculo. Medido com 16
//     miniaturas diferentes: teto 1 = 3,12 s, teto 4 = 2,22 s, teto 16 = 2,39 s.
//     Passar de 4 não acelera nada e só rouba núcleos do streaming e do ffmpeg
//     de transcodificação, que rodam ao mesmo tempo.
const maxMiniaturasParalelas = 4

type geracao struct {
	pronto chan struct{}
	nome   string
	err    error
}

var (
	geracaoMu  sync.Mutex
	emGeracao  = map[string]*geracao{}
	vagasThumb = make(chan struct{}, maxMiniaturasParalelas)
)

// runFFmpeg escreve em um temporário e renomeia, para o cache nunca conter um
// JPEG truncado. Devolve o nome do arquivo dentro de destDir.
func runFFmpeg(ctx context.Context, destDir, name string, args ...string) (string, error) {
	bin, err := FFmpegPath()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, name)
	if _, err := os.Stat(dest); err == nil {
		return name, nil
	}

	// Voo único por chave de cache. A chave inclui caminho, mtime e largura,
	// então dois pedidos com a mesma chave querem exatamente o mesmo arquivo.
	geracaoMu.Lock()
	g, jaTem := emGeracao[dest]
	if !jaTem {
		g = &geracao{pronto: make(chan struct{})}
		emGeracao[dest] = g
		// A geração roda solta, não dentro da requisição que a pediu. Duas
		// razões: quem pediu primeiro não deve poder matar, ao navegar para
		// outra página, o trabalho que outros clientes estão esperando; e um
		// JPEG de 200 ms abandonado no meio é desperdício puro, enquanto
		// terminá-lo transforma o próximo pedido em acerto de cache. O prazo
		// existe para um arquivo patológico não prender uma vaga para sempre.
		go func() {
			prazo, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancelar()

			g.nome, g.err = geraMiniatura(prazo, bin, destDir, dest, name, args)

			geracaoMu.Lock()
			delete(emGeracao, dest)
			geracaoMu.Unlock()
			close(g.pronto)
		}()
	}
	geracaoMu.Unlock()

	// Daqui para baixo, quem gerou e quem só esperava são a mesma coisa: cada
	// um espera com o SEU prazo e desiste sozinho, sem afetar o trabalho.
	select {
	case <-g.pronto:
		return g.nome, g.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// geraMiniatura é o trabalho em si, já serializado pelo voo único e limitado
// pelo semáforo.
func geraMiniatura(ctx context.Context, bin, destDir, dest, name string, args []string) (string, error) {
	select {
	case vagasThumb <- struct{}{}:
		defer func() { <-vagasThumb }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// Segunda checagem: a fila pode ter durado o bastante para outra chave
	// (largura diferente, mesmo arquivo) já ter deixado esta pronta.
	if _, err := os.Stat(dest); err == nil {
		return name, nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(destDir, "gen-*.jpg")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	full := append([]string{"-y", "-v", "error"}, args...)
	full = append(full, "-f", "image2", tmpName)

	cmd := exec.CommandContext(ctx, bin, full...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg (%s): %s", err, string(out))
	}

	info, err := os.Stat(tmpName)
	if err != nil || info.Size() == 0 {
		return "", errors.New("ffmpeg não produziu imagem")
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", err
	}
	return name, nil
}
