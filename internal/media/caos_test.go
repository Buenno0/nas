package media

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Caos: um ffmpeg lendo uma URL da nuvem que parou de responder. A geração de
// miniatura roda solta da requisição (para servir a outros clientes), mas o
// kill switch ainda precisa matá-la. Uma FIFO sem escritor faz o papel da URL
// travada: o ffmpeg fica bloqueado para sempre abrindo a entrada.
func TestCaosMiniaturaMorreComOKillSwitch(t *testing.T) {
	if _, err := FFmpegPath(); err != nil {
		t.Skip("sem ffmpeg")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "travada.mkv")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("sem mkfifo: %v", err)
	}

	vida, killSwitch := context.WithCancel(context.Background())
	ctx := ComVida(context.Background(), vida)

	fim := make(chan error, 1)
	go func() {
		_, err := VideoFrameDe(ctx, "nuvem:bibliotecas/1/x/travada.mkv", fifo, 1, 60, filepath.Join(dir, "thumbs"))
		fim <- err
	}()

	// Dá tempo de o ffmpeg subir e travar na entrada.
	select {
	case err := <-fim:
		t.Fatalf("o ffmpeg terminou antes do kill switch: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	inicio := time.Now()
	killSwitch()
	select {
	case err := <-fim:
		if err == nil {
			t.Fatal("miniatura sem entrada não pode dar certo")
		}
		if d := time.Since(inicio); d > time.Second {
			t.Fatalf("o ffmpeg levou %s para morrer", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o ffmpeg sobreviveu ao kill switch (o prazo normal é de 2 min)")
	}
}
