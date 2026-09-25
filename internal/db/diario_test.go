package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestDiarioGravaFiltraEPoda(t *testing.T) {
	ctx := context.Background()
	d, err := OpenAt(filepath.Join(t.TempDir(), "nas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ch, sair := AssinarDiario()
	defer sair()
	d.Anota(ctx, "envio.criado", 7, 0, "filme.mkv", map[string]any{"tamanho": 10})
	d.Anota(ctx, "envio.parte", 7, 0, "filme.mkv", map[string]any{"n": 1})
	d.Anota(ctx, "modo.corte", 0, 0, "", nil)

	if n := <-ch; n.Tipo != "envio.criado" || n.UploadID == nil || *n.UploadID != 7 {
		t.Fatalf("ouvinte recebeu %+v", n)
	}
	envios, err := d.Diario(ctx, FiltroDoDiario{Tipo: "envio."})
	if err != nil || len(envios) != 2 || envios[0].Tipo != "envio.parte" {
		t.Fatalf("filtro por tipo: %v %+v", err, envios)
	}
	todas, _ := d.Diario(ctx, FiltroDoDiario{Antes: envios[0].ID})
	if len(todas) != 1 {
		t.Fatalf("paginação: %+v", todas)
	}

	if _, err := d.ExecContext(ctx, `UPDATE diario SET em = ? WHERE tipo = 'modo.corte'`,
		time.Now().Add(-40*24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if n, err := d.PodaDiario(ctx, 30*24*time.Hour); err != nil || n != 1 {
		t.Fatalf("poda: %d %v", n, err)
	}
}
