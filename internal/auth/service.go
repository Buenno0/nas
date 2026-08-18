package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"nas/internal/db"
)

const (
	// CookieName é o nome do cookie de sessão.
	CookieName = "nas_session"
	// SessionTTL é quanto tempo dura um login com "manter conectado".
	SessionTTL = 30 * 24 * time.Hour
	// ShortSessionTTL é o login de uma sentada: sem "manter conectado", a
	// sessão morre no fim do dia e o cookie some quando o navegador fecha.
	ShortSessionTTL = 12 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("usuário ou senha incorretos")
	ErrNoSession          = errors.New("sessão inválida ou expirada")
)

type Service struct {
	db      *db.DB
	limiter *Limiter
}

func NewService(database *db.DB) *Service {
	return &Service{db: database, limiter: NewLimiter()}
}

// EnsureInitialUser cria o primeiro usuário se o banco estiver vazio e devolve
// a senha gerada, para o chamador mostrar no terminal uma única vez.
func (s *Service) EnsureInitialUser(ctx context.Context) (username, password string, created bool, err error) {
	n, err := s.db.CountUsers(ctx)
	if err != nil {
		return "", "", false, err
	}
	if n > 0 {
		return "", "", false, nil
	}

	username = defaultUsername()
	password, err = GeneratePassword(16)
	if err != nil {
		return "", "", false, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", "", false, err
	}
	// Quem instalou o servidor é o administrador.
	if _, err := s.db.CreateUser(ctx, username, hash, true, true); err != nil {
		return "", "", false, err
	}
	return username, password, true, nil
}

func defaultUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return strings.ToLower(u.Username)
	}
	if n := os.Getenv("USER"); n != "" {
		return strings.ToLower(n)
	}
	return "admin"
}

// CreateUser adiciona um usuário com senha definida.
func (s *Service) CreateUser(ctx context.Context, username, password string, isAdmin bool) error {
	if len(password) < 8 {
		return errors.New("a senha precisa de pelo menos 8 caracteres")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.CreateUser(ctx, username, hash, false, isAdmin)
	return err
}

// Login valida as credenciais e devolve o token de sessão em claro — é a única
// vez que ele existe fora do navegador. remember escolhe entre a sessão longa
// e a de uma sentada; o chamador usa o TTL devolvido para decidir se o cookie
// expira em data marcada ou quando o navegador fecha.
func (s *Service) Login(ctx context.Context, clientIP, username, password, userAgent string, remember bool) (token string, u db.User, ttl time.Duration, err error) {
	if wait, blocked := s.limiter.Blocked(clientIP); blocked {
		return "", db.User{}, 0, fmt.Errorf("muitas tentativas: tente de novo em %s", wait.Round(time.Second))
	}

	u, err = s.db.UserByName(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			// Gasta o mesmo tempo de um hash real para não revelar, pelo
			// tempo de resposta, se o usuário existe.
			_, _ = HashPassword(password)
			s.limiter.Fail(clientIP)
			return "", db.User{}, 0, ErrInvalidCredentials
		}
		return "", db.User{}, 0, err
	}

	ok, err := VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		s.limiter.Fail(clientIP)
		return "", db.User{}, 0, ErrInvalidCredentials
	}
	s.limiter.Reset(clientIP)

	token, err = newToken()
	if err != nil {
		return "", db.User{}, 0, err
	}

	ttl = ShortSessionTTL
	if remember {
		ttl = SessionTTL
	}
	if err := s.db.CreateSession(ctx, hashToken(token), u.ID, userAgent, time.Now().Add(ttl)); err != nil {
		return "", db.User{}, 0, err
	}
	return token, u, ttl, nil
}

// UserFromToken resolve o cookie de sessão.
func (s *Service) UserFromToken(ctx context.Context, token string) (db.User, error) {
	if token == "" {
		return db.User{}, ErrNoSession
	}
	u, err := s.db.UserBySession(ctx, hashToken(token))
	if errors.Is(err, db.ErrNotFound) {
		return db.User{}, ErrNoSession
	}
	return u, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.db.DeleteSession(ctx, hashToken(token))
}

// ChangePassword troca a senha e derruba as outras sessões do usuário.
func (s *Service) ChangePassword(ctx context.Context, userID int64, current, next string) error {
	if len(next) < 8 {
		return errors.New("a nova senha precisa de pelo menos 8 caracteres")
	}
	u, err := s.db.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(u.PasswordHash, current)
	if err != nil || !ok {
		return errors.New("senha atual incorreta")
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.db.SetPassword(ctx, userID, hash); err != nil {
		return err
	}
	return s.db.DeleteUserSessions(ctx, userID)
}

// CleanupExpired roda periodicamente enquanto o servidor está no ar.
func (s *Service) CleanupExpired(ctx context.Context) (int64, error) {
	return s.db.DeleteExpiredSessions(ctx)
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("gerando token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
