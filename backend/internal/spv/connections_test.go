package spv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
)

type connectionStoreStub struct {
	clients     map[string]string
	connections map[string]Connection
	states      map[string]OAuthState
	events      []string
}

func (s *connectionStoreStub) ClientCUI(_ context.Context, id string) (string, error) {
	value, ok := s.clients[id]
	if !ok {
		return "", apperrors.ErrNotFound
	}
	return value, nil
}
func (s *connectionStoreStub) ConnectionByClient(_ context.Context, id string) (*Connection, error) {
	value, ok := s.connections[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	return &value, nil
}
func (s *connectionStoreStub) CreateOAuthState(_ context.Context, value OAuthState) error {
	s.states[value.StateHash] = value
	return nil
}
func (s *connectionStoreStub) ConsumeOAuthState(_ context.Context, hash string, now time.Time) (OAuthState, error) {
	value, ok := s.states[hash]
	if !ok || value.ConsumedAt != nil || !value.ExpiresAt.After(now) {
		return OAuthState{}, ErrInvalidOAuthState
	}
	value.ConsumedAt = &now
	s.states[hash] = value
	return value, nil
}
func (s *connectionStoreStub) CompleteOAuthConnection(_ context.Context, state OAuthState, access, refresh string, accessExpiry time.Time, refreshExpiry *time.Time, _ Actor, now time.Time) (Connection, bool, error) {
	old, exists := s.connections[state.ClientID]
	value := Connection{ID: "connection-1", ClientID: state.ClientID, CIF: s.clients[state.ClientID], Environment: state.Environment, AccessTokenCiphertext: access, RefreshTokenCiphertext: refresh, AccessTokenExpiresAt: accessExpiry, RefreshTokenExpiresAt: refreshExpiry, Status: "ACTIVE", LastSyncStatus: "NEVER", ConnectedAt: &now}
	if exists {
		value.ID = old.ID
	}
	s.connections[state.ClientID] = value
	s.events = append(s.events, "connected")
	return value, exists, nil
}
func (s *connectionStoreStub) DisconnectSPVConnection(_ context.Context, clientID, _ string, _ Actor, _ time.Time) (Connection, bool, error) {
	value, ok := s.connections[clientID]
	if !ok {
		return Connection{}, false, apperrors.ErrNotFound
	}
	value.Status = "REVOKED"
	value.AccessTokenCiphertext, value.RefreshTokenCiphertext = "", ""
	s.connections[clientID] = value
	s.events = append(s.events, "disconnected")
	return value, true, nil
}
func (s *connectionStoreStub) RecordManualSyncRequested(_ context.Context, _ Connection, _ string, _ Actor, _ time.Time) (bool, error) {
	s.events = append(s.events, "sync")
	return true, nil
}

type oauthStub struct {
	calls int
	token TokenResponse
	err   error
}

func (o *oauthStub) ExchangeToken(context.Context, string, string, string, string) (TokenResponse, error) {
	o.calls++
	return o.token, o.err
}

type cipherStub struct{}

func (cipherStub) Encrypt(value string) (string, error) { return "encrypted:" + value, nil }
func (cipherStub) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "encrypted:"), nil
}

type publisherStub struct{ calls int }

func (p *publisherStub) PublishSPVSync(context.Context, string) (string, error) {
	p.calls++
	return "job", nil
}

func newConnectionManagerTest() (*ConnectionManager, *connectionStoreStub, *oauthStub, *publisherStub) {
	store := &connectionStoreStub{clients: map[string]string{"client-a": "RO123", "client-b": "RO456"}, connections: map[string]Connection{}, states: map[string]OAuthState{}}
	oauth := &oauthStub{token: TokenResponse{AccessToken: "access-secret", RefreshToken: "refresh-secret", ExpiresIn: time.Hour, RefreshExpiresIn: 2 * time.Hour}}
	publisher := &publisherStub{}
	manager := NewConnectionManager(store, oauth, cipherStub{}, publisher, ConnectionManagerConfig{Environment: "TEST", AuthorizeURL: "https://anaf.example/authorize", OAuthClientID: "app", OAuthClientSecret: "secret", RedirectURI: "https://api.example/api/v1/integrations/anaf/callback", FrontendBaseURL: "https://diana.example", StateTTL: 5 * time.Minute, Enabled: true})
	manager.clock = func() time.Time { return time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC) }
	return manager, store, oauth, publisher
}

func TestOAuthStartPersistsRandomHashedClientBoundState(t *testing.T) {
	manager, store, _, _ := newConnectionManagerTest()
	first, err := manager.StartOAuth(context.Background(), "client-a")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := manager.StartOAuth(context.Background(), "client-a")
	if first == second || strings.Contains(first, "RO123") || strings.Contains(first, "secret") {
		t.Fatalf("unsafe authorization URLs: %q %q", first, second)
	}
	if len(store.states) != 2 {
		t.Fatalf("states=%d", len(store.states))
	}
	for _, state := range store.states {
		if state.ClientID != "client-a" || state.Environment != "TEST" || state.ExpiresAt.Sub(state.CreatedAt) != 5*time.Minute || len(state.StateHash) != 64 {
			t.Fatalf("state=%+v", state)
		}
	}
}

func TestOAuthCallbackIsSingleUseAndStoresOnlyEncryptedTokens(t *testing.T) {
	manager, store, oauth, _ := newConnectionManagerTest()
	authorizationURL, _ := manager.StartOAuth(context.Background(), "client-a")
	raw := authorizationURL[strings.Index(authorizationURL, "state=")+6:]
	raw = strings.Split(raw, "&")[0]
	// State is hex, so URL decoding is not needed in this fixture.
	redirect, err := manager.Callback(context.Background(), raw, "code", "", Actor{ID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(redirect, "/clients/client-a") || !strings.Contains(redirect, "result=connected") {
		t.Fatalf("redirect=%s", redirect)
	}
	connection := store.connections["client-a"]
	if connection.AccessTokenCiphertext != "encrypted:access-secret" || connection.RefreshTokenCiphertext != "encrypted:refresh-secret" || strings.Contains(connection.AccessTokenCiphertext, "refresh-secret") {
		t.Fatalf("connection=%+v", connection)
	}
	if _, err = manager.Callback(context.Background(), raw, "code", "", Actor{}); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("replay err=%v", err)
	}
	if oauth.calls != 1 || len(store.events) != 1 {
		t.Fatalf("oauth=%d events=%v", oauth.calls, store.events)
	}
}

func TestOAuthExpiredAndUnknownStatesCannotStoreTokens(t *testing.T) {
	manager, store, oauth, _ := newConnectionManagerTest()
	raw := "unknown"
	digest := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(digest[:])
	store.states[hash] = OAuthState{StateHash: hash, ClientID: "client-b", Environment: "TEST", ReturnPath: "/clients/client-b", CreatedAt: manager.clock().Add(-time.Hour), ExpiresAt: manager.clock().Add(-time.Minute)}
	if _, err := manager.Callback(context.Background(), raw, "code", "", Actor{}); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("err=%v", err)
	}
	if oauth.calls != 0 || len(store.connections) != 0 {
		t.Fatalf("oauth=%d connections=%v", oauth.calls, store.connections)
	}
}

func TestCancelledAuthorizationConsumesStateWithoutTokenExchange(t *testing.T) {
	manager, store, oauth, _ := newConnectionManagerTest()
	authorizationURL, _ := manager.StartOAuth(context.Background(), "client-a")
	raw := authorizationURL[strings.Index(authorizationURL, "state=")+6:]
	raw = strings.Split(raw, "&")[0]
	redirect, err := manager.Callback(context.Background(), raw, "", "access_denied", Actor{})
	if err == nil || !strings.Contains(redirect, "authorization_cancelled") || oauth.calls != 0 || len(store.connections) != 0 {
		t.Fatalf("redirect=%s oauth=%d connections=%v err=%v", redirect, oauth.calls, store.connections, err)
	}
	if _, err = manager.Callback(context.Background(), raw, "code", "", Actor{}); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("cancelled state replay err=%v", err)
	}
}

func TestConnectionCommandsRespectHealthAndPreserveOneIdentity(t *testing.T) {
	manager, store, _, publisher := newConnectionManagerTest()
	now := manager.clock()
	store.connections["client-a"] = Connection{ID: "same", ClientID: "client-a", Status: "ACTIVE", AccessTokenCiphertext: "encrypted:synthetic-access", RefreshTokenCiphertext: "encrypted:synthetic-refresh", Environment: "TEST", LastSyncStatus: "FAILED", LastError: "provider detail", ConnectedAt: &now}
	view, err := manager.RequestSync(context.Background(), "client-a", "sync-1", Actor{})
	if err != nil || view.Status != StatusConnected || publisher.calls != 1 {
		t.Fatalf("view=%+v calls=%d err=%v", view, publisher.calls, err)
	}
	disabled, err := manager.Disconnect(context.Background(), "client-a", "disconnect-1", Actor{})
	if err != nil || disabled.Status != StatusDisabled || store.connections["client-a"].ID != "same" {
		t.Fatalf("view=%+v err=%v", disabled, err)
	}
	if _, err = manager.RequestSync(context.Background(), "client-a", "sync-2", Actor{}); !errors.Is(err, ErrConnectionInactive) {
		t.Fatalf("err=%v", err)
	}
}
