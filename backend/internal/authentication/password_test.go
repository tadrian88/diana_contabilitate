package authentication

import (
	"strings"
	"testing"
)

func TestArgon2idPasswordHashAndVerification(t *testing.T) {
	password := "Correct-Horse-2026!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, password) || !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unsafe hash format: %q", hash)
	}
	ok, err := VerifyPassword(hash, password)
	if err != nil || !ok {
		t.Fatalf("valid password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(hash, "incorrect-password")
	if err != nil || ok {
		t.Fatalf("invalid password accepted: ok=%v err=%v", ok, err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Demo@AccountingTechCo.COM "); got != "demo@accountingtechco.com" {
		t.Fatalf("got %q", got)
	}
}
