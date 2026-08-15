package cli

import (
	"context"
	"fmt"
	"time"

	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/meta"
	"nas/internal/scan"
)

func cmdScan(ctx context.Context, args []string) error {
	force := len(args) > 0 && (args[0] == "--force" || args[0] == "-f")

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
		return fmt.Errorf("nenhuma biblioteca cadastrada: use `nas lib add <caminho>`")
	}

	start := time.Now()
	scanner := scan.New(database)
	scanner.Force = force
	stats, err := scanner.ScanAll(ctx, printProgress)
	if err != nil {
		return err
	}
	fmt.Printf("\r\033[Kscan concluído em %s: %s\n", time.Since(start).Round(time.Millisecond), stats)

	return enrich(ctx, database, false)
}

// enrich busca capas e metadados. Roda logo após o scan e também sozinho,
// via `nas meta [--all]`.
func enrich(ctx context.Context, database *db.DB, all bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	enricher, err := meta.NewEnricher(database, cfg)
	if err != nil {
		return err
	}
	if !enricher.TMDBEnabled() {
		fmt.Println("TMDB sem chave: usando capas geradas do próprio arquivo.")
		fmt.Println("configure com: nas config set tmdb_key <sua chave>")
	}

	stats, err := enricher.Run(ctx, all, func(p meta.Progress) {
		fmt.Printf("\r\033[Kmetadados  %d/%d  %s", p.Current, p.Total, truncate(p.Title, 40))
	})
	if err != nil {
		return err
	}
	fmt.Printf("\r\033[Kmetadados: %s\n", stats)
	return nil
}

func cmdMeta(ctx context.Context, args []string) error {
	all := len(args) > 0 && (args[0] == "--all" || args[0] == "-a")

	database, err := db.Open()
	if err != nil {
		return err
	}
	defer database.Close()

	return enrich(ctx, database, all)
}

// printProgress reescreve sempre a mesma linha do terminal.
func printProgress(p scan.Progress) {
	fmt.Printf("\r\033[K%s  %d/%d  %s", p.Library, p.Current, p.Total, truncate(p.File, 50))
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-max+1:])
}
