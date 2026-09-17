//go:build redis_integration

package workerruntime

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"
	"github.com/hibiken/asynq"
)

type redisExtractionProcessor struct {
	calls, logical atomic.Int32
	transient      atomic.Bool
	wrongID        atomic.Bool
}

func (p *redisExtractionProcessor) Extract(_ context.Context, id string) error {
	p.calls.Add(1)
	if id != "document-test" {
		p.wrongID.Store(true)
	}
	if p.transient.CompareAndSwap(true, false) {
		return contractingestion.ErrExtractionTransient
	}
	p.logical.CompareAndSwap(0, 1)
	return nil
}
func TestRealAsynqContractExtractionDuplicateAndTransientRetry(t *testing.T) {
	options, _ := isolatedRedis(t)
	client := asynq.NewClient(options)
	defer client.Close()
	processor := &redisExtractionProcessor{}
	processor.transient.Store(true)
	handler := NewContractExtractionHandler(processor, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	server := asynq.NewServer(options, asynq.Config{Concurrency: 2, Queues: map[string]int{"workflow-test": 1}, DelayedTaskCheckInterval: 20 * time.Millisecond, RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond }})
	mux := asynq.NewServeMux()
	mux.Handle(ContractExtractionTask, handler)
	if err := server.Start(mux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Shutdown)
	publisher := NewAsynqPublisher(client, "workflow-test", 3, time.Minute)
	job := JobFromOutbox(outbox.Entry{ID: "out-test", EventType: outbox.EventContractExtractionRequested, AggregateID: "document-test", IdempotencyKey: "extract-test"})
	for i := 0; i < 2; i++ {
		if _, err := publisher.Publish(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, func() bool { return processor.calls.Load() >= 3 && processor.logical.Load() == 1 })
	if processor.wrongID.Load() {
		t.Fatal("document identifier changed across queue transport")
	}
}
