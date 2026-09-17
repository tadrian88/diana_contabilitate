//go:build redis_integration

package workerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type idempotentProcessor struct {
	calls        atomic.Int32
	logical      atomic.Int32
	failuresLeft atomic.Int32
}

func (p *idempotentProcessor) ProcessContinuation(context.Context, invoicing.OutboxEntry) (invoicing.ContinuationOutcome, error) {
	p.calls.Add(1)
	if p.failuresLeft.Add(-1) >= 0 {
		return "", errors.New("temporary database outage")
	}
	if p.logical.CompareAndSwap(0, 1) {
		return invoicing.ContinuationProcessed, nil
	}
	return invoicing.ContinuationStale, nil
}

func isolatedRedis(t *testing.T) (asynq.RedisConnOpt, *redis.Client) {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	redisOptions, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if redisOptions.DB == 0 {
		t.Fatal("TEST_REDIS_URL must select a dedicated non-zero Redis database")
	}
	client := redis.NewClient(redisOptions)
	if err = client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	options, err := asynq.ParseRedisURI(url)
	if err != nil {
		t.Fatal(err)
	}
	return options, client
}

func startTestWorker(t *testing.T, options asynq.RedisConnOpt, processor ContinuationProcessor) (*asynq.Server, func()) {
	return startTestWorkerWithRetryDelay(t, options, processor, 10*time.Millisecond)
}

func startTestWorkerWithRetryDelay(t *testing.T, options asynq.RedisConnOpt, processor ContinuationProcessor, retryDelay time.Duration) (*asynq.Server, func()) {
	t.Helper()
	server := asynq.NewServer(options, asynq.Config{
		Concurrency:              2,
		Queues:                   map[string]int{"workflow-test": 1},
		DelayedTaskCheckInterval: 20 * time.Millisecond,
		RetryDelayFunc:           func(int, error, *asynq.Task) time.Duration { return retryDelay },
	})
	mux := asynq.NewServeMux()
	mux.Handle(ContinueInvoiceTask, NewHandler(processor, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics()))
	if err := server.Start(mux); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() { once.Do(server.Shutdown) }
	t.Cleanup(stop)
	return server, stop
}

func enqueueTestJob(t *testing.T, options asynq.RedisConnOpt, suffix string) {
	t.Helper()
	client := asynq.NewClient(options)
	defer client.Close()
	job := Job{OutboxID: "out-" + suffix, InvoiceID: "invoice-" + suffix, EventType: outbox.EventInvoiceContinue, IdempotencyKey: "continue-" + suffix, CorrelationID: "request-" + suffix}
	payload, _ := json.Marshal(job)
	if _, err := client.Enqueue(asynq.NewTask(ContinueInvoiceTask, payload), asynq.Queue("workflow-test"), asynq.MaxRetry(3)); err != nil {
		t.Fatal(err)
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("eventual condition was not met")
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

func TestRealAsynqDuplicateDeliveryProducesOneLogicalMutation(t *testing.T) {
	options, _ := isolatedRedis(t)
	processor := &idempotentProcessor{}
	_, _ = startTestWorker(t, options, processor)
	enqueueTestJob(t, options, "duplicate")
	enqueueTestJob(t, options, "duplicate")
	eventually(t, func() bool { return processor.calls.Load() >= 2 })
	if processor.logical.Load() != 1 {
		t.Fatalf("logical mutations=%d", processor.logical.Load())
	}
}

func TestRealAsynqRetriesTransientWorkerFailure(t *testing.T) {
	options, _ := isolatedRedis(t)
	processor := &idempotentProcessor{}
	processor.failuresLeft.Store(1)
	_, _ = startTestWorker(t, options, processor)
	enqueueTestJob(t, options, "retry")
	eventually(t, func() bool { return processor.logical.Load() == 1 })
	if processor.calls.Load() < 2 {
		t.Fatalf("calls=%d", processor.calls.Load())
	}
}

func TestRedisDispatchOutageReleasesOutboxAndRecoveryLosesNoWork(t *testing.T) {
	options, _ := isolatedRedis(t)
	store := &memoryOutbox{entries: []outbox.Entry{{ID: "out-dispatch-recovery", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-dispatch-recovery", IdempotencyKey: "continue-dispatch-recovery", CorrelationID: "request-dispatch-recovery", Attempts: 1}}}
	unavailableClient := asynq.NewClient(asynq.RedisClientOpt{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond})
	unavailablePublisher := NewAsynqPublisher(unavailableClient, "workflow-test", 3, time.Minute)
	if count, err := testDispatcher(store, unavailablePublisher).DispatchBatch(context.Background()); err == nil || count != 0 || len(store.released) != 1 {
		t.Fatalf("count=%d released=%v err=%v", count, store.released, err)
	}
	_ = unavailableClient.Close()

	processor := &idempotentProcessor{}
	_, _ = startTestWorker(t, options, processor)
	client := asynq.NewClient(options)
	defer client.Close()
	publisher := NewAsynqPublisher(client, "workflow-test", 3, time.Minute)
	if count, err := testDispatcher(store, publisher).DispatchBatch(context.Background()); err != nil || count != 1 {
		t.Fatalf("recovery count=%d err=%v", count, err)
	}
	eventually(t, func() bool { return processor.logical.Load() == 1 })
}

func TestQueuedJobSurvivesWorkerRestart(t *testing.T) {
	options, _ := isolatedRedis(t)
	failing := &idempotentProcessor{}
	failing.failuresLeft.Store(100)
	// Keep the failed delivery in Asynq's durable retry set while the first
	// worker shuts down, rather than allowing a fast test retry to exhaust it.
	_, stopFirst := startTestWorkerWithRetryDelay(t, options, failing, time.Second)
	enqueueTestJob(t, options, "restart")
	eventually(t, func() bool { return failing.calls.Load() >= 1 })
	stopFirst()
	processor := &idempotentProcessor{}
	_, _ = startTestWorker(t, options, processor)
	eventually(t, func() bool { return processor.logical.Load() == 1 })
}
