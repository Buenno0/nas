package cli

import (
	"strings"
	"testing"
)

func TestTerminalQRCode(t *testing.T) {
	qr, err := terminalQRCode("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(qr, "\n"); lines < 10 {
		t.Fatalf("QR Code pequeno demais: %d linhas", lines)
	}
	if !strings.Contains(qr, "\x1b[37;47m") || !strings.Contains(qr, "\x1b[30;40m") {
		t.Fatal("QR Code não inclui os contrastes claro e escuro")
	}
}

func TestTerminalQRCodeChangesWithContent(t *testing.T) {
	first, err := terminalQRCode("https://example.com/um")
	if err != nil {
		t.Fatal(err)
	}
	second, err := terminalQRCode("https://example.com/dois")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("endereços diferentes geraram a mesma saída")
	}
}
