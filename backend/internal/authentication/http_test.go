package authentication

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/platform/requestactor"
)

type memoryStore struct {
	user             User
	session          Session
	createdTokenHash []byte
	revoked          bool
	events           []string
}

func (s *memoryStore) UserByEmail(_ context.Context, email string) (User, error) {
	if email != s.user.Email {
		return User{}, ErrInvalidCredentials
	}
	return s.user, nil
}
func (s *memoryStore) CreateSession(_ context.Context, userID string, tokenHash, csrfHash []byte, _, expires time.Time) (string, error) {
	s.createdTokenHash = append([]byte(nil), tokenHash...)
	s.session = Session{ID: "session-1", User: s.user, CSRFHash: append([]byte(nil), csrfHash...), ExpiresAt: expires}
	return s.session.ID, nil
}
func (s *memoryStore) SessionByTokenHash(_ context.Context, tokenHash []byte, now time.Time) (Session, error) {
	if s.session.ID == "" || s.revoked || !s.session.ExpiresAt.After(now) || !bytes.Equal(tokenHash, s.createdTokenHash) {
		return Session{}, ErrInvalidCredentials
	}
	return s.session, nil
}
func (s *memoryStore) RevokeSession(context.Context, []byte, time.Time) error {
	s.revoked = true
	return nil
}
func (s *memoryStore) RecordEvent(_ context.Context, event string, _ *string, _ time.Time) error {
	s.events = append(s.events, event)
	return nil
}

func newTestHTTP(t *testing.T, store *memoryStore) *HTTP {
	t.Helper()
	h := NewHTTP(store, nil, Config{FrontendURL: "http://example.test", SessionTTL: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.allowLoginForTest = func(context.Context, string) bool { return true }
	return h
}

func TestLoginSessionRequestActorAndLogout(t *testing.T) {
	hash, err := HashPassword("Valid-Test-Password-2026!")
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{user: User{ID: "user-1", Email: "demo@accountingtechco.com", PasswordHash: hash, Status: "ACTIVE", Persona: "CONTABIL", AuthorizedClientIDs: []string{"client-a"}}}
	h := newTestHTTP(t, store)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := requestactor.FromContext(r.Context())
		if !ok || actor.ID != "user-1" || actor.AllowsClient("client-b") || !actor.AllowsClient("client-a") {
			t.Fatalf("unexpected actor: %+v", actor)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := h.Wrap(next)

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":" Demo@AccountingTechCo.com ","password":"Valid-Test-Password-2026!"}`))
	login.Header.Set("Origin", "http://example.test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, login)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "demo@accountingtechco.com") {
		t.Fatalf("login: %d %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	var sessionCookie, csrfCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == SessionCookie {
			sessionCookie = cookie
		}
		if cookie.Name == CSRFCookie {
			csrfCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || csrfCookie == nil || csrfCookie.HttpOnly {
		t.Fatal("cookie security attributes incorrect")
	}

	protected := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	protected.AddCookie(sessionCookie)
	protectedResponse := httptest.NewRecorder()
	handler.ServeHTTP(protectedResponse, protected)
	if protectedResponse.Code != http.StatusOK {
		t.Fatalf("protected status=%d", protectedResponse.Code)
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.Header.Set("Origin", "http://example.test")
	logout.Header.Set("X-CSRF-Token", csrfCookie.Value)
	logout.AddCookie(sessionCookie)
	logout.AddCookie(csrfCookie)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent || !store.revoked {
		t.Fatalf("logout status=%d revoked=%v", logoutResponse.Code, store.revoked)
	}
}

func TestLoginFailuresAreGenericAndDisabledUserCannotLogin(t *testing.T) {
	hash, _ := HashPassword("Valid-Test-Password-2026!")
	for _, test := range []struct{ name, email, password, status string }{
		{"wrong password", "demo@accountingtechco.com", "Wrong-Test-Password!", "ACTIVE"},
		{"unknown email", "unknown@example.com", "Wrong-Test-Password!", "ACTIVE"},
		{"disabled", "demo@accountingtechco.com", "Valid-Test-Password-2026!", "DISABLED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryStore{user: User{ID: "user-1", Email: "demo@accountingtechco.com", PasswordHash: hash, Status: test.status}}
			h := newTestHTTP(t, store)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"`+test.email+`","password":"`+test.password+`"}`))
			response := httptest.NewRecorder()
			h.Wrap(http.NotFoundHandler()).ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "Email sau parolă incorectă") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAnonymousSensitiveAPIsAreUnauthorizedAndCallbackIsPublic(t *testing.T) {
	h := newTestHTTP(t, &memoryStore{})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := h.Wrap(next)
	for _, path := range []string{"/api/v1/clients", "/api/v1/invoices", "/api/v1/accounts", "/api/v1/contracts", "/api/v1/rules", "/api/v1/validation-tasks", "/api/v1/clients/a/spv", "/api/v1/clients/a/invoices/i/saga-export/artifact"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", path, response.Code)
		}
	}
	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/api/v1/integrations/anaf/callback?state=x&code=y", nil))
	if callback.Code != http.StatusNoContent {
		t.Fatalf("callback status=%d", callback.Code)
	}
}

func TestExpiredSessionAndCSRFFailure(t *testing.T) {
	store := &memoryStore{session: Session{ID: "expired", ExpiresAt: time.Now().Add(-time.Minute)}, createdTokenHash: TokenHash("token")}
	h := newTestHTTP(t, store)
	handler := h.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Fatal("request reached protected handler") }))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clients", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: "token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestLoginRateLimitStopsCredentialVerification(t *testing.T) {
	store := &memoryStore{}
	h := newTestHTTP(t, store)
	h.allowLoginForTest = func(context.Context, string) bool { return false }
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"demo@example.com","password":"Password-Only-For-Test!"}`))
	response := httptest.NewRecorder()
	h.Wrap(http.NotFoundHandler()).ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || len(store.events) != 0 {
		t.Fatalf("status=%d events=%v", response.Code, store.events)
	}
}
