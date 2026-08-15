package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/term"
)

// announceShareURL deixa o endereço pronto para abrir em outro aparelho:
// copia para o clipboard e, em um terminal interativo, imprime um QR Code.
// Falhar ao copiar ou gerar o QR nunca impede o servidor de subir.
func announceShareURL(label, url string) {
	fmt.Printf("\n  ✦ %s\n    %s\n", url, label)

	if err := copyToClipboard(url); err == nil {
		fmt.Println("    copiado para a área de transferência")
	} else {
		fmt.Println("    clipboard indisponível — copie o endereço acima")
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	qr, err := terminalQRCode(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aviso: não consegui gerar o QR Code:", err)
		return
	}
	fmt.Println("\n    aponte a câmera do celular:")
	fmt.Print(qr)
}

// terminalQRCode usa cores explícitas para o código continuar legível em
// terminais claros ou escuros. Cada caractere representa dois módulos na
// vertical, mantendo o QR compacto sem deformá-lo.
func terminalQRCode(content string) (string, error) {
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	bits := qr.Bitmap()
	if len(bits) == 0 {
		return "", errors.New("matriz vazia")
	}

	const (
		reset     = "\x1b[0m"
		bothLight = "\x1b[37;47m "
		bothDark  = "\x1b[30;40m "
		darkLight = "\x1b[30;47m▀"
		lightDark = "\x1b[37;40m▀"
	)

	var out strings.Builder
	for y := 0; y < len(bits); y += 2 {
		out.WriteString("    ")
		for x := range bits[y] {
			top := bits[y][x]
			bottom := false // linha ausente pertence à margem clara do QR
			if y+1 < len(bits) {
				bottom = bits[y+1][x]
			}
			switch {
			case top && bottom:
				out.WriteString(bothDark)
			case top:
				out.WriteString(darkLight)
			case bottom:
				out.WriteString(lightDark)
			default:
				out.WriteString(bothLight)
			}
		}
		out.WriteString(reset)
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func copyToClipboard(text string) error {
	commands := clipboardCommands()
	for _, command := range commands {
		path, err := exec.LookPath(command[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, command[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return errors.New("nenhum utilitário de clipboard disponível")
}

func clipboardCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		return [][]string{{"cmd", "/c", "clip"}}
	default:
		return [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		}
	}
}
