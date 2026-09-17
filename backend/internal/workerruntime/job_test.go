package workerruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/platform/observability"

	"github.com/hibiken/asynq"
)

type processorStub struct {
	outcome invoicing.ContinuationOutcome
	err     error
	calls   int
}

func (p *processorStub) ProcessContinuation(context.Context, invoicing.OutboxEntry) (invoicing.ContinuationOutcome, error) {
	p.calls++
	return p.outcome, p.err
}

func testHandler(processor ContinuationProcessor) *Handler {
	return NewHandler(processor, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
}

func TestHandlerAcknowledgesStaleAndDuplicateDeliveries(t *testing.T) {
	processor := &processorStub{outcome: invoicing.ContinuationStale}
	handler := testHandler(processor)
	task := asynq.NewTask(ContinueInvoiceTask, []byte(`{"outbox_id":"out-1","invoice_id":"invoice-1","event_type":"INVOICE_CONTINUE","idempotency_key":"continue-1","correlation_id":"request-1"}`))
	if err := handler.ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := handler.ProcessTask(context.Background(), task); err != nil || processor.calls != 2 {
		t.Fatalf("duplicate delivery err=%v calls=%d", err, processor.calls)
	}
}

func TestHandlerDoesNotRetryMalformedOrPermanentJobs(t *testing.T) {
	if err := testHandler(&processorStub{}).ProcessTask(context.Background(), asynq.NewTask(ContinueInvoiceTask, []byte(`{}`))); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("malformed error=%v", err)
	}
	processor := &processorStub{err: apperrors.ErrValidation}
	task := asynq.NewTask(ContinueInvoiceTask, []byte(`{"outbox_id":"out-1","invoice_id":"invoice-1","event_type":"INVOICE_CONTINUE","idempotency_key":"continue-1"}`))
	if err := testHandler(processor).ProcessTask(context.Background(), task); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("permanent error=%v", err)
	}
}

func TestHandlerReturnsTransientDatabaseFailureForAsynqRetry(t *testing.T) {
	transient := errors.New("database unavailable")
	processor := &processorStub{err: transient}
	task := asynq.NewTask(ContinueInvoiceTask, []byte(`{"outbox_id":"out-1","invoice_id":"invoice-1","event_type":"INVOICE_CONTINUE","idempotency_key":"continue-1"}`))
	if err := testHandler(processor).ProcessTask(context.Background(), task); !errors.Is(err, transient) {
		t.Fatalf("transient error=%v", err)
	}
}

func TestHandlerLogCarriesOperationalCorrelationWithoutBusinessPayload(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	processor := &processorStub{outcome: invoicing.ContinuationProcessed}
	handler := NewHandler(processor, logger, observability.NewMetrics())
	task := asynq.NewTask(ContinueInvoiceTask, []byte(`{"outbox_id":"out-1","invoice_id":"invoice-1","event_type":"INVOICE_CONTINUE","idempotency_key":"continue-1","correlation_id":"request-1"}`))
	if err := handler.ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	logLine := output.String()
	for _, field := range []string{`"outbox_id":"out-1"`, `"invoice_id":"invoice-1"`, `"correlation_id":"request-1"`} {
		if !bytes.Contains([]byte(logLine), []byte(field)) {
			t.Fatalf("missing %s in %s", field, logLine)
		}
	}
}
