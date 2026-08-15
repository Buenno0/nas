package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"text/tabwriter"

	"golang.org/x/term"

	"nas/internal/auth"
	"nas/internal/db"
)

func cmdUser(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("uso: nas user <add|ls|rm|promote|demote>")
	}
	switch args[0] {
	case "add":
		return userAdd(ctx, args[1:])
	case "ls", "list":
		return userList(ctx)
	case "rm", "remove":
		return userRemove(ctx, args[1:])
	case "promote":
		return userSetAdmin(ctx, args[1:], true)
	case "demote":
		return userSetAdmin(ctx, args[1:], false)
	default:
		return fmt.Errorf("subcomando desconhecido: user %s", args[0])
	}
}

func userAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user add", flag.ContinueOnError)
	admin := fs.Bool("admin", false, "dar poderes de administrador")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("uso: nas user add <usuário> [--admin]")
	}
	username := strings.TrimSpace(positional[0])

	password, err := promptPassword("senha: ")
	if err != nil {
		return err
	}
	confirm, err := promptPassword("confirme: ")
	if err != nil {
		return err
	}
	if password != confirm {
		return errors.New("as senhas não conferem")
	}

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	if err := auth.NewService(database).CreateUser(ctx, username, password, *admin); err != nil {
		return err
	}
	papel := "usuário"
	if *admin {
		papel = "administrador"
	}
	fmt.Printf("%s %q criado\n", papel, username)
	return nil
}

func userList(ctx context.Context) error {
	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	users, err := database.Users(ctx)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSUÁRIO\tPAPEL\tCRIADO EM")
	for _, u := range users {
		papel := "usuário"
		if u.IsAdmin {
			papel = "administrador"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", u.ID, u.Username, papel, u.CreatedAt.Format("02/01/2006"))
	}
	return w.Flush()
}

func userRemove(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("uso: nas user rm <usuário>")
	}
	username := strings.TrimSpace(args[0])

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	if err := database.DeleteUser(ctx, username); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("usuário %q não existe", username)
		}
		return err
	}
	// Progresso, favoritos e sessões saem junto (ON DELETE CASCADE); o acervo
	// em disco não é tocado.
	fmt.Printf("usuário %q removido\n", username)
	return nil
}

func userSetAdmin(ctx context.Context, args []string, admin bool) error {
	if len(args) != 1 {
		return errors.New("uso: nas user promote|demote <usuário>")
	}
	username := strings.TrimSpace(args[0])

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	if err := database.SetAdmin(ctx, username, admin); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("usuário %q não existe", username)
		}
		return err
	}
	if admin {
		fmt.Printf("%q agora é administrador\n", username)
	} else {
		fmt.Printf("%q agora é usuário comum\n", username)
	}
	return nil
}

// promptPassword lê sem ecoar. Fora de um terminal (pipe, script) não há como
// esconder a digitação, então o comando recusa em vez de vazar a senha.
func promptPassword(label string) (string, error) {
	fd := int(syscall.Stdin)
	if !term.IsTerminal(fd) {
		return "", errors.New("este comando precisa de um terminal interativo")
	}
	fmt.Fprint(os.Stderr, label)
	data, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
