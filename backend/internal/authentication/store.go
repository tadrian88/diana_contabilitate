package authentication

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user disabled")
	ErrAlreadyExists      = errors.New("user already exists")
	ErrNotFound           = errors.New("user not found")
)

type User struct {
	ID, Email, Status, Persona string
	PasswordHash               string
	AllClients                 bool
	AuthorizedClientIDs        []string
}

type Session struct {
	ID        string
	User      User
	CSRFHash  []byte
	ExpiresAt time.Time
}

type Store interface {
	UserByEmail(context.Context, string) (User, error)
	CreateSession(context.Context, string, []byte, []byte, time.Time, time.Time) (string, error)
	SessionByTokenHash(context.Context, []byte, time.Time) (Session, error)
	RevokeSession(context.Context, []byte, time.Time) error
	RecordEvent(context.Context, string, *string, time.Time) error
}

type SQLStore struct{ DB *sql.DB }

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (s SQLStore) UserByEmail(ctx context.Context, normalized string) (User, error) {
	var user User
	err := s.DB.QueryRowContext(ctx, `SELECT id,email,password_hash,status,persona,all_clients FROM auth_users WHERE email=$1`, normalized).
		Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Status, &user.Persona, &user.AllClients)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT client_id FROM auth_user_client_grants WHERE user_id=$1 ORDER BY client_id`, user.ID)
	if err != nil {
		return User{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return User{}, err
		}
		user.AuthorizedClientIDs = append(user.AuthorizedClientIDs, id)
	}
	return user, rows.Err()
}

func (s SQLStore) CreateSession(ctx context.Context, userID string, tokenHash, csrfHash []byte, now, expires time.Time) (string, error) {
	id, err := randomID("session-")
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO auth_sessions(id,user_id,token_hash,csrf_hash,created_at,last_seen_at,expires_at) VALUES($1,$2,$3,$4,$5,$5,$6)`, id, userID, tokenHash, csrfHash, now, expires)
	return id, err
}

func (s SQLStore) SessionByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) (Session, error) {
	var session Session
	err := s.DB.QueryRowContext(ctx, `SELECT s.id,s.csrf_hash,s.expires_at,u.id,u.email,u.status,u.persona,u.all_clients
		FROM auth_sessions s JOIN auth_users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>$2`, tokenHash, now).
		Scan(&session.ID, &session.CSRFHash, &session.ExpiresAt, &session.User.ID, &session.User.Email, &session.User.Status, &session.User.Persona, &session.User.AllClients)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}
	if session.User.Status != "ACTIVE" {
		return Session{}, ErrUserDisabled
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT client_id FROM auth_user_client_grants WHERE user_id=$1 ORDER BY client_id`, session.User.ID)
	if err != nil {
		return Session{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return Session{}, err
		}
		session.User.AuthorizedClientIDs = append(session.User.AuthorizedClientIDs, id)
	}
	if err := rows.Err(); err != nil {
		return Session{}, err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE auth_sessions SET last_seen_at=$2 WHERE id=$1`, session.ID, now)
	return session, nil
}

func (s SQLStore) RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=$2 WHERE token_hash=$1 AND revoked_at IS NULL`, tokenHash, now)
	return err
}

func (s SQLStore) RecordEvent(ctx context.Context, event string, userID *string, now time.Time) error {
	id, err := randomID("auth-event-")
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO auth_events(id,user_id,event_type,occurred_at) VALUES($1,$2,$3,$4)`, id, userID, event, now)
	return err
}

type ProvisionResult string

const (
	ProvisionCreated ProvisionResult = "created"
	ProvisionExists  ProvisionResult = "already exists"
	PasswordReset    ProvisionResult = "password reset"
)

func (s SQLStore) Provision(ctx context.Context, email, passwordHash string, allClients bool, now time.Time) (ProvisionResult, error) {
	email = NormalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return "", errors.New("valid email is required")
	}
	id, err := randomID("user-")
	if err != nil {
		return "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var createdID string
	err = tx.QueryRowContext(ctx, `INSERT INTO auth_users(id,email,password_hash,status,persona,all_clients,created_at,updated_at) VALUES($1,$2,$3,'ACTIVE','CONTABIL',$4,$5,$5) ON CONFLICT(email) DO NOTHING RETURNING id`, id, email, passwordHash, allClients, now).Scan(&createdID)
	if errors.Is(err, sql.ErrNoRows) {
		return ProvisionExists, ErrAlreadyExists
	}
	if err != nil {
		return "", err
	}
	if allClients {
		eventID, eventErr := randomID("auth-event-")
		if eventErr != nil {
			return "", eventErr
		}
		if _, eventErr = tx.ExecContext(ctx, `INSERT INTO auth_events(id,user_id,event_type,occurred_at) VALUES($1,$2,'GRANT_CHANGED',$3)`, eventID, createdID, now); eventErr != nil {
			return "", eventErr
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return ProvisionCreated, nil
}

func (s SQLStore) ResetPassword(ctx context.Context, email, passwordHash string, now time.Time) (ProvisionResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var userID string
	err = tx.QueryRowContext(ctx, `UPDATE auth_users SET password_hash=$2,updated_at=$3,credential_version=credential_version+1 WHERE email=$1 RETURNING id`, NormalizeEmail(email), passwordHash, now).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, userID, now); err != nil {
		return "", err
	}
	eventID, err := randomID("auth-event-")
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_events(id,user_id,event_type,occurred_at) VALUES($1,$2,'PASSWORD_CHANGED',$3)`, eventID, userID, now); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return PasswordReset, nil
}

func TokenHash(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func randomID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate identifier: %w", err)
	}
	return prefix + hex.EncodeToString(value), nil
}
