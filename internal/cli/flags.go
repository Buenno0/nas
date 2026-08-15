package cli

import "flag"

// parseInterleaved permite flags depois dos argumentos posicionais
// (`nas lib add ~/Filmes --kind movie`), coisa que o flag padrão não faz:
// ele para na primeira palavra que não começa com "-".
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
}
