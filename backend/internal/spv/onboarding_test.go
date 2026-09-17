package spv

import (
	"context"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"errors"
	"net/url"
	"testing"
	"time"
)

type inactiveSPVStore struct{ *memorySPVStore }

func (inactiveSPVStore) ClientOperational(context.Context, string) (bool, error) { return false, nil }
func TestClientOnboardingQueuedSyncStopsForInactiveClient(t *testing.T) {
	store := inactiveSPVStore{&memorySPVStore{connection: Connection{ID: "connection", ClientID: "inactive", Status: "ACTIVE"}}}
	service := NewService(store, nil, nil, nil, nil, ServiceConfig{})
	_, err := service.Sync(context.Background(), "connection")
	if !errors.Is(err, ErrPermanent) {
		t.Fatal("inactive queued sync did not stop", err)
	}
}
func TestClientOnboardingRefreshExpiryRequiresAuthorization(t *testing.T) {
	manager, _, _, _ := newConnectionManagerTest()
	expired := manager.clock().Add(-time.Minute)
	v := manager.view(Connection{Status: "ACTIVE", AccessTokenCiphertext: "encrypted-access", RefreshTokenCiphertext: "encrypted-refresh", RefreshTokenExpiresAt: &expired})
	if v.Status != StatusNeedsReauthentication || v.ImportAutomatic {
		t.Fatalf("%+v", v)
	}
	future := manager.clock().Add(time.Hour)
	v = manager.view(Connection{Status: "ACTIVE", AccessTokenCiphertext: "encrypted-access", RefreshTokenCiphertext: "encrypted-refresh", AccessTokenExpiresAt: expired, RefreshTokenExpiresAt: &future})
	if v.Status != StatusConnected {
		t.Fatal("refreshable access expiry treated as disconnected", v)
	}
}

func TestClientOnboardingOAuthCallbackCannotCrossActorGrants(t *testing.T) {
	manager, _, oauth, _ := newConnectionManagerTest()
	authorization, err := manager.StartOAuth(context.Background(), "client-b")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorization)
	ctx := requestactor.WithActor(context.Background(), requestactor.Actor{ID: "a", AuthorizedClientIDs: []string{"client-a"}})
	_, err = manager.Callback(ctx, parsed.Query().Get("state"), "synthetic-code", "", Actor{ID: "a"})
	if !errors.Is(err, apperrors.ErrNotFound) || oauth.calls != 0 {
		t.Fatal("cross-grant OAuth callback reached provider", err, oauth.calls)
	}
}
