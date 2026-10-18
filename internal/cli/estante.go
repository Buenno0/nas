package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sort"
	"time"

	"nas/internal/estante"
)

// cmdEstante: nas estante sondar — a sonda da Fase 0 do PLANO-ESTANTE.md.
// Só lê: não liga, não desliga e não mexe no volume.
func cmdEstante(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "sondar" {
		return errors.New("uso: nas estante sondar [--ps5 IP] [--tv URL-da-descrição] [--espera 3s]")
	}
	fs := flag.NewFlagSet("estante sondar", flag.ContinueOnError)
	ps5 := fs.String("ps5", "", "IP do PS5 (além do broadcast)")
	tv := fs.String("tv", "", "URL da descrição UPnP da TV (sem ela, procura por SSDP)")
	espera := fs.Duration("espera", 3*time.Second, "quanto esperar pelas respostas")
	if _, err := parseInterleaved(fs, args[1:]); err != nil {
		return err
	}

	fmt.Println("== PS5 (DDP) ==")
	consoles, err := estante.Descobrir(ctx, *ps5, *espera)
	if err != nil {
		fmt.Println("  erro:", err)
	}
	if len(consoles) == 0 {
		fmt.Println("  ninguém respondeu. Confira: PS5 na tomada; em descanso, \"continuar conectado")
		fmt.Println("  à internet\" ligado; e a permissão de Rede Local do macOS para este terminal.")
	}
	for _, c := range consoles {
		imprimirConsole(c)
	}

	fmt.Println()
	fmt.Println("== TV (UPnP/DLNA) ==")
	var tvs []*estante.TV
	if *tv != "" {
		t, err := estante.AbrirTV(ctx, *tv, nil)
		if err != nil {
			return err
		}
		tvs = append(tvs, t)
	} else {
		tvs, err = estante.ProcurarTVs(ctx, *espera)
		if err != nil {
			fmt.Println("  erro no SSDP:", err)
		}
	}
	if len(tvs) == 0 {
		fmt.Println("  nenhum renderizador DLNA respondeu. A TV só aparece na rede quando está ligada.")
	}
	for _, t := range tvs {
		sondarTV(ctx, t)
	}
	return nil
}

func imprimirConsole(c estante.Console) {
	fmt.Printf("  %s  %s %q  %s (%d)\n", c.Endereco, c.Tipo, c.Nome, c.Estado, c.Codigo)
	if c.Jogo != "" {
		fmt.Printf("    jogando: %s\n", c.Jogo)
	}
	chaves := make([]string, 0, len(c.Campos))
	for k := range c.Campos {
		chaves = append(chaves, k)
	}
	sort.Strings(chaves)
	for _, k := range chaves {
		fmt.Printf("    %s: %s\n", k, c.Campos[k])
	}
}

func sondarTV(ctx context.Context, t *estante.TV) {
	fmt.Printf("  %s (%s %s)\n    descrição: %s\n", t.Nome, t.Fabricante, t.Modelo, t.Descricao)
	if v, err := t.Volume(ctx); err != nil {
		fmt.Println("    volume: erro:", err)
	} else {
		fmt.Println("    volume:", v)
	}
	if m, err := t.Mudo(ctx); err != nil {
		fmt.Println("    mudo: erro:", err)
	} else {
		fmt.Println("    mudo:", m)
	}
	if s, err := t.Transporte(ctx); err != nil {
		fmt.Println("    reprodução DLNA: erro:", err)
	} else {
		fmt.Println("    reprodução DLNA:", s)
	}
}
