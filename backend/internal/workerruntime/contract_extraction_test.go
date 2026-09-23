package workerruntime

import (
	"bytes"
	"context"
	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"
	"encoding/json"
	"errors"
	"github.com/hibiken/asynq"
	"io"
	"log/slog"
	"strings"
	"testing"
)

type extractionProcessorStub struct {
	id  string
	err error
}

func (p *extractionProcessorStub) Extract(_ context.Context, id string) error {
	p.id = id
	return p.err
}
func TestContractExtractionIdentifierOnlyOutboxJob(t *testing.T) {
	p := &extractionProcessorStub{}
	handler := NewContractExtractionHandler(p, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	job := JobFromOutbox(outbox.Entry{ID: "out", EventType: outbox.EventContractExtractionRequested, AggregateID: "doc", IdempotencyKey: "key"})
	payload, _ := json.Marshal(job)
	if strings.Contains(string(payload), "PDF") || job.DocumentID != "doc" || job.InvoiceID != "" {
		t.Fatal("sensitive payload or routing")
	}
	if err := handler.ProcessTask(context.Background(), asynq.NewTask(ContractExtractionTask, payload)); err != nil || p.id != "doc" {
		t.Fatal("extraction job")
	}
	for _, failure := range []error{ci.ErrExtractionPermanent, ci.ErrExtractionTransient, ci.ErrExtractionBusy} {
		p.err = failure
		err := handler.ProcessTask(context.Background(), asynq.NewTask(ContractExtractionTask, payload))
		if errors.Is(err, asynq.SkipRetry) != errors.Is(failure, ci.ErrExtractionPermanent) {
			t.Fatalf("retry category=%v", err)
		}
	}
	p.err = &ci.ExtractionFailure{Category: ci.FailureInvalidStructuredOutput, Retry: ci.RetryOnce, Provider: "GEMINI", Model: "test-model"}
	if err := handler.ProcessTask(context.Background(), asynq.NewTask(ContractExtractionTask, payload)); err == nil || errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("first invalid output must receive one retry: %v", err)
	}
	p.err = &ci.ExtractionFailure{Category: ci.FailureProviderAuthentication, Retry: ci.RetryNever, HTTPStatus: 401, Provider: "GEMINI", Model: "test-model"}
	if err := handler.ProcessTask(context.Background(), asynq.NewTask(ContractExtractionTask, payload)); !errors.Is(err, asynq.SkipRetry) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("permanent safe category was retried or leaked: %v", err)
	}
}
func TestContractExtractionMalformedJob(t *testing.T) {
	h := NewContractExtractionHandler(&extractionProcessorStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	if err := h.ProcessTask(context.Background(), asynq.NewTask(ContractExtractionTask, []byte(`{"document_id":"doc"}`))); !errors.Is(err, asynq.SkipRetry) {
		t.Fatal("malformed job retried")
	}
}

func TestContractExtractionLogsSafeValidationDiagnostic(t *testing.T) {
	var output bytes.Buffer
	p := &extractionProcessorStub{err: &ci.ExtractionFailure{
		Category: ci.FailureProposalValidationFailed, Retry: ci.RetryOnce, Provider: "GEMINI", Model: "test-model",
		ValidationCode: "INVALID_AMOUNT", ValidationPath: "serviceTerms[0].unitPrice",
	}}
	handler := NewContractExtractionHandler(p, slog.New(slog.NewJSONHandler(&output, nil)), observability.NewMetrics())
	job := JobFromOutbox(outbox.Entry{ID: "out", EventType: outbox.EventContractExtractionRequested, AggregateID: "doc", IdempotencyKey: "key"})
	payload, _ := json.Marshal(job)
	_ = handler.ProcessTask(context.Background(), asynq.NewTask(ContractExtractionTask, payload))
	logged := output.String()
	if !strings.Contains(logged, `"validation_code":"INVALID_AMOUNT"`) || !strings.Contains(logged, `"validation_path":"serviceTerms[0].unitPrice"`) || strings.Contains(logged, "secret-price") {
		t.Fatalf("unsafe or incomplete log: %s", logged)
	}
}
