package workerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"

	"github.com/hibiken/asynq"
)

type contractAvailableProcessorStub struct {
	contractID, eventKey, correlationID string
	summary                             contracts.ResumeSummary
	err                                 error
}

func (s *contractAvailableProcessorStub) ProcessContractAvailable(_ context.Context, contractID, eventKey, correlationID string) (contracts.ResumeSummary, error) {
	s.contractID, s.eventKey, s.correlationID = contractID, eventKey, correlationID
	return s.summary, s.err
}

func TestContractAvailableHandlerRoutesIdentifierOnlyJob(t *testing.T) {
	processor := &contractAvailableProcessorStub{summary: contracts.ResumeSummary{Evaluated: 3, AutomaticallyResumed: 1, ConfirmationRequired: 1, StillMissingContract: 1}}
	handler := NewContractAvailableHandler(processor, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	job := JobFromOutbox(outbox.Entry{ID: "out-1", EventType: outbox.EventContractAvailable, AggregateID: "contract-1", IdempotencyKey: "available-1", CorrelationID: "trace-1"})
	payload, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = handler.ProcessTask(context.Background(), asynq.NewTask(ContractAvailableTask, payload)); err != nil {
		t.Fatal(err)
	}
	if processor.contractID != "contract-1" || processor.eventKey != "available-1" || processor.correlationID != "trace-1" {
		t.Fatalf("processor received contract=%q key=%q correlation=%q", processor.contractID, processor.eventKey, processor.correlationID)
	}
}

func TestContractAvailableHandlerRejectsMalformedJobWithoutRetry(t *testing.T) {
	handler := NewContractAvailableHandler(&contractAvailableProcessorStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	err := handler.ProcessTask(context.Background(), asynq.NewTask(ContractAvailableTask, []byte(`{"event_type":"CONTRACT_AVAILABLE"}`)))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("error=%v", err)
	}
}

func TestJobFromOutboxRoutesContractAggregate(t *testing.T) {
	job := JobFromOutbox(outbox.Entry{ID: "out-1", EventType: outbox.EventContractAvailable, AggregateID: "contract-1", IdempotencyKey: "key-1"})
	if job.ContractID != "contract-1" || job.InvoiceID != "" {
		t.Fatalf("job=%+v", job)
	}
}
