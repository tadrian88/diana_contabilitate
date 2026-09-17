//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/outboxentry"
)

func TestConcurrentOutboxClaimUsesSkipLockedAndClaimsEveryRowOnce(t *testing.T) {
	store, ctx, _, _ := pipelineTestStore(t)
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := make([]string, 12)
	for index := range ids {
		ids[index] = fmt.Sprintf("m7-claim-%d-%d", integrationSequence.Load(), index)
		payload, _ := json.Marshal(map[string]string{"invoice_id": ids[index]})
		if _, err := store.Client.OutboxEntry.Create().SetID(ids[index]).SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID(ids[index]).SetPayload(payload).SetIdempotencyKey("key-" + ids[index]).SetStatus(outboxentry.StatusPENDING).SetCreatedAt(base.Add(time.Duration(index) * time.Second)).SetAvailableAt(base).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.IDIn(ids...)).Exec(ctx) })

	claimed := make(chan string, len(ids))
	var group sync.WaitGroup
	for worker := range 3 {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			rows, err := store.Claim(context.Background(), fmt.Sprintf("worker-%d", worker), 4, base, time.Minute)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			for _, row := range rows {
				claimed <- row.ID
			}
		}(worker)
	}
	group.Wait()
	close(claimed)
	seen := make(map[string]bool)
	for id := range claimed {
		if seen[id] {
			t.Fatalf("row claimed twice: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != len(ids) {
		t.Fatalf("claimed=%d want=%d", len(seen), len(ids))
	}
}

func TestExpiredOutboxLeaseIsRecoveredAfterDispatcherCrash(t *testing.T) {
	store, ctx, _, _ := pipelineTestStore(t)
	base := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	id := fmt.Sprintf("m7-recovery-%d", integrationSequence.Load())
	payload, _ := json.Marshal(map[string]string{"invoice_id": id})
	if _, err := store.Client.OutboxEntry.Create().SetID(id).SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID(id).SetPayload(payload).SetIdempotencyKey("key-" + id).SetStatus(outboxentry.StatusPENDING).SetCreatedAt(base).SetAvailableAt(base).Save(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Client.OutboxEntry.DeleteOneID(id).Exec(ctx) })
	first, err := store.Claim(ctx, "crashed-worker", 1, base, time.Minute)
	if err != nil || len(first) != 1 {
		t.Fatalf("first=%v err=%v", first, err)
	}
	beforeExpiry, err := store.Claim(ctx, "replacement-worker", 1, base.Add(30*time.Second), time.Minute)
	if err != nil || len(beforeExpiry) != 0 {
		t.Fatalf("premature claim=%v err=%v", beforeExpiry, err)
	}
	afterExpiry, err := store.Claim(ctx, "replacement-worker", 1, base.Add(2*time.Minute), time.Minute)
	if err != nil || len(afterExpiry) != 1 || afterExpiry[0].Attempts != 2 {
		t.Fatalf("recovered=%v err=%v", afterExpiry, err)
	}
}
