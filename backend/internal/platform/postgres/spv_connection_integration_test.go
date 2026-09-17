//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/spvconnection"
	"diana-contabilitate/backend/ent/spvoauthstate"
	"diana-contabilitate/backend/ent/spvsourcedocument"
	"diana-contabilitate/backend/internal/spv"
)

func TestSPVOAuthStateConnectionLifecycleAndAuditAreDurable(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientID := "spv-ux-client-" + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)
	defer func() {
		_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.SPVOAuthState.Delete().Where(spvoauthstate.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.SPVSourceDocument.Delete().Where(spvsourcedocument.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.SPVConnection.Delete().Where(spvconnection.ClientIDEQ(clientID)).Exec(ctx)
		_ = store.Client.AccountingClient.DeleteOneID(clientID).Exec(ctx)
	}()
	if _, err = store.Client.AccountingClient.Create().SetID(clientID).SetName("Client SPV UX").SetCui("RO" + suffix).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	state := spv.OAuthState{ID: "state-" + suffix, StateHash: "hash-" + suffix, ClientID: clientID, Environment: "TEST", ReturnPath: "/clients/" + clientID, CreatedAt: now, ExpiresAt: now.Add(5 * time.Minute)}
	if err = store.CreateOAuthState(ctx, state); err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, consumeErr := store.ConsumeOAuthState(ctx, state.StateHash, now.Add(time.Second)); consumeErr == nil {
				winners.Add(1)
			} else if !errors.Is(consumeErr, spv.ErrInvalidOAuthState) {
				t.Errorf("consume: %v", consumeErr)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("state winners=%d", winners.Load())
	}
	connection, reconnected, err := store.CompleteOAuthConnection(ctx, state, "encrypted-access", "encrypted-refresh", now.Add(time.Hour), nil, spv.Actor{ID: "user-1", Display: "Contabil"}, now.Add(time.Second))
	if err != nil || reconnected {
		t.Fatalf("connection=%+v reconnected=%v err=%v", connection, reconnected, err)
	}
	if connection.Status != "ACTIVE" || connection.ConnectedAt == nil {
		t.Fatalf("connection=%+v", connection)
	}
	if err = store.MarkSyncFinished(ctx, connection.ID, now.Add(1500*time.Millisecond), errors.Join(spv.ErrReauthenticationRequired, spv.ErrPermanent)); err != nil {
		t.Fatal(err)
	}
	connectionAfterRefresh, err := store.GetConnection(ctx, connection.ID)
	if err != nil || connectionAfterRefresh.Status != "EXPIRED" || connectionAfterRefresh.LastSyncStatus != "FAILED" {
		t.Fatalf("connection=%+v err=%v", connectionAfterRefresh, err)
	}
	if _, err = store.Client.SPVSourceDocument.Create().SetID("doc-" + suffix).SetConnectionID(connection.ID).SetClientID(clientID).SetExternalMessageID("message-" + suffix).SetDiscoveredAt(now).SetAvailableAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	disabled, changed, err := store.DisconnectSPVConnection(ctx, clientID, "disconnect-"+suffix, spv.Actor{ID: "user-1"}, now.Add(2*time.Second))
	if err != nil || !changed || disabled.Status != "REVOKED" {
		t.Fatalf("connection=%+v changed=%v err=%v", disabled, changed, err)
	}
	if count, _ := store.Client.SPVSourceDocument.Query().Where(spvsourcedocument.ClientIDEQ(clientID)).Count(ctx); count != 1 {
		t.Fatalf("source documents=%d", count)
	}
	if count, _ := store.Client.ActivityEvent.Query().Where(activityevent.ClientIDEQ(clientID), activityevent.EventTypeIn("SPV_CONNECTION_CONNECTED", "SPV_CONNECTION_DISCONNECTED")).Count(ctx); count != 2 {
		t.Fatalf("audit events=%d", count)
	}
}
