package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Exercita o executor de verdade nos fixtures gerados, com ffmpeg real.
func TestPrepararFixtures(t *testing.T) {
	fix := "/private/tmp/claude-501/-Users-buenno-nas/d39706d6-4675-4458-9ce9-e8da6fa460dd/scratchpad/fixtures"
	if _, err := os.Stat(fix); err != nil {
		t.Skip("fixtures ausentes")
	}
	dir := t.TempDir()
	p := NovoPreparador(dir, 500<<20, 1<<30, 2)

	casos := []struct{ arquivo, receita string }{
		{"caso-container.mkv", "remux"},
		{"caso-audio.mkv", "audio"},
		{"caso-video.mp4", "video1080"},
	}

	for _, c := range casos {
		t.Run(c.receita, func(t *testing.T) {
			origem := filepath.Join(fix, c.arquivo)
			info, err := os.Stat(origem)
			if err != nil {
				t.Fatal(err)
			}
			pedido := Pedido{FileID: 1, Origem: origem, MTime: info.ModTime().Unix(), Duracao: 20, Receita: c.receita}

			if got := p.Consultar(pedido); got.Estado != EstadoAusente {
				t.Fatalf("estado inicial = %q, quero ausente", got.Estado)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			inicio := time.Now()
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

			saida, err := os.Stat(final.Arquivo)
			if err != nil || saida.Size() == 0 {
				t.Fatalf("arquivo preparado inválido: %v", err)
			}
			t.Logf("%s: %s em %s, %d KB (origem %d KB)",
				c.receita, NomeDaReceita(c.receita), time.Since(inicio).Round(time.Millisecond),
				saida.Size()/1024, info.Size()/1024)

			// Segundo pedido tem de bater no cache, sem rodar ffmpeg de novo.
			t2 := time.Now()
			if got := p.Consultar(pedido); got.Estado != EstadoPronto {
				t.Error("cache não reconheceu o arquivo pronto")
			} else if time.Since(t2) > 50*time.Millisecond {
				t.Error("consulta ao cache lenta demais para ser cache")
			}
		})
	}
}

// Dois pedidos simultâneos do mesmo arquivo devem compartilhar um ffmpeg só.
func TestPreparoDeduplicado(t *testing.T) {
	fix := "/private/tmp/claude-501/-Users-buenno-nas/d39706d6-4675-4458-9ce9-e8da6fa460dd/scratchpad/fixtures/caso-container.mkv"
	info, err := os.Stat(fix)
	if err != nil {
		t.Skip("fixture ausente")
	}
	p := NovoPreparador(t.TempDir(), 500<<20, 1<<30, 2)
	pedido := Pedido{FileID: 9, Origem: fix, MTime: info.ModTime().Unix(), Duracao: 20, Receita: "remux"}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for range 5 {
		if _, err := p.Pedir(ctx, pedido); err != nil {
			t.Fatal(err)
		}
	}
	p.mu.Lock()
	n := len(p.trabalhos)
	p.mu.Unlock()
	if n != 1 {
		t.Errorf("%d trabalhos para o mesmo arquivo, quero 1", n)
	}
	if _, err := p.Esperar(ctx, pedido); err != nil {
		t.Fatal(err)
	}
}
