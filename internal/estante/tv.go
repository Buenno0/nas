package estante

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TV é um renderizador DLNA (UPnP MediaRenderer). A Bravia da sala
// (KDL-32W655D, 2016) não tem a API REST das Bravias com Android, mas expõe
// volume e mudo pelo RenderingControl e reprodução pelo AVTransport, sem
// senha. Ela só aparece na rede enquanto está ligada.
type TV struct {
	Nome       string `json:"nome"`
	Fabricante string `json:"fabricante"`
	Modelo     string `json:"modelo"`
	UDN        string `json:"udn"`
	Descricao  string `json:"descricao"` // URL do XML de descrição (o LOCATION do SSDP)

	controle map[string]string // tipo de serviço curto → URL absoluta de controle
	HTTP     *http.Client      `json:"-"`
}

// httpCurto é o cliente da Central: a TV desligada não responde nada, e a
// sondagem não pode ficar presa esperando por ela.
var httpCurto = http.Client{Timeout: 2 * time.Second}

const (
	svcVolume     = "RenderingControl"
	svcTransporte = "AVTransport"
)

type descricaoUPnP struct {
	Device struct {
		Nome       string `xml:"friendlyName"`
		Fabricante string `xml:"manufacturer"`
		Modelo     string `xml:"modelName"`
		UDN        string `xml:"UDN"`
		Servicos   []struct {
			Tipo     string `xml:"serviceType"`
			Controle string `xml:"controlURL"`
		} `xml:"serviceList>service"`
	} `xml:"device"`
}

func (t *TV) cliente() *http.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	return &http.Client{Timeout: 3 * time.Second}
}

// AbrirTV lê a descrição UPnP em local e monta a TV. Os controlURL vêm
// relativos ("control/RenderingControl"), resolvidos contra o próprio local.
func AbrirTV(ctx context.Context, local string, cli *http.Client) (*TV, error) {
	base, err := url.Parse(local)
	if err != nil {
		return nil, err
	}
	t := &TV{Descricao: local, HTTP: cli, controle: map[string]string{}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, local, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.cliente().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d descricaoUPnP
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return nil, fmt.Errorf("descrição UPnP de %s: %w", local, err)
	}
	t.Nome, t.Fabricante, t.Modelo, t.UDN = d.Device.Nome, d.Device.Fabricante, d.Device.Modelo, d.Device.UDN
	for _, s := range d.Device.Servicos {
		ref, err := url.Parse(strings.TrimSpace(s.Controle))
		if err != nil {
			continue
		}
		// "urn:schemas-upnp-org:service:RenderingControl:1" → "RenderingControl"
		partes := strings.Split(s.Tipo, ":")
		if len(partes) >= 2 {
			t.controle[partes[len(partes)-2]] = base.ResolveReference(ref).String()
		}
	}
	if t.controle[svcVolume] == "" {
		return nil, fmt.Errorf("%s não tem RenderingControl", t.Nome)
	}
	return t, nil
}

// ErroUPnP é a falha SOAP que o aparelho devolve (402 = argumento inválido,
// 501 = ação falhou).
type ErroUPnP struct {
	Codigo    int
	Descricao string
}

func (e ErroUPnP) Error() string { return fmt.Sprintf("UPnP %d: %s", e.Codigo, e.Descricao) }

// acao chama uma ação SOAP e devolve os campos da resposta. args vai na ordem
// dada: alguns aparelhos recusam argumentos fora da ordem do SCPD.
func (t *TV) acao(ctx context.Context, servico, nome string, args ...[2]string) (map[string]string, error) {
	alvo := t.controle[servico]
	if alvo == "" {
		return nil, fmt.Errorf("a TV não tem %s", servico)
	}
	urn := "urn:schemas-upnp-org:service:" + servico + ":1"
	var corpo bytes.Buffer
	corpo.WriteString(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&corpo, `<u:%s xmlns:u="%s"><InstanceID>0</InstanceID>`, nome, urn)
	for _, a := range args {
		corpo.WriteString("<" + a[0] + ">")
		_ = xml.EscapeText(&corpo, []byte(a[1]))
		corpo.WriteString("</" + a[0] + ">")
	}
	fmt.Fprintf(&corpo, `</u:%s></s:Body></s:Envelope>`, nome)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, alvo, &corpo)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"`+urn+"#"+nome+`"`)
	resp, err := t.cliente().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return lerSOAP(io.LimitReader(resp.Body, 1<<20))
}

// lerSOAP achata a resposta: cada elemento folha vira chave → texto. A falha
// SOAP vira ErroUPnP.
func lerSOAP(r io.Reader) (map[string]string, error) {
	dec := xml.NewDecoder(r)
	campos := map[string]string{}
	var atual string
	var texto strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("resposta SOAP inválida: %w", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			atual = v.Name.Local
			texto.Reset()
		case xml.CharData:
			texto.Write(v)
		case xml.EndElement:
			if v.Name.Local == atual {
				campos[atual] = strings.TrimSpace(texto.String())
			}
			atual = ""
		}
	}
	if _, falhou := campos["faultcode"]; falhou {
		cod, _ := strconv.Atoi(campos["errorCode"])
		return nil, ErroUPnP{Codigo: cod, Descricao: campos["errorDescription"]}
	}
	return campos, nil
}

func (t *TV) Volume(ctx context.Context) (int, error) {
	r, err := t.acao(ctx, svcVolume, "GetVolume", [2]string{"Channel", "Master"})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(r["CurrentVolume"])
}

func (t *TV) DefinirVolume(ctx context.Context, v int) error {
	_, err := t.acao(ctx, svcVolume, "SetVolume", [2]string{"Channel", "Master"}, [2]string{"DesiredVolume", strconv.Itoa(v)})
	return err
}

func (t *TV) Mudo(ctx context.Context) (bool, error) {
	r, err := t.acao(ctx, svcVolume, "GetMute", [2]string{"Channel", "Master"})
	if err != nil {
		return false, err
	}
	return r["CurrentMute"] == "1" || r["CurrentMute"] == "true", nil
}

func (t *TV) DefinirMudo(ctx context.Context, mudo bool) error {
	v := "0"
	if mudo {
		v = "1"
	}
	_, err := t.acao(ctx, svcVolume, "SetMute", [2]string{"Channel", "Master"}, [2]string{"DesiredMute", v})
	return err
}

// Transporte é o estado do AVTransport: NO_MEDIA_PRESENT quando a TV está na
// antena/HDMI, PLAYING/PAUSED_PLAYBACK quando toca algo mandado por DLNA.
func (t *TV) Transporte(ctx context.Context) (string, error) {
	r, err := t.acao(ctx, svcTransporte, "GetTransportInfo")
	if err != nil {
		return "", err
	}
	return r["CurrentTransportState"], nil
}

// ProcurarTVs faz um M-SEARCH SSDP por MediaRenderer e abre cada um que
// responder. Aparelhos que não são TV (caixas de som DLNA) também aparecem;
// quem chama filtra pelo fabricante se quiser.
func ProcurarTVs(ctx context.Context, espera time.Duration) ([]*TV, error) {
	locais, err := buscarSSDP(ctx, "urn:schemas-upnp-org:device:MediaRenderer:1", espera)
	if err != nil {
		return nil, err
	}
	var tvs []*TV
	for _, l := range locais {
		t, err := AbrirTV(ctx, l, nil)
		if err != nil {
			continue
		}
		tvs = append(tvs, t)
	}
	return tvs, nil
}

// buscarSSDP devolve os LOCATION distintos que responderam ao M-SEARCH.
func buscarSSDP(ctx context.Context, st string, espera time.Duration) ([]string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	// MX é o atraso máximo que o aparelho sorteia antes de responder. A TV da
	// sala responde em milissegundos ou não responde: multicast no Wi-Fi se
	// perde. Por isso a busca sai três vezes, como a especificação do UPnP
	// sugere.
	busca := []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + st + "\r\n\r\n")
	grupo := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	for i := range 3 {
		if _, err := conn.WriteToUDP(busca, grupo); err != nil && i == 0 {
			return nil, fmt.Errorf("enviando M-SEARCH: %w", err)
		}
		if i < 2 {
			time.Sleep(150 * time.Millisecond)
		}
	}
	fim := time.Now().Add(espera)
	if d, ok := ctx.Deadline(); ok && d.Before(fim) {
		fim = d
	}
	_ = conn.SetReadDeadline(fim)

	vistos := map[string]bool{}
	var locais []string
	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return locais, nil
			}
			return locais, err
		}
		if l := localDoSSDP(buf[:n]); l != "" && !vistos[l] {
			vistos[l] = true
			locais = append(locais, l)
		}
	}
}

func localDoSSDP(b []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		chave, valor, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.EqualFold(strings.TrimSpace(chave), "location") {
			return strings.TrimSpace(valor)
		}
	}
	return ""
}
