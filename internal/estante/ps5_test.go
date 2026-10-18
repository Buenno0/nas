package estante

import "testing"

func TestInterpretarDDP(t *testing.T) {
	ligado := "HTTP/1.1 200 Ok\nhost-id:0123456789AB\nhost-type:PS5\nhost-name:PS5-123\n" +
		"host-request-port:997\ndevice-discovery-protocol-version:00030010\nsystem-version:09400004\n" +
		"running-app-name:Astro Bot\nrunning-app-titleid:PPSA01234\n"
	c, err := InterpretarDDP([]byte(ligado))
	if err != nil {
		t.Fatal(err)
	}
	if c.Estado != PS5Ligado || c.Tipo != "PS5" || c.Nome != "PS5-123" || c.Jogo != "Astro Bot" || c.Campos["host-request-port"] != "997" {
		t.Fatalf("ligado lido errado: %+v", c)
	}

	c, err = InterpretarDDP([]byte("HTTP/1.1 620 Server Standby\r\nhost-type:PS5\r\nhost-name:PS5-123\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Estado != PS5Descanso || c.Nome != "PS5-123" || c.Jogo != "" {
		t.Fatalf("descanso lido errado: %+v", c)
	}

	// O eco da própria busca (broadcast volta para quem mandou) não é resposta.
	if _, err := InterpretarDDP(MensagemSRCH(versaoPS5)); err == nil {
		t.Fatal("SRCH lido como resposta")
	}
}

func TestMensagemSRCH(t *testing.T) {
	if got := string(MensagemSRCH(versaoPS5)); got != "SRCH * HTTP/1.1\ndevice-discovery-protocol-version:00030010\n" {
		t.Fatalf("busca: %q", got)
	}
}
