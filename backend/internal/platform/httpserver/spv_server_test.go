package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"diana-contabilitate/backend/internal/spv"
)

type httpSPVStore struct {
	state      spv.OAuthState
	connection *spv.Connection
}

func (*httpSPVStore) ClientCUI(_ context.Context, id string) (string, error) {
	if id != "client-a" {
		return "", apperrors.ErrNotFound
	}
	return "RO123", nil
}
func (s *httpSPVStore) ConnectionByClient(_ context.Context, id string) (*spv.Connection, error) {
	if id != "client-a" || s.connection == nil {
		return nil, apperrors.ErrNotFound
	}
	value := *s.connection
	return &value, nil
}
func (s *httpSPVStore) CreateOAuthState(_ context.Context, value spv.OAuthState) error {
	s.state = value
	return nil
}
func (s *httpSPVStore) ConsumeOAuthState(_ context.Context, hash string, now time.Time) (spv.OAuthState, error) {
	if s.state.StateHash != hash || s.state.ConsumedAt != nil || !s.state.ExpiresAt.After(now) {
		return spv.OAuthState{}, spv.ErrInvalidOAuthState
	}
	s.state.ConsumedAt = &now
	return s.state, nil
}
func (s *httpSPVStore) CompleteOAuthConnection(_ context.Context, state spv.OAuthState, access, refresh string, expires time.Time, refreshExpires *time.Time, _ spv.Actor, now time.Time) (spv.Connection, bool, error) {
	value := spv.Connection{ID: "connection", ClientID: state.ClientID, CIF: "RO123", Environment: state.Environment, AccessTokenCiphertext: access, RefreshTokenCiphertext: refresh, AccessTokenExpiresAt: expires, RefreshTokenExpiresAt: refreshExpires, Status: "ACTIVE", LastSyncStatus: "NEVER", ConnectedAt: &now}
	existed := s.connection != nil
	s.connection = &value
	return value, existed, nil
}
func (s *httpSPVStore) DisconnectSPVConnection(_ context.Context, _ string, _ string, _ spv.Actor, _ time.Time) (spv.Connection, bool, error) {
	if s.connection == nil {
		return spv.Connection{}, false, apperrors.ErrNotFound
	}
	s.connection.Status = "REVOKED"
	s.connection.AccessTokenCiphertext, s.connection.RefreshTokenCiphertext = "", ""
	return *s.connection, true, nil
}
func (*httpSPVStore) RecordManualSyncRequested(context.Context, spv.Connection, string, spv.Actor, time.Time) (bool, error) {
	return true, nil
}

type httpOAuth struct{}

func (httpOAuth) ExchangeToken(context.Context, string, string, string, string) (spv.TokenResponse, error) {
	return spv.TokenResponse{AccessToken: "raw-access", RefreshToken: "raw-refresh", ExpiresIn: time.Hour}, nil
}

type httpCipher struct{}

func (httpCipher) Encrypt(value string) (string, error) { return "encrypted:" + value, nil }
func (httpCipher) Decrypt(value string) (string, error) { return value, nil }

type httpPublisher struct{}

func (httpPublisher) PublishSPVSync(context.Context, string) (string, error) { return "job", nil }

func TestSPVHTTPReadOAuthCommandsAndCredentialBoundary(t *testing.T) {
	store := &httpSPVStore{}
	manager := spv.NewConnectionManager(store, httpOAuth{}, httpCipher{}, httpPublisher{}, spv.ConnectionManagerConfig{Environment: "TEST", AuthorizeURL: "https://anaf.example/authorize", OAuthClientID: "app", OAuthClientSecret: "secret", RedirectURI: "https://api.example/api/v1/integrations/anaf/callback", FrontendBaseURL: "https://diana.example", StateTTL: time.Minute, Enabled: true})
	handler := withTestActor(NewWithSPV(nil, nil, nil, nil, nil, nil, manager, readyStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics()), requestactor.Actor{ID: "test-accountant", Display: "Test", Persona: "CONTABIL", AllClients: true})
	call := func(method, target, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, target, nil)
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	read := call(http.MethodGet, "/api/v1/clients/client-a/spv", "")
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"status":"NOT_CONNECTED"`) {
		t.Fatalf("status=%d body=%s", read.Code, read.Body.String())
	}
	start := call(http.MethodPost, "/api/v1/clients/client-a/spv/oauth/start", "start")
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), "authorizationUrl") {
		t.Fatalf("status=%d body=%s", start.Code, start.Body.String())
	}
	now := time.Now().UTC()
	store.connection = &spv.Connection{ID: "connection", ClientID: "client-a", Environment: "TEST", Status: "ACTIVE", LastSyncStatus: "SUCCEEDED", AccessTokenCiphertext: "encrypted:raw-access", RefreshTokenCiphertext: "encrypted:raw-refresh", ConnectedAt: &now}
	connectedRead := call(http.MethodGet, "/api/v1/clients/client-a/spv", "")
	if connectedRead.Code != http.StatusOK || !strings.Contains(connectedRead.Body.String(), `"status":"CONNECTED"`) {
		t.Fatalf("status=%d body=%s", connectedRead.Code, connectedRead.Body.String())
	}
	// Manager security tests cover state hashing/replay; this transport assertion
	// ensures no credential-shaped fields can cross the connected read DTO.
	for _, forbidden := range []string{"accessToken", "refreshToken", "ciphertext", "raw-access", "raw-refresh"} {
		if strings.Contains(connectedRead.Body.String(), forbidden) {
			t.Fatalf("credential leaked: %s", connectedRead.Body.String())
		}
	}
}
