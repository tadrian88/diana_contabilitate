//go:build integration

package postgres

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/accountingclient"
	"diana-contabilitate/backend/ent/spvconnection"
	"diana-contabilitate/backend/ent/spvsourcedocument"
	"diana-contabilitate/backend/internal/spv"
)

func TestConcurrentSPVDiscoveryAndClaimProduceOneTechnicalDelivery(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	clientID := "spv-integration-client"
	connectionID := "spv-integration-connection"
	defer func() {
		_, _ = store.Client.SPVSourceDocument.Delete().Where(spvsourcedocument.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.SPVConnection.Delete().Where(spvconnection.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.AccountingClient.Delete().Where(accountingclient.IDEQ(clientID)).Exec(ctx)
	}()
	_, err = store.Client.AccountingClient.Create().SetID(clientID).SetName("SPV integration").SetCui("RO990001").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Client.SPVConnection.Create().SetID(connectionID).SetClientID(clientID).SetCif("RO990001").SetEnvironment(spvconnection.EnvironmentTEST).SetAccessTokenCiphertext("encrypted").SetRefreshTokenCiphertext("encrypted").SetAccessTokenExpiresAt(now.Add(time.Hour)).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	connection := spv.Connection{ID: connectionID, ClientID: clientID, CIF: "RO990001"}
	message := spv.Message{ID: "90001", UploadID: "80001"}
	var created atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, wasCreated, discoverErr := store.Discover(ctx, connection, message, now)
			if discoverErr != nil {
				t.Errorf("discover: %v", discoverErr)
				return
			}
			if wasCreated {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 {
		t.Fatalf("created=%d", created.Load())
	}
	document, _, err := store.Discover(ctx, connection, message, now)
	if err != nil {
		t.Fatal(err)
	}
	var claims atomic.Int32
	for index := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			if _, claimErr := store.ClaimSourceDocument(ctx, document.ID, "worker", now.Add(time.Duration(worker)*time.Millisecond), time.Minute); claimErr == nil {
				claims.Add(1)
			}
		}(index)
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatalf("claims=%d", claims.Load())
	}
}
