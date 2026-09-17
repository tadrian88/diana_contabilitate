//go:build integration

package authentication

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestIntegrationProvisionDuplicateResetAndPersistentSession(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := SQLStore{DB: db}
	ctx := context.Background()
	email := "authentication-v1-integration@accountingtechco.test"
	_, _ = db.ExecContext(ctx, `DELETE FROM auth_users WHERE email=$1`, email)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM auth_users WHERE email=$1`, email) })

	password := "Integration-Password-2026!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.Provision(ctx, "  "+strings.ToUpper(email)+" ", hash, false, time.Now()); err != nil || result != ProvisionCreated {
		t.Fatalf("result=%s err=%v", result, err)
	}
	if _, err := store.Provision(ctx, email, hash, false, time.Now()); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate err=%v", err)
	}
	var persisted string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM auth_users WHERE email=$1`, email).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted == password || strings.Contains(persisted, password) {
		t.Fatal("plaintext password persisted")
	}

	user, err := store.UserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := store.CreateSession(ctx, user.ID, TokenHash("session-secret"), TokenHash("csrf-secret"), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SessionByTokenHash(ctx, TokenHash("session-secret"), now); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeSession(ctx, TokenHash("session-secret"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SessionByTokenHash(ctx, TokenHash("session-secret"), now); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked session err=%v", err)
	}

	newHash, _ := HashPassword("Replacement-Password-2026!")
	if result, err := store.ResetPassword(ctx, email, newHash, now); err != nil || result != PasswordReset {
		t.Fatalf("result=%s err=%v", result, err)
	}
}
