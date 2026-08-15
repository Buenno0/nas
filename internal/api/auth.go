package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"nas/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	Username           string `json:"username"`
	MustChangePassword bool   `json:"must_change_password"`
	IsAdmin            bool   `json:"is_admin"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}

	ip := auth.ClientIP(r, s.opts.TrustProxy)
	token, user, err := s.auth.Login(r.Context(), ip, req.Username, req.Password, r.UserAgent())
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			status = http.StatusTooManyRequests
		}
		writeError(w, status, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(auth.SessionTTL),
	})
	writeJSON(w, http.StatusOK, userResponse{Username: user.Username, MustChangePassword: user.MustChangePassword, IsAdmin: user.IsAdmin})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.CookieName); err == nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			writeError(w, http.StatusInternalServerError, "não foi possível encerrar a sessão")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, userResponse{Username: user.Username, MustChangePassword: user.MustChangePassword, IsAdmin: user.IsAdmin})
}

type passwordRequest struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())

	var req passwordRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requisição inválida")
		return
	}
	if err := s.auth.ChangePassword(r.Context(), user.ID, req.Current, req.New); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Todas as sessões caíram, inclusive esta: o cliente precisa entrar de novo.
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
