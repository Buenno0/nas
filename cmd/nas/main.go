// Comando nas — servidor de mídia pessoal.
package main

import (
	"log"
	"os"

	"nas/internal/cli"
)

func main() {
	log.SetFlags(log.Ltime)
	os.Exit(cli.Run(os.Args[1:]))
}
