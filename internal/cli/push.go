package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"nas/internal/api"
	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/scan"
)

// cmdPush envia um arquivo do Mac para o bucket.
//
// Se o arquivo já está numa biblioteca, ele passa a "ambos": continua tocando
// do disco e ganha a cópia na nuvem. Se não está, entra no catálogo como item
// só da nuvem na biblioteca de --lib.
//
// O envio é retomável: um push interrompido (Ctrl+C, queda, `nas modo local`)
// continua das partes que o bucket já tem na próxima vez que o mesmo arquivo
// for enviado.
func cmdPush(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	libID := fs.Int64("lib", 0, "biblioteca de destino, para arquivos fora de qualquer biblioteca")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("uso: nas push <arquivo> [--lib N]")
	}
	origem, err := filepath.Abs(pos[0])
	if err != nil {
		return err
	}
	info, err := os.Stat(origem)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("push envia um arquivo por vez")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Modo != config.ModoHibrido {
		return errors.New("modo de nuvem local: nada sai do Mac. Use `nas modo hibrido` antes")
	}

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	// A chave própria deste processo obedece ao config.json: `nas modo local`
	// em outro terminal corta o push no meio, e ele retoma depois.
	chave := cloud.Nova(func() config.Nuvem { return cfg.Nuvem }, false)
	if err := chave.Ativar(ctx); err != nil {
		return fmt.Errorf("conectando à nuvem: %w", err)
	}
	defer chave.Desligar()
	go vigiarConfig(ctx, chave)

	arm, nctx, cancel, ok := chave.Vincular(ctx)
	if !ok {
		return cloud.ErrModoLocal
	}
	defer cancel()

	u, err := prepararPush(nctx, database, arm, origem, info.Size(), *libID)
	if err != nil {
		return err
	}
	if u.MediaFileID != nil {
		_ = database.MarcaNaNuvem(ctx, *u.MediaFileID, db.LocalEnviando, "")
	}

	partes, err := enviarPartes(nctx, arm, u, origem)
	if err != nil {
		if u.MediaFileID != nil {
			// Não terminou: o arquivo volta a ser só local. O upload segue
			// registrado e o próximo push retoma.
			_ = database.MarcaNaNuvem(context.WithoutCancel(ctx), *u.MediaFileID, db.LocalLocal, "")
		}
		if chave.Modo() == cloud.ModoLocal {
			return errors.New("kill switch acionado: envio pausado, rode o push de novo no híbrido para retomar")
		}
		return err
	}

	if err := arm.ConcluirEnvio(nctx, u.Key, u.UploadID, partes); err != nil {
		return fmt.Errorf("fechando o envio: %w", err)
	}
	if n, err := arm.Tamanho(nctx, u.Key); err != nil || n != u.Tamanho {
		return fmt.Errorf("o bucket tem %d bytes, esperados %d", n, u.Tamanho)
	}
	if err := database.EstadoDoUpload(ctx, u.ID, "concluido"); err != nil {
		return err
	}

	if u.MediaFileID != nil {
		if err := database.MarcaNaNuvem(ctx, *u.MediaFileID, db.LocalAmbos, u.Key); err != nil {
			return err
		}
		fmt.Printf("\npronto: %s agora está no Mac e na nuvem.\n", filepath.Base(origem))
		return nil
	}
	lib, err := database.Library(ctx, u.LibraryID)
	if err != nil {
		return err
	}
	// O ffprobe lê o arquivo local, que é o mesmo conteúdo: sem tráfego.
	if _, err := scan.New(database).IndexarNuvem(ctx, lib, u.Key, u.Nome, u.Tamanho, origem); err != nil {
		return fmt.Errorf("enviado, mas não indexado: %w", err)
	}
	fmt.Printf("\npronto: %s está na nuvem, na biblioteca %q.\n", filepath.Base(origem), lib.Name)
	return nil
}

// prepararPush retoma um envio pendente do mesmo arquivo ou abre um novo.
func prepararPush(ctx context.Context, database *db.DB, arm cloud.Armazenamento, origem string, tamanho, libID int64) (db.Upload, error) {
	if u, err := database.UploadPendenteDe(ctx, origem); err == nil {
		if u.Tamanho == tamanho {
			fmt.Printf("retomando envio de %s\n", u.Nome)
			return u, nil
		}
		// O arquivo mudou desde o envio interrompido: aquele não serve mais.
		_ = arm.AbortarEnvio(ctx, u.Key, u.UploadID)
		_ = database.EstadoDoUpload(ctx, u.ID, "abortado")
	}

	u := db.Upload{Tamanho: tamanho, Origem: origem, ParteTamanho: api.TamanhoDaParte(tamanho)}
	if fileID, err := database.FileIDPorCaminho(ctx, origem); err == nil {
		f, err := database.FileByID(ctx, fileID)
		if err != nil {
			return db.Upload{}, err
		}
		if f.Localizacao == db.LocalAmbos {
			return db.Upload{}, errors.New("este arquivo já está na nuvem")
		}
		u.MediaFileID = &fileID
		u.LibraryID = f.LibraryID
		u.Nome = f.RelPath
	} else {
		if libID == 0 {
			return db.Upload{}, errors.New("arquivo fora das bibliotecas: diga onde ele entra com --lib N (veja `nas lib ls`)")
		}
		if _, err := database.Library(ctx, libID); err != nil {
			return db.Upload{}, fmt.Errorf("biblioteca %d: %w", libID, err)
		}
		if _, ok := scan.TipoPorExtensao(origem); !ok {
			return db.Upload{}, errors.New("formato que o Ozymandias não indexa")
		}
		u.LibraryID = libID
		u.Nome = filepath.Base(origem)
	}

	key, err := api.ChaveDoUpload(u.LibraryID, u.Nome)
	if err != nil {
		return db.Upload{}, err
	}
	u.Key = key
	if u.UploadID, err = arm.IniciarEnvio(ctx, key, ""); err != nil {
		return db.Upload{}, fmt.Errorf("abrindo envio: %w", err)
	}
	if u.ID, err = database.CriaUpload(ctx, u); err != nil {
		_ = arm.AbortarEnvio(ctx, key, u.UploadID)
		return db.Upload{}, err
	}
	return u, nil
}

// enviarPartes manda o que falta e devolve a lista completa, em ordem.
func enviarPartes(ctx context.Context, arm cloud.Armazenamento, u db.Upload, origem string) ([]cloud.Parte, error) {
	feitas, err := arm.PartesEnviadas(ctx, u.Key, u.UploadID)
	if err != nil {
		return nil, fmt.Errorf("consultando partes: %w", err)
	}
	tem := map[int32]cloud.Parte{}
	for _, p := range feitas {
		tem[p.Numero] = p
	}

	f, err := os.Open(origem)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	total := int32((u.Tamanho + u.ParteTamanho - 1) / u.ParteTamanho)
	inicio := time.Now()
	var enviados int64
	for n := int32(1); n <= total; n++ {
		if _, ok := tem[n]; ok {
			continue
		}
		off := int64(n-1) * u.ParteTamanho
		tam := min(u.ParteTamanho, u.Tamanho-off)
		etag, err := arm.EnviarParte(ctx, u.Key, u.UploadID, n, io.NewSectionReader(f, off, tam), tam)
		if err != nil {
			return nil, fmt.Errorf("parte %d: %w", n, err)
		}
		tem[n] = cloud.Parte{Numero: n, ETag: etag}
		enviados += tam
		mbps := float64(enviados) * 8 / 1e6 / max(time.Since(inicio).Seconds(), 0.001)
		fmt.Printf("\r  %d/%d partes · %.0f Mbps   ", len(tem), total, mbps)
	}

	partes := make([]cloud.Parte, 0, len(tem))
	for _, p := range tem {
		partes = append(partes, p)
	}
	sort.Slice(partes, func(i, j int) bool { return partes[i].Numero < partes[j].Numero })
	return partes, nil
}

// vigiarConfig aciona o kill switch deste processo quando o config.json
// volta para o modo local.
func vigiarConfig(ctx context.Context, chave *cloud.Chave) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if cfg, err := config.Load(); err == nil && cfg.Modo != config.ModoHibrido {
				chave.Desligar()
				return
			}
		}
	}
}
