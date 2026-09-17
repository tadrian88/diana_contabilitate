//go:build redis_integration

package workerruntime

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func TestManualSPVSyncClicksCoalesceInExistingAsynqQueue(t *testing.T) {
	options, _ := isolatedRedis(t)
	client := asynq.NewClient(options)
	defer client.Close()
	publisher := NewAsynqPublisher(client, "workflow-test", 3, time.Minute)
	if _, err := publisher.PublishSPVSync(context.Background(), "connection-manual-sync"); err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.PublishSPVSync(context.Background(), "connection-manual-sync"); err != nil {
		t.Fatalf("duplicate click should be accepted: %v", err)
	}
	inspector := asynq.NewInspector(options)
	defer inspector.Close()
	pending, err := inspector.ListPendingTasks("workflow-test")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, task := range pending {
		if task.Type == SPVSyncTask {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("SPV sync jobs=%d", count)
	}
}
