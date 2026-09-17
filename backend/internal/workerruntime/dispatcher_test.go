package workerruntime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type memoryOutbox struct {
	mu         sync.Mutex
	entries    []outbox.Entry
	dispatched []string
	released   []string
	failed     []string
	claimed    map[string]outbox.Entry
	markErr    error
}

func (s *memoryOutbox) Claim(context.Context, string, int, time.Time, time.Duration) ([]outbox.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := append([]outbox.Entry(nil), s.entries...)
	s.entries = nil
	if s.claimed == nil {
		s.claimed = make(map[string]outbox.Entry)
	}
	for _, entry := range result {
		s.claimed[entry.ID] = entry
	}
	return result, nil
}
func (s *memoryOutbox) MarkDispatched(_ context.Context, id, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.markErr != nil {
		return s.markErr
	}
	s.dispatched = append(s.dispatched, id)
	delete(s.claimed, id)
	return nil
}
func (s *memoryOutbox) ReleaseClaim(_ context.Context, id, _, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released = append(s.released, id)
	if entry, ok := s.claimed[id]; ok {
		s.entries = append(s.entries, entry)
		delete(s.claimed, id)
	}
	return nil
}
func (s *memoryOutbox) MarkFailed(_ context.Context, id, _, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, id)
	delete(s.claimed, id)
	return nil
}
func (*memoryOutbox) Stats(context.Context, time.Time) (outbox.Stats, error) {
	return outbox.Stats{}, nil
}

type capturePublisher struct {
	jobs []Job
	err  error
}

func (p *capturePublisher) Publish(_ context.Context, job Job) (string, error) {
	p.jobs = append(p.jobs, job)
	return "job-1", p.err
}

func testDispatcher(store outbox.Store, publisher Publisher) *Dispatcher {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewDispatcher(store, publisher, DispatcherConfig{Owner: "worker-1", BatchSize: 10, MaxAttempts: 3, PollInterval: time.Second, ClaimTTL: time.Minute, RetryMin: time.Second, RetryMax: time.Minute}, logger, observability.NewMetrics())
}

func TestDispatcherArchivesExhaustedPublishFailureWithoutValidationWork(t *testing.T) {
	store := &memoryOutbox{entries: []outbox.Entry{{ID: "out-1", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-1", IdempotencyKey: "continue-1", Attempts: 3}}}
	publisher := &capturePublisher{err: errors.New("redis unavailable")}
	count, err := testDispatcher(store, publisher).DispatchBatch(context.Background())
	if err == nil || count != 0 || len(store.failed) != 1 || len(store.released) != 0 {
		t.Fatalf("count=%d failed=%v released=%v err=%v", count, store.failed, store.released, err)
	}
}

func TestDispatcherPublishesIdentityPayloadThenMarksDispatched(t *testing.T) {
	store := &memoryOutbox{entries: []outbox.Entry{{ID: "out-1", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-1", IdempotencyKey: "continue-1", CorrelationID: "request-1", Attempts: 1}}}
	publisher := &capturePublisher{}
	count, err := testDispatcher(store, publisher).DispatchBatch(context.Background())
	if err != nil || count != 1 || len(store.dispatched) != 1 || len(publisher.jobs) != 1 {
		t.Fatalf("count=%d dispatched=%v jobs=%v err=%v", count, store.dispatched, publisher.jobs, err)
	}
	if publisher.jobs[0].InvoiceID != "invoice-1" || publisher.jobs[0].CorrelationID != "request-1" {
		t.Fatalf("job contains mutable data or lost correlation: %+v", publisher.jobs[0])
	}
}

func TestDispatcherReleasesClaimAfterTransientRedisFailure(t *testing.T) {
	store := &memoryOutbox{entries: []outbox.Entry{{ID: "out-1", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-1", IdempotencyKey: "continue-1", Attempts: 1}}}
	publisher := &capturePublisher{err: errors.New("redis unavailable")}
	count, err := testDispatcher(store, publisher).DispatchBatch(context.Background())
	if err == nil || count != 0 || len(store.released) != 1 || len(store.dispatched) != 0 {
		t.Fatalf("count=%d released=%v dispatched=%v err=%v", count, store.released, store.dispatched, err)
	}
	publisher.err = nil
	count, err = testDispatcher(store, publisher).DispatchBatch(context.Background())
	if err != nil || count != 1 || len(store.dispatched) != 1 {
		t.Fatalf("recovery count=%d dispatched=%v err=%v", count, store.dispatched, err)
	}
}

func TestDispatcherStopsPollingOnCancellation(t *testing.T) {
	dispatcher := testDispatcher(&memoryOutbox{}, &capturePublisher{})
	dispatcher.config.PollInterval = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- dispatcher.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not stop")
	}
}

func TestPublishMarkCrashWindowLeavesClaimForLeaseRecovery(t *testing.T) {
	store := &memoryOutbox{entries: []outbox.Entry{{ID: "out-1", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-1", IdempotencyKey: "continue-1", Attempts: 1}}, markErr: errors.New("database unavailable after publish")}
	publisher := &capturePublisher{}
	count, err := testDispatcher(store, publisher).DispatchBatch(context.Background())
	if err == nil || count != 0 || len(publisher.jobs) != 1 || len(store.released) != 0 {
		t.Fatalf("count=%d jobs=%d released=%v err=%v", count, len(publisher.jobs), store.released, err)
	}
}

func TestDispatcherCreatesAnOpenTelemetryBoundary(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)
	store := &memoryOutbox{}
	if _, err := testDispatcher(store, &capturePublisher{}).DispatchBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "outbox.dispatch" {
		t.Fatalf("spans=%+v", spans)
	}
}
