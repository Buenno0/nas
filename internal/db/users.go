package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID                 int64     `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	MustChangePassword bool      `json:"must_change_password"`
	IsAdmin            bool      `json:"is_admin"`
	CreatedAt          time.Time `json:"created_at"`
}

func (d *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (d *DB) CreateUser(ctx context.Context, username, passwordHash string, mustChange, isAdmin bool) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, errors.New("usuário não pode ser vazio")
	}
	now := time.Now()
	change := 0
	if mustChange {
		change = 1
	}
	admin := 0
	if isAdmin {
		admin = 1
	}
	res, err := d.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, must_change_password, is_admin, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		username, passwordHash, change, admin, now.Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, fmt.Errorf("usuário %q já existe", username)
		}
		return User{}, fmt.Errorf("criando usuário: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{
		ID:                 id,
		Username:           username,
		PasswordHash:       passwordHash,
		MustChangePassword: mustChange,
		IsAdmin:            isAdmin,
		CreatedAt:          now,
	}, nil
}

func (d *DB) UserByName(ctx context.Context, username string) (User, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, username, password_hash, must_change_password, is_admin, created_at
		   FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func (d *DB) UserByID(ctx context.Context, id int64) (User, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, username, password_hash, must_change_password, is_admin, created_at
		   FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (d *DB) SetPassword(ctx context.Context, userID int64, hash string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, must_change_password = 0 WHERE id = ?`, hash, userID)
	return err
}

func scanUser(s scanner) (User, error) {
	var (
		u         User
		change    int
		admin     int
		createdAt int64
	)
	err := s.Scan(&u.ID, &u.Username, &u.PasswordHash, &change, &admin, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.MustChangePassword = change != 0
	u.IsAdmin = admin != 0
	u.CreatedAt = time.Unix(createdAt, 0)
	return u, nil
}

// Users lista todas as contas, para `nas user ls`.
func (d *DB) Users(ctx context.Context) ([]User, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, username, password_hash, must_change_password, is_admin, created_at
		   FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listando usuários: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// DeleteUser remove uma conta. Recusa deixar o servidor sem nenhum admin —
// seria preciso mexer no SQLite na mão para voltar atrás.
func (d *DB) DeleteUser(ctx context.Context, username string) error {
	u, err := d.UserByName(ctx, username)
	if err != nil {
		return err
	}
	if u.IsAdmin {
		var admins int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return errors.New("este é o único administrador: promova outro antes de remover")
		}
	}
	_, err = d.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, u.ID)
	return err
}

// SetAdmin promove ou rebaixa uma conta.
func (d *DB) SetAdmin(ctx context.Context, username string, admin bool) error {
	u, err := d.UserByName(ctx, username)
	if err != nil {
		return err
	}
	if !admin {
		var admins int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&admins); err != nil {
			return err
		}
		if u.IsAdmin && admins <= 1 {
			return errors.New("este é o único administrador: promova outro antes de rebaixar")
		}
	}
	value := 0
	if admin {
		value = 1
	}
	_, err = d.ExecContext(ctx, `UPDATE users SET is_admin = ? WHERE id = ?`, value, u.ID)
	return err
}

// Sessões — o banco guarda o SHA-256 do token, nunca o token em si, para que
// uma cópia do nas.db não permita entrar como o usuário.

func (d *DB) CreateSession(ctx context.Context, tokenHash string, userID int64, userAgent string, expiresAt time.Time) error {
	_, err := d.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, user_agent, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		tokenHash, userID, userAgent, time.Now().Unix(), expiresAt.Unix())
	if err != nil {
		return fmt.Errorf("criando sessão: %w", err)
	}
	return nil
}

// UserBySession devolve o dono de uma sessão válida.
func (d *DB) UserBySession(ctx context.Context, tokenHash string) (User, error) {
	row := d.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash, u.must_change_password, u.is_admin, u.created_at
		  FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > ?`,
		tokenHash, time.Now().Unix())
	return scanUser(row)
}

func (d *DB) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, tokenHash)
	return err
}

// DeleteUserSessions encerra todas as sessões de um usuário (usado ao trocar
// a senha).
func (d *DB) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (d *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	res, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
