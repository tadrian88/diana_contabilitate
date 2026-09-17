package workerruntime

import (
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
}
func TestContractExtractionMalformedJob(t *testing.T) {
	h := NewContractExtractionHandler(&extractionProcessorStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	if err := h.ProcessTask(context.Background(), asynq.NewTask(ContractExtractionTask, []byte(`{"document_id":"doc"}`))); !errors.Is(err, asynq.SkipRetry) {
		t.Fatal("malformed job retried")
	}
}
