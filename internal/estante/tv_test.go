package estante

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const descricaoDaSala = `<?xml version="1.0" encoding="utf-8"?><root xmlns="urn:schemas-upnp-org:device-1-0">
<device><deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
<friendlyName>KDL-32W655D</friendlyName><manufacturer>Sony Corporation</manufacturer>
<modelName>KDL-32W700B</modelName><UDN>uuid:13c1cbc0</UDN><serviceList>
<service><serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType><controlURL>control/RenderingControl</controlURL></service>
<service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>control/AVTransport</controlURL></service>
</serviceList></device></root>`

func resposta(acao, servico, corpo string) string {
	return `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:` + acao +
		`Response xmlns:u="urn:schemas-upnp-org:service:` + servico + `:1">` + corpo + `</u:` + acao + `Response></s:Body></s:Envelope>`
}

// tvFalsa imita a KDL-32W655D: descrição em /dmr.xml, controle relativo.
func tvFalsa(t *testing.T) *httptest.Server {
	t.Helper()
	volume, mudo := 2, false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dmr.xml" {
			_, _ = io.WriteString(w, descricaoDaSala)
			return
		}
		corpo, _ := io.ReadAll(r.Body)
		acao := r.Header.Get("SOAPACTION")
		switch {
		case r.URL.Path == "/control/RenderingControl" && strings.HasSuffix(acao, `#GetVolume"`):
			_, _ = io.WriteString(w, resposta("GetVolume", "RenderingControl", "<CurrentVolume>"+strconv.Itoa(volume)+"</CurrentVolume>"))
		case r.URL.Path == "/control/RenderingControl" && strings.HasSuffix(acao, `#SetVolume"`):
			// A ordem importa para a TV: InstanceID, Channel, DesiredVolume.
			s := string(corpo)
			i, c, d := strings.Index(s, "<InstanceID>"), strings.Index(s, "<Channel>"), strings.Index(s, "<DesiredVolume>")
			if !(i < c && c < d) {
				t.Errorf("argumentos fora de ordem: %s", s)
			}
			v, err := strconv.Atoi(s[d+len("<DesiredVolume>") : strings.Index(s, "</DesiredVolume>")])
			if err != nil {
				t.Errorf("DesiredVolume: %v", err)
			}
			volume = v
			_, _ = io.WriteString(w, resposta("SetVolume", "RenderingControl", ""))
		case strings.HasSuffix(acao, `#GetMute"`):
			_, _ = io.WriteString(w, resposta("GetMute", "RenderingControl", "<CurrentMute>"+map[bool]string{true: "1", false: "0"}[mudo]+"</CurrentMute>"))
		case strings.HasSuffix(acao, `#SetMute"`):
			mudo = strings.Contains(string(corpo), "<DesiredMute>1<")
			_, _ = io.WriteString(w, resposta("SetMute", "RenderingControl", ""))
		case r.URL.Path == "/control/AVTransport":
			_, _ = io.WriteString(w, resposta("GetTransportInfo", "AVTransport", "<CurrentTransportState>NO_MEDIA_PRESENT</CurrentTransportState>"))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode>`+
				`<detail><UPnPError><errorCode>401</errorCode><errorDescription>Invalid Action</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
		}
	}))
}

func TestTV(t *testing.T) {
	srv := tvFalsa(t)
	defer srv.Close()
	ctx := context.Background()

	tv, err := AbrirTV(ctx, srv.URL+"/dmr.xml", nil)
	if err != nil {
		t.Fatal(err)
	}
	if tv.Nome != "KDL-32W655D" || tv.controle[svcVolume] != srv.URL+"/control/RenderingControl" {
		t.Fatalf("descrição lida errado: %+v", tv)
	}
	if v, err := tv.Volume(ctx); err != nil || v != 2 {
		t.Fatalf("volume: %d %v", v, err)
	}
	if err := tv.DefinirVolume(ctx, 30); err != nil {
		t.Fatal(err)
	}
	if v, _ := tv.Volume(ctx); v != 30 {
		t.Fatalf("volume depois do Set: %d", v)
	}
	if s, err := tv.Transporte(ctx); err != nil || s != "NO_MEDIA_PRESENT" {
		t.Fatalf("transporte: %q %v", s, err)
	}
	if err := tv.DefinirMudo(ctx, true); err != nil {
		t.Fatal(err)
	}
	if m, err := tv.Mudo(ctx); err != nil || !m {
		t.Fatalf("mudo: %v %v", m, err)
	}
	var e ErroUPnP
	if _, err := tv.acao(ctx, svcVolume, "ListPresets"); !errors.As(err, &e) || e.Codigo != 401 {
		t.Fatalf("falha SOAP devia virar ErroUPnP, veio %v", err)
	}
}

func TestLocalDoSSDP(t *testing.T) {
	r := "HTTP/1.1 200 OK\r\nLOCATION: http://192.168.15.6:2870/dmr.xml\r\nSERVER: IPI/1.0 UPnP/1.0 DLNADOC/1.50\r\n\r\n"
	if l := localDoSSDP([]byte(r)); l != "http://192.168.15.6:2870/dmr.xml" {
		t.Fatalf("location: %q", l)
	}
}
