// Package cli implementa a interface de linha de comando do NAS.
//
// `nas` sem argumentos abre o menu interativo (1 = local, 2 = tunnel).
// Os subcomandos existem para uso não-interativo e scripts.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// Version é sobrescrito no build via -ldflags.
var Version = "dev"

const usage = `Ozymandias — servidor de mídia pessoal

Uso:
  nas                      menu interativo (1 = local, 2 = tunnel)
  nas serve [--local|--tunnel] [--port N] [--sem-nuvem]
  nas modo [local|hibrido] modo de nuvem (local = kill switch, zero AWS)
  nas push <arquivo> [--lib N] envia ao bucket (modo híbrido)
  nas worker [--uma-vez]   processador da nuvem (roda no container da AWS)
  nas serve --nuvem        instância cloud do Ozymandias (container da AWS)
  nas lib add <caminho> [--kind movie|tv|music|photo]
  nas lib ls
  nas scan                 indexa os arquivos e busca metadados
  nas meta [--all]         só metadados (--all refaz tudo)
  nas user add <usuário> [--admin]
  nas user ls | rm <usuário> | promote <usuário> | demote <usuário>
  nas passwd <usuário>     redefine a senha (recuperação de acesso)
  nas status
  nas stop
  nas config [set <chave> <valor>]
  nas version
`

// Run executa o comando pedido e devolve o código de saída do processo.
func Run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		return runMenu(ctx)
	}

	cmd, rest := args[0], args[1:]
	var err error

	switch cmd {
	case "serve":
		err = cmdServe(ctx, rest)
	case "lib":
		err = cmdLib(ctx, rest)
	case "scan":
		err = cmdScan(ctx, rest)
	case "meta":
		err = cmdMeta(ctx, rest)
	case "user":
		err = cmdUser(ctx, rest)
	case "passwd":
		err = cmdPasswd(ctx, rest)
	case "config":
		err = cmdConfig(rest)
	case "modo":
		err = cmdModo(rest)
	case "push":
		err = cmdPush(ctx, rest)
	case "worker":
		err = cmdWorker(ctx, rest)
	case "status":
		err = cmdStatus()
	case "stop":
		err = cmdStop()
	case "version", "--version", "-v":
		fmt.Println("nas", Version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n%s", cmd, usage)
		return 2
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	return 0
}
