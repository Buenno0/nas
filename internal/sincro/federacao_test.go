package sincro

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
)

// nuvemAoLado monta uma instância cloud que divide o bucket do ambiente a.
func nuvemAoLado(t *testing.T, a *ambiente) *Motor {
	t.Helper()
	database, err := db.OpenAt(filepath.Join(t.TempDir(), "nuvem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return Novo(database, a.chave, func() int64 { return 0 },
		func() config.Nuvem { return config.Nuvem{Papel: config.PapelNuvem} })
}

func TestSnapshotReplicaOMacNaNuvem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := montar(t)
	ctx := context.Background()
	arm, _, _ := a.chave.Hibrido()

	if _, err := a.db.CreateUser(ctx, "karen", "$argon2id$hash", false, true); err != nil {
		t.Fatal(err)
	}
	local := a.arquivoLocal(t, "Stalker (1979).mkv", 1000)
	titulo, _ := a.db.UpsertTitle(ctx, db.Title{LibraryID: a.lib.ID, Kind: "movie", Name: "Stalker", SortName: "stalker", Year: 1979})
	a.db.SetFileTitle(ctx, local.ID, titulo)
	naNuvem := a.arquivoLocal(t, "Solaris (1972).mkv", 1000)
	// O objeto existe de fato no bucket: sem ele, a reconciliação de fundo
	// (que esquece o que foi apagado por fora) rebaixaria o item a local.
	if err := arm.Gravar(ctx, "bibliotecas/1/x/Solaris (1972).mkv", []byte("filme"), ""); err != nil {
		t.Fatal(err)
	}
	a.db.MarcaNaNuvem(ctx, naNuvem.ID, db.LocalAmbos, "bibliotecas/1/x/Solaris (1972).mkv")
	user, _ := a.db.UserByName(ctx, "karen")
	a.db.SaveProgress(ctx, user.ID, naNuvem.ID, 600, 6000)

	if err := a.motor.publicarSnapshot(ctx, arm); err != nil {
		t.Fatal(err)
	}
	nv := nuvemAoLado(t, a)
	if err := nv.carregarSnapshot(ctx, arm); err != nil {
		t.Fatal(err)
	}

	if u, err := nv.db.UserByName(ctx, "karen"); err != nil || u.ID != user.ID || !u.IsAdmin {
		t.Fatalf("usuário não replicado com o mesmo ID: %+v %v", u, err)
	}
	idLocal, err := nv.db.ArquivoPorRef(ctx, db.Ref{Mac: local.Path})
	if err != nil {
		t.Fatal("item só do Mac não chegou à nuvem")
	}
	if f, _ := nv.db.FileByID(ctx, idLocal); f.Localizacao != db.LocalLocal || f.Path != db.PrefixoNoMac+local.Path {
		t.Fatalf("item só do Mac na nuvem: %s em %s", f.Localizacao, f.Path)
	}
	idNuvem, err := nv.db.ArquivoPorRef(ctx, db.Ref{Key: "bibliotecas/1/x/Solaris (1972).mkv"})
	if err != nil {
		t.Fatal("item com cópia no bucket não chegou à nuvem")
	}
	if f, _ := nv.db.FileByID(ctx, idNuvem); f.Localizacao != db.LocalNuvem {
		t.Fatalf("item do bucket deveria tocar da nuvem: %s", f.Localizacao)
	}
	if n, _ := nv.db.ContaEventos(ctx); n != 0 {
		t.Fatalf("aplicar o snapshot gerou %d eventos: ecoaria de volta ao Mac", n)
	}

	// Idempotente: aplicar de novo (forçando) não duplica nada.
	nv.db.GravaEstadoNuvem(ctx, "snapshot_etag", "")
	if err := nv.carregarSnapshot(ctx, arm); err != nil {
		t.Fatal(err)
	}
	var n int
	nv.db.QueryRow(`SELECT COUNT(*) FROM media_files`).Scan(&n)
	if n != 2 {
		t.Fatalf("reaplicar o snapshot deixou %d arquivos, esperados 2", n)
	}

	// Progresso na nuvem mais novo volta ao Mac; um mais velho não passa.
	ref, _ := nv.db.RefDoArquivo(ctx, idNuvem)
	evento := func(id string, pos float64, quando int64) cloud.EventoFederado {
		p, _ := json.Marshal(map[string]any{"user_id": user.ID, "ref": ref, "posicao": pos, "duracao": 6000.0, "updated_at": quando})
		return cloud.EventoFederado{SchemaVersion: 1, EventID: id, Origem: "nuvem", Tipo: "progresso.atualizado", Payload: p}
	}
	if err := a.motor.aplicarFederado(ctx, arm, evento("nuvem-1", 3000, 1<<40)); err != nil {
		t.Fatal(err)
	}
	if err := a.motor.aplicarFederado(ctx, arm, evento("nuvem-2", 10, 1)); err != nil {
		t.Fatal(err)
	}
	var pos float64
	a.db.QueryRow(`SELECT position_sec FROM progress WHERE user_id = ? AND media_file_id = ?`, user.ID, naNuvem.ID).Scan(&pos)
	if pos != 3000 {
		t.Fatalf("LWW: posição no Mac = %v, esperada 3000", pos)
	}
	// Repetido: ignorado.
	if err := a.motor.aplicarFederado(ctx, arm, evento("nuvem-1", 1, 1<<41)); err != nil {
		t.Fatal(err)
	}
	a.db.QueryRow(`SELECT position_sec FROM progress WHERE user_id = ? AND media_file_id = ?`, user.ID, naNuvem.ID).Scan(&pos)
	if pos != 3000 {
		t.Fatal("evento repetido foi aplicado de novo")
	}

	// Removido no Mac → some da nuvem no próximo snapshot.
	a.db.DeleteFilesByID(ctx, []int64{local.ID})
	if err := a.motor.publicarSnapshot(ctx, arm); err != nil {
		t.Fatal(err)
	}
	if err := nv.carregarSnapshot(ctx, arm); err != nil {
		t.Fatal(err)
	}
	if _, err := nv.db.ArquivoPorRef(ctx, db.Ref{Mac: local.Path}); err == nil {
		t.Fatal("item removido no Mac continuou na nuvem")
	}
}

// O disco do container é efêmero: depois de um reinício a capa some do cache,
// mas o banco (Litestream) volta. Pedir a capa a traz de novo do bucket.
func TestPosterVoltaDoBucketDepoisDoReinicio(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := montar(t)
	ctx := context.Background()
	arm, _, _ := a.chave.Hibrido()
	if err := arm.Gravar(ctx, prefixoDosPoster+"w500_x.jpg", []byte("jpeg"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	nv := nuvemAoLado(t, a)
	if !nv.PosterDaNuvem(ctx, "w500_x.jpg") {
		t.Fatal("a capa não voltou do bucket")
	}
	dir, _ := config.PosterDir()
	if dados, err := os.ReadFile(filepath.Join(dir, "w500_x.jpg")); err != nil || string(dados) != "jpeg" {
		t.Fatalf("cache: %q %v", dados, err)
	}
	for _, ruim := range []string{"../ca.key", "a/b.jpg", "inexistente.jpg"} {
		if nv.PosterDaNuvem(ctx, ruim) {
			t.Fatalf("%q não deveria ser servido", ruim)
		}
	}
}
