package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestPareamentoDaTVEmiteSessaoUmaUnicaVez(t *testing.T) {
	srv, _, comum := prepara(t)
	start := chama(t, srv, http.MethodPost, "/api/auth/device/start", "",
		`{"device_name":"Fire TV da sala","client":"firetv","version":"1.0"}`)
	if start.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", start.Code, start.Body.String())
	}
	var pairing deviceStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &pairing); err != nil {
		t.Fatal(err)
	}
	if pairing.DeviceCode == "" || pairing.UserCode == "" || pairing.ExpiresIn != 600 {
		t.Fatalf("resposta incompleta: %+v", pairing)
	}

	pending := chama(t, srv, http.MethodPost, "/api/auth/device/token", "",
		`{"device_code":"`+pairing.DeviceCode+`"}`)
	if pending.Code != http.StatusAccepted {
		t.Fatalf("token pendente = %d: %s", pending.Code, pending.Body.String())
	}

	approve := chama(t, srv, http.MethodPost, "/api/auth/device/approve", comum,
		`{"user_code":"`+pairing.UserCode+`"}`)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", approve.Code, approve.Body.String())
	}

	// O teste não espera dois segundos: simula o intervalo que a TV respeita.
	srv.devices.mu.Lock()
	srv.devices.byDevice[pairing.DeviceCode].LastPoll = srv.devices.byDevice[pairing.DeviceCode].LastPoll.Add(-devicePollInterval)
	srv.devices.mu.Unlock()

	token := chama(t, srv, http.MethodPost, "/api/auth/device/token", "",
		`{"device_code":"`+pairing.DeviceCode+`"}`)
	if token.Code != http.StatusOK {
		t.Fatalf("token aprovado = %d: %s", token.Code, token.Body.String())
	}
	var session userResponse
	if err := json.Unmarshal(token.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Username != "visita" || session.Token == "" || session.ExpiraEm == "" {
		t.Fatalf("sessão inválida: %+v", session)
	}
	if me := chamaBearer(t, srv, http.MethodGet, "/api/auth/me", session.Token, ""); me.Code != http.StatusOK {
		t.Fatalf("sessão emitida não abre a API: %d", me.Code)
	}

	again := chama(t, srv, http.MethodPost, "/api/auth/device/token", "",
		`{"device_code":"`+pairing.DeviceCode+`"}`)
	if again.Code != http.StatusGone {
		t.Fatalf("segundo consumo = %d, quero 410", again.Code)
	}
}

func TestPareamentoRespeitaIntervaloEExpira(t *testing.T) {
	srv, _, _ := prepara(t)
	start := chama(t, srv, http.MethodPost, "/api/auth/device/start", "",
		`{"device_name":"Fire TV","client":"firetv","version":"1.0"}`)
	var pairing deviceStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &pairing); err != nil {
		t.Fatal(err)
	}

	first := chama(t, srv, http.MethodPost, "/api/auth/device/token", "",
		`{"device_code":"`+pairing.DeviceCode+`"}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("primeiro polling = %d, quero 202", first.Code)
	}
	tooSoon := chama(t, srv, http.MethodPost, "/api/auth/device/token", "",
		`{"device_code":"`+pairing.DeviceCode+`"}`)
	if tooSoon.Code != http.StatusTooManyRequests {
		t.Fatalf("polling adiantado = %d, quero 429", tooSoon.Code)
	}

	srv.devices.mu.Lock()
	srv.devices.byDevice[pairing.DeviceCode].ExpiresAt = time.Now().Add(-time.Second)
	srv.devices.mu.Unlock()
	expired := chama(t, srv, http.MethodPost, "/api/auth/device/token", "",
		`{"device_code":"`+pairing.DeviceCode+`"}`)
	if expired.Code != http.StatusGone {
		t.Fatalf("pareamento expirado = %d, quero 410", expired.Code)
	}
}

func TestPareamentoLimitaCriacoesPorIP(t *testing.T) {
	srv, _, _ := prepara(t)
	for i := 0; i < deviceStartsPerMinute; i++ {
		if rec := chama(t, srv, http.MethodPost, "/api/auth/device/start", "", `{}`); rec.Code != http.StatusOK {
			t.Fatalf("criação %d = %d", i+1, rec.Code)
		}
	}
	blocked := chama(t, srv, http.MethodPost, "/api/auth/device/start", "", `{}`)
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("criação acima do limite = %d, quero 429", blocked.Code)
	}
}

func TestPareamentoRejeitaCodigoEDeviceCodeInventados(t *testing.T) {
	srv, _, comum := prepara(t)
	approve := chama(t, srv, http.MethodPost, "/api/auth/device/approve", comum,
		`{"user_code":"AAAA-BBBB"}`)
	if approve.Code != http.StatusNotFound {
		t.Fatalf("approve inventado = %d, quero 404", approve.Code)
	}
	token := chama(t, srv, http.MethodPost, "/api/auth/device/token", "",
		`{"device_code":"inventado"}`)
	if token.Code != http.StatusGone {
		t.Fatalf("device_code inventado = %d, quero 410", token.Code)
	}
}
