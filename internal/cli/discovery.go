package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/grandcat/zeroconf"
)

// announceOzymandias publica o serviço que clientes de TV descobrem na LAN.
// O registro acompanha exatamente a vida do processo e nunca é feito no modo
// tunnel, onde o servidor escuta apenas no loopback.
func announceOzymandias(port int) (func(), error) {
	name := "Ozymandias"
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		name = "Ozymandias em " + strings.TrimSuffix(host, ".local")
	}
	server, err := zeroconf.Register(
		name,
		"_ozymandias._tcp",
		"local.",
		port,
		[]string{"api=2", "mode=local", "path=/healthz"},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("anunciando Ozymandias por DNS-SD: %w", err)
	}
	return server.Shutdown, nil
}
