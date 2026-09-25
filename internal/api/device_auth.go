package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"nas/internal/auth"
)

const (
	devicePairingTTL      = 10 * time.Minute
	devicePollInterval    = 2 * time.Second
	devicePairingMax      = 100
	deviceStartsPerMinute = 5
)

type devicePairing struct {
	DeviceCode     string
	UserCode       string
	DeviceName     string
	UserAgent      string
	ExpiresAt      time.Time
	LastPoll       time.Time
	ApprovedUserID int64
}

type devicePairings struct {
	mu       sync.Mutex
	byDevice map[string]*devicePairing
	byUser   map[string]string
	starts   map[string][]time.Time
}

func newDevicePairings() *devicePairings {
	return &devicePairings{
		byDevice: make(map[string]*devicePairing),
		byUser:   make(map[string]string),
		starts:   make(map[string][]time.Time),
	}
}

type deviceStartRequest struct {
	DeviceName string `json:"device_name"`
	Client     string `json:"client"`
	Version    string `json:"version"`
}

type deviceStartResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type deviceCodeRequest struct {
	DeviceCode string `json:"device_code"`
}

type deviceApproveRequest struct {
	UserCode string `json:"user_code"`
}

func (s *Server) handleDeviceStart(w http.ResponseWriter, r *http.Request) {
	var req deviceStartRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}
	name := strings.TrimSpace(req.DeviceName)
	if name == "" {
		name = "Fire TV"
	}
	if len(name) > 64 {
		writeError(w, http.StatusBadRequest, "nome do dispositivo muito longo")
		return
	}

	now := time.Now()
	ip := auth.ClientIP(r, s.opts.TrustProxy)
	s.devices.mu.Lock()
	s.devices.cleanupLocked(now)
	if !s.devices.allowStartLocked(ip, now) {
		s.devices.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "muitas tentativas de pareamento")
		return
	}
	if len(s.devices.byDevice) >= devicePairingMax {
		s.devices.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "limite de pareamentos pendentes atingido")
		return
	}
	deviceCode, err := randomDeviceCode()
	if err != nil {
		s.devices.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "não foi possível iniciar o pareamento")
		return
	}
	userCode, err := s.devices.uniqueUserCodeLocked()
	if err != nil {
		s.devices.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "não foi possível iniciar o pareamento")
		return
	}
	userAgent := strings.TrimSpace(strings.Join([]string{req.Client, req.Version, name}, " "))
	pairing := &devicePairing{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		DeviceName: name,
		UserAgent:  userAgent,
		ExpiresAt:  now.Add(devicePairingTTL),
	}
	s.devices.byDevice[deviceCode] = pairing
	s.devices.byUser[userCode] = deviceCode
	s.devices.mu.Unlock()

	verification := s.deviceVerificationURL(r)
	complete := verification + "?codigo=" + url.QueryEscape(userCode)
	writeJSON(w, http.StatusOK, deviceStartResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verification,
		VerificationURIComplete: complete,
		ExpiresIn:               int(devicePairingTTL.Seconds()),
		Interval:                int(devicePollInterval.Seconds()),
	})
}

func (s *Server) handleDeviceApprove(w http.ResponseWriter, r *http.Request) {
	var req deviceApproveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}
	code := normalizeUserCode(req.UserCode)
	user, _ := auth.UserFrom(r.Context())

	now := time.Now()
	s.devices.mu.Lock()
	s.devices.cleanupLocked(now)
	deviceCode, ok := s.devices.byUser[code]
	pairing := s.devices.byDevice[deviceCode]
	if !ok || pairing == nil {
		s.devices.mu.Unlock()
		writeError(w, http.StatusNotFound, "código de pareamento inválido ou expirado")
		return
	}
	pairing.ApprovedUserID = user.ID
	deviceName := pairing.DeviceName
	s.devices.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "device_name": deviceName})
}

func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	var req deviceCodeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}
	now := time.Now()
	s.devices.mu.Lock()
	s.devices.cleanupLocked(now)
	pairing := s.devices.byDevice[req.DeviceCode]
	if pairing == nil {
		s.devices.mu.Unlock()
		writeError(w, http.StatusGone, "pareamento inválido, expirado ou já utilizado")
		return
	}
	if !pairing.LastPoll.IsZero() && now.Sub(pairing.LastPoll) < devicePollInterval {
		s.devices.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "aguarde antes de consultar novamente")
		return
	}
	pairing.LastPoll = now
	if pairing.ApprovedUserID == 0 {
		s.devices.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "pending", "interval": int(devicePollInterval.Seconds()),
		})
		return
	}
	userID := pairing.ApprovedUserID
	userAgent := pairing.UserAgent
	delete(s.devices.byDevice, pairing.DeviceCode)
	delete(s.devices.byUser, pairing.UserCode)
	s.devices.mu.Unlock()

	user, err := s.db.UserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "usuário do pareamento não existe mais")
		return
	}
	token, err := s.auth.IssueSession(r.Context(), userID, userAgent, auth.SessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "não foi possível criar a sessão da TV")
		return
	}
	writeJSON(w, http.StatusOK, userResponse{
		Username:           user.Username,
		MustChangePassword: user.MustChangePassword,
		IsAdmin:            user.IsAdmin,
		Token:              token,
		ExpiraEm:           now.Add(auth.SessionTTL).UTC().Format(time.RFC3339),
	})
}

func (s *Server) deviceVerificationURL(r *http.Request) string {
	scheme := "http"
	if s.opts.SecureCookies || r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/conectar"
}

func (d *devicePairings) cleanupLocked(now time.Time) {
	for code, pairing := range d.byDevice {
		if !now.Before(pairing.ExpiresAt) {
			delete(d.byDevice, code)
			delete(d.byUser, pairing.UserCode)
		}
	}
	for ip, starts := range d.starts {
		kept := starts[:0]
		for _, started := range starts {
			if now.Sub(started) < time.Minute {
				kept = append(kept, started)
			}
		}
		if len(kept) == 0 {
			delete(d.starts, ip)
		} else {
			d.starts[ip] = kept
		}
	}
}

func (d *devicePairings) allowStartLocked(ip string, now time.Time) bool {
	if len(d.starts[ip]) >= deviceStartsPerMinute {
		return false
	}
	d.starts[ip] = append(d.starts[ip], now)
	return true
}

func (d *devicePairings) uniqueUserCodeLocked() (string, error) {
	for range 20 {
		code, err := randomUserCode()
		if err != nil {
			return "", err
		}
		if _, exists := d.byUser[code]; !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("não foi possível gerar código único")
}

func randomDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomUserCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	random := make([]byte, len(b))
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return string(b[:4]) + "-" + string(b[4:]), nil
}

func normalizeUserCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "")
	if len(value) == 8 {
		return value[:4] + "-" + value[4:]
	}
	return value
}
