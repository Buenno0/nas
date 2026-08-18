package cli

import (
	"net"
	"os"
	"strings"
)

// MDNSName devolve o nome da máquina na rede local (Bonjour/mDNS), do tipo
// "macbook-air-de-mateus.local". Vale mais que o IP: não muda quando o
// roteador dá outro endereço. Vazio quando o sistema não expõe um nome .local.
func MDNSName() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	host = strings.TrimSuffix(host, ".")
	if !strings.HasSuffix(strings.ToLower(host), ".local") {
		return ""
	}
	return host
}

// LANIP tenta descobrir o IP da máquina na rede local, para imprimir uma URL
// que outros dispositivos consigam abrir. Não abre conexão de fato — um socket
// UDP só resolve qual interface o sistema usaria para sair.
func LANIP() string {
	conn, err := net.Dial("udp", "192.168.0.1:1")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP.IsPrivate() {
			return addr.IP.String()
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil && ip4.IsPrivate() {
				return ip4.String()
			}
		}
	}
	return "127.0.0.1"
}
