package cli

import (
	"strings"
	"testing"
)

func TestNormalizeMDNSName(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		acceptsBare bool
		want        string
	}{
		{"nome do Bonjour", "MacBook-Air-de-Mateus\n", true, "MacBook-Air-de-Mateus.local"},
		{"nome completo", "ozymandias.local.", false, "ozymandias.local"},
		{"hostname comum não é presumido", "servidor", false, ""},
		{"hostname desconhecido", "unknown", true, ""},
		{"localhost não é compartilhável", "localhost", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMDNSName(tt.value, tt.acceptsBare); got != tt.want {
				t.Fatalf("normalizeMDNSName(%q, %v) = %q; quero %q", tt.value, tt.acceptsBare, got, tt.want)
			}
		})
	}
}

func TestMDNSNameUsesTheMacBonjourName(t *testing.T) {
	if name := MDNSName(); name == "" {
		t.Skip("sistema sem nome Bonjour")
	} else if !strings.HasSuffix(strings.ToLower(name), ".local") {
		t.Fatalf("MDNSName() = %q; quero nome terminado em .local", name)
	}
}
