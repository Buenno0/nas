package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"nas/internal/db"
)

func cmdLib(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("uso: nas lib <add|ls|rm>")
	}
	switch args[0] {
	case "add":
		return libAdd(ctx, args[1:])
	case "ls", "list":
		return libList(ctx)
	case "rm", "remove":
		return libRemove(ctx, args[1:])
	default:
		return fmt.Errorf("subcomando desconhecido: lib %s", args[0])
	}
}

func libAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("lib add", flag.ContinueOnError)
	kindFlag := fs.String("kind", "", "movie, tv, music ou photo")
	nameFlag := fs.String("name", "", "nome exibido (padrão: nome da pasta)")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("uso: nas lib add <caminho> [--kind movie|tv|music|photo] [--name Nome]")
	}

	path, err := filepath.Abs(positional[0])
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("acessando %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s não é uma pasta", path)
	}

	folder := filepath.Base(path)
	var kind db.Kind
	if *kindFlag != "" {
		kind, err = db.ParseKind(*kindFlag)
		if err != nil {
			return err
		}
	} else {
		var ok bool
		kind, ok = db.GuessKind(folder)
		if !ok {
			return fmt.Errorf("não deu para inferir o tipo pelo nome %q: informe --kind movie|tv|music|photo", folder)
		}
	}

	name := *nameFlag
	if name == "" {
		name = folder
	}

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	lib, err := database.AddLibrary(ctx, name, path, kind)
	if err != nil {
		return err
	}
	fmt.Printf("biblioteca #%d %q (%s) → %s\n", lib.ID, lib.Name, lib.Kind, lib.Path)
	fmt.Println("rode `nas scan` para indexar.")
	return nil
}

func libList(ctx context.Context) error {
	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	libs, err := database.Libraries(ctx)
	if err != nil {
		return err
	}
	if len(libs) == 0 {
		fmt.Println("nenhuma biblioteca. Adicione com: nas lib add ~/Media/Filmes --kind movie")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNOME\tTIPO\tÚLTIMO SCAN\tCAMINHO")
	for _, l := range libs {
		last := "nunca"
		if l.ScannedAt != nil {
			last = l.ScannedAt.Format("02/01 15:04")
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", l.ID, l.Name, l.Kind, last, l.Path)
	}
	return w.Flush()
}

func libRemove(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("uso: nas lib rm <id>")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil {
		return fmt.Errorf("id inválido: %s", args[0])
	}

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	if err := database.DeleteLibrary(ctx, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("biblioteca #%d não existe", id)
		}
		return err
	}
	// Só o índice é removido; nenhum arquivo de mídia é tocado.
	fmt.Printf("biblioteca #%d removida do índice (os arquivos continuam no disco)\n", id)
	return nil
}
