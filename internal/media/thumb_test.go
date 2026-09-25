package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// videoGerado cria um vídeo de teste com o próprio ffmpeg, para os testes não
// dependerem do acervo de ninguém. Um só por execução do pacote.
var (
	videoUmaVez sync.Once
	videoPath   string
)

func videoDeTeste(tb testing.TB) string {
	tb.Helper()
	if _, err := FFmpegPath(); err != nil {
		tb.Skip("sem ffmpeg")
	}
	videoUmaVez.Do(func() {
		bin, _ := FFmpegPath()
		dir, err := os.MkdirTemp("", "nas-thumb-*")
		if err != nil {
			return
		}
		caminho := filepath.Join(dir, "fonte.mp4")
		// 1080p e 40s: pesado o bastante para a decodificação custar algo
		// mensurável, leve o bastante para gerar em poucos segundos.
		cmd := exec.Command(bin, "-y", "-v", "error",
			"-f", "lavfi", "-i", "testsrc=duration=40:size=1920x1080:rate=25",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
			caminho)
		if out, err := cmd.CombinedOutput(); err != nil {
			tb.Logf("gerando vídeo de teste: %v — %s", err, out)
			return
		}
		videoPath = caminho
	})
	if videoPath == "" {
		tb.Skip("não foi possível gerar o vídeo de teste")
	}
	return videoPath
}

// contaFFmpeg conta processos ffmpeg do sistema. Conta os de fora também, então
// quem usa desconta a linha de base.
func contaFFmpeg() int {
	saida, err := exec.Command("pgrep", "-x", "ffmpeg").Output()
	if err != nil {
		return 0 // pgrep sai com 1 quando não há nenhum
	}
	n := 0
	for _, linha := range strings.Split(strings.TrimSpace(string(saida)), "\n") {
		if linha != "" {
			n++
		}
	}
	return n
}

// picoDeFFmpeg roda fn amostrando processos ffmpeg e devolve o pico acima da
// linha de base — um transcode do servidor rodando ao lado não falseia o teste.
func picoDeFFmpeg(fn func()) int {
	base := contaFFmpeg()
	pico := 0
	var mu sync.Mutex
	pare := make(chan struct{})
	fim := make(chan struct{})

	go func() {
		defer close(fim)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-pare:
				return
			case <-tick.C:
				if n := contaFFmpeg() - base; n > 0 {
					mu.Lock()
					if n > pico {
						pico = n
					}
					mu.Unlock()
				}
			}
		}
	}()

	fn()
	close(pare)
	<-fim

	mu.Lock()
	defer mu.Unlock()
	return pico
}

func rajada(tb testing.TB, dir string, n int, mtimeDe func(i int) int64) time.Duration {
	tb.Helper()
	fonte := videoDeTeste(tb)
	inicio := time.Now()

	var wg sync.WaitGroup
	erros := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, erros[i] = VideoFrame(context.Background(), fonte, mtimeDe(i), 40, dir)
		}(i)
	}
	wg.Wait()

	for i, err := range erros {
		if err != nil {
			tb.Fatalf("pedido %d falhou: %v", i, err)
		}
	}
	return time.Since(inicio)
}

// Uma página abrindo com o cache frio pede tudo de uma vez. Quando os pedidos
// são da MESMA miniatura, tem de rodar um ffmpeg só — antes do voo único, 16
// pedidos viravam 16 processos idênticos, 2,47s e 904% de CPU.
func TestVooUnicoDeMiniatura(t *testing.T) {
	dir := t.TempDir()

	var decorrido time.Duration
	pico := picoDeFFmpeg(func() {
		decorrido = rajada(t, dir, 16, func(int) int64 { return 42 })
	})

	t.Logf("16 pedidos da mesma miniatura: %v, pico de %d ffmpeg", decorrido.Round(time.Millisecond), pico)

	if pico > 1 {
		t.Fatalf("%d processos ffmpeg para uma única miniatura: o voo único não está funcionando", pico)
	}
	itens, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(itens) != 1 {
		t.Fatalf("%d arquivos no cache, quero 1", len(itens))
	}
}

// Miniaturas distintas podem rodar em paralelo, mas não sem teto: sem o
// semáforo, uma grade de capas fria saturava a máquina e roubava núcleos do
// streaming e da transcodificação, que rodam ao mesmo tempo.
func TestSemaforoDeMiniaturas(t *testing.T) {
	dir := t.TempDir()

	var decorrido time.Duration
	pico := picoDeFFmpeg(func() {
		decorrido = rajada(t, dir, 16, func(i int) int64 { return int64(1000 + i) })
	})

	t.Logf("16 miniaturas distintas: %v, pico de %d ffmpeg (teto %d)",
		decorrido.Round(time.Millisecond), pico, maxMiniaturasParalelas)

	if pico > maxMiniaturasParalelas {
		t.Fatalf("pico de %d processos, teto é %d", pico, maxMiniaturasParalelas)
	}
	itens, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(itens) != 16 {
		t.Fatalf("%d arquivos no cache, quero 16", len(itens))
	}
}

// Quem desiste não estraga o trabalho dos outros: o cliente cancelado recebe
// erro, mas a miniatura continua sendo gerada e fica no cache.
func TestCancelarNaoPerdeOTrabalho(t *testing.T) {
	fonte := videoDeTeste(t)
	dir := t.TempDir()

	ctx, cancelar := context.WithCancel(context.Background())
	erro := make(chan error, 1)
	go func() {
		_, err := VideoFrame(ctx, fonte, 777, 40, dir)
		erro <- err
	}()

	// Tempo de o trabalho começar, e então o cliente desaparece.
	time.Sleep(30 * time.Millisecond)
	cancelar()
	if err := <-erro; err == nil {
		t.Log("o trabalho terminou antes do cancelamento; teste inconclusivo mas não falho")
	}

	// A geração segue solta: em pouco tempo o arquivo tem de aparecer.
	prazo := time.Now().Add(30 * time.Second)
	for time.Now().Before(prazo) {
		if itens, _ := os.ReadDir(dir); len(itens) == 1 {
			nome := itens[0].Name()
			if strings.HasPrefix(nome, "gen-") {
				time.Sleep(50 * time.Millisecond) // ainda é o temporário
				continue
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	itens, _ := os.ReadDir(dir)
	nomes := []string{}
	for _, i := range itens {
		nomes = append(nomes, i.Name())
	}
	t.Fatalf("o cancelamento jogou o trabalho fora; cache tem %v", nomes)
}
