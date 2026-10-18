// Package estante fala com os aparelhos da sala que ficam na mesma rede do
// Mac: o PS5 (pelo protocolo de descoberta do Remote Play) e a TV Bravia (pela
// API REST da Sony).
package estante

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Portas e versões do DDP, o protocolo de descoberta do Remote Play. O PS4
// ouve na 987 com a versão 00020020; o PS5, na 9302 com a 00030010.
const (
	portaPS4  = 987
	portaPS5  = 9302
	versaoPS4 = "00020020"
	versaoPS5 = "00030010"
)

// EstadoPS5 é o que o console diz sobre si.
type EstadoPS5 string

const (
	PS5Ligado     EstadoPS5 = "ligado"
	PS5Descanso   EstadoPS5 = "descanso"
	PS5Desconhece EstadoPS5 = "desconhecido"
)

// Console é uma resposta ao SRCH.
type Console struct {
	Endereco string            `json:"endereco"`
	Estado   EstadoPS5         `json:"estado"`
	Codigo   int               `json:"codigo"`
	Tipo     string            `json:"tipo,omitempty"` // PS4 ou PS5
	Nome     string            `json:"nome,omitempty"`
	ID       string            `json:"id,omitempty"`
	Jogo     string            `json:"jogo,omitempty"`
	Campos   map[string]string `json:"campos"`
}

// MensagemSRCH monta a busca do DDP.
func MensagemSRCH(versao string) []byte {
	return []byte("SRCH * HTTP/1.1\ndevice-discovery-protocol-version:" + versao + "\n")
}

// InterpretarDDP lê uma resposta: a primeira linha é um status HTTP (200 =
// ligado, 620 = em descanso) e o resto são pares chave:valor.
func InterpretarDDP(pacote []byte) (Console, error) {
	sc := bufio.NewScanner(strings.NewReader(string(pacote)))
	if !sc.Scan() {
		return Console{}, errors.New("ddp: pacote vazio")
	}
	partes := strings.Fields(sc.Text())
	if len(partes) < 2 || !strings.HasPrefix(partes[0], "HTTP/") {
		return Console{}, fmt.Errorf("ddp: status inválido %q", sc.Text())
	}
	codigo, err := strconv.Atoi(partes[1])
	if err != nil {
		return Console{}, fmt.Errorf("ddp: código inválido %q", partes[1])
	}
	c := Console{Codigo: codigo, Campos: map[string]string{}}
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		c.Campos[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	switch codigo {
	case 200:
		c.Estado = PS5Ligado
	case 620:
		c.Estado = PS5Descanso
	default:
		c.Estado = PS5Desconhece
	}
	c.Tipo = c.Campos["host-type"]
	c.Nome = c.Campos["host-name"]
	c.ID = c.Campos["host-id"]
	c.Jogo = c.Campos["running-app-name"]
	return c, nil
}

// Descobrir manda o SRCH por broadcast (geral e de cada interface) e para o
// host dado, se houver, nas portas do PS5 e do PS4, e coleta as respostas
// até o prazo. Um console aparece uma vez por endereço.
func Descobrir(ctx context.Context, host string, prazo time.Duration) ([]Console, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	destinos := broadcasts()
	if host != "" {
		destinos = append(destinos, host)
	}
	var envios int
	var ultimoErr error
	for _, d := range destinos {
		for _, alvo := range []struct {
			porta  int
			versao string
		}{{portaPS5, versaoPS5}, {portaPS4, versaoPS4}} {
			addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(d, strconv.Itoa(alvo.porta)))
			if err != nil {
				ultimoErr = err
				continue
			}
			if _, err := conn.WriteToUDP(MensagemSRCH(alvo.versao), addr); err != nil {
				ultimoErr = err
				continue
			}
			envios++
		}
	}
	if envios == 0 {
		return nil, fmt.Errorf("ddp: nenhum SRCH saiu: %w", ultimoErr)
	}

	fim := time.Now().Add(prazo)
	if d, ok := ctx.Deadline(); ok && d.Before(fim) {
		fim = d
	}
	vistos := map[string]bool{}
	var achados []Console
	buf := make([]byte, 2048)
	for {
		_ = conn.SetReadDeadline(fim)
		n, de, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return achados, nil
			}
			return achados, err
		}
		c, err := InterpretarDDP(buf[:n])
		if err != nil {
			continue // outra coisa respondeu na porta
		}
		c.Endereco = de.IP.String()
		if vistos[c.Endereco] {
			continue
		}
		vistos[c.Endereco] = true
		achados = append(achados, c)
	}
}

// broadcasts devolve 255.255.255.255 e o broadcast dirigido de cada
// interface IPv4 ativa: o macOS nem sempre deixa o geral sair por todas.
func broadcasts() []string {
	out := []string{"255.255.255.255"}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip, mask := ipn.IP.To4(), ipn.Mask
			if len(mask) == net.IPv6len {
				mask = mask[12:]
			}
			b := make(net.IP, 4)
			for i := range b {
				b[i] = ip[i] | ^mask[i]
			}
			out = append(out, b.String())
		}
	}
	return out
}
