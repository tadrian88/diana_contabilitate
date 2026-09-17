package invoicing

import (
	"context"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/outbox"
)

type noMutationPipelineStore struct {
	invoice     *Invoice
	transitions int
}

type unconfirmedExporter struct{}

func (unconfirmedExporter) Export(context.Context, *Invoice, string) (SagaExportResult, error) {
	return SagaExportResult{Confirmed: false, Reference: "generated-attempt"}, nil
}

type transientExporter struct{ err error }

func (e transientExporter) Export(context.Context, *Invoice, string) (SagaExportResult, error) {
	return SagaExportResult{}, e.err
}

func (s *noMutationPipelineStore) GetInvoice(context.Context, string) (*Invoice, error) {
	return s.invoice, nil
}

func TestWorkerDoesNotEquateGeneratedFileWithSAGAImport(t *testing.T) {
	store := &noMutationPipelineStore{invoice: &Invoice{ID: "invoice-1", ClientID: "client-1", PipelineStatus: StatusExporting, Revision: 4}}
	service := NewPipelineService(store, unconfirmedExporter{}, nil)
	outcome, err := service.ProcessContinuation(context.Background(), OutboxEntry{ID: "out-1", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-1", IdempotencyKey: "continue-1"})
	if err != nil || outcome != ContinuationProcessed || store.transitions != 0 {
		t.Fatalf("outcome=%s transitions=%d err=%v", outcome, store.transitions, err)
	}
}

func TestWorkerReturnsTransientSAGAInfrastructureFailureForBoundedRetry(t *testing.T) {
	want := errors.New("temporary artifact store failure")
	store := &noMutationPipelineStore{invoice: &Invoice{ID: "invoice-1", ClientID: "client-1", PipelineStatus: StatusExporting, Revision: 4}}
	service := NewPipelineService(store, transientExporter{err: want}, nil)
	outcome, err := service.ProcessContinuation(context.Background(), OutboxEntry{ID: "out-1", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-1", IdempotencyKey: "continue-1"})
	if !errors.Is(err, want) || outcome != ContinuationProcessed || store.transitions != 0 {
		t.Fatalf("outcome=%s transitions=%d err=%v", outcome, store.transitions, err)
	}
}
func (*noMutationPipelineStore) IngestInvoice(context.Context, IngestionInput, string, time.Time) (*Invoice, bool, error) {
	panic("not expected")
}
func (s *noMutationPipelineStore) ApplyTransition(context.Context, TransitionCommand, TransitionDefinition, time.Time) (*Invoice, bool, error) {
	s.transitions++
	return s.invoice, true, nil
}
func (*noMutationPipelineStore) RecordSagaFailure(context.Context, string, uint64, string, string, time.Time) (*Invoice, bool, error) {
	panic("not expected")
}
func (*noMutationPipelineStore) PendingOutbox(context.Context, int, time.Time) ([]OutboxEntry, error) {
	return nil, nil
}
func (*noMutationPipelineStore) MarkOutboxProcessed(context.Context, string, time.Time) error {
	return nil
}

func TestWorkerAcknowledgesTerminalAndHumanBlockedInvoicesWithoutMutation(t *testing.T) {
	for _, status := range []PipelineStatus{StatusDuplicate, StatusExported, StatusAwaitingContract, StatusAwaitingMatchConfirm, StatusAwaitingReview} {
		store := &noMutationPipelineStore{invoice: &Invoice{ID: "invoice-1", PipelineStatus: status, Revision: 4}}
		service := NewPipelineService(store, NewFakeSagaExporter(), nil)
		outcome, err := service.ProcessContinuation(context.Background(), OutboxEntry{ID: "out-1", EventType: outbox.EventInvoiceContinue, AggregateID: "invoice-1", IdempotencyKey: "continue-1"})
		if err != nil || outcome != ContinuationStale || store.transitions != 0 {
			t.Errorf("status=%s outcome=%s transitions=%d err=%v", status, outcome, store.transitions, err)
		}
	}
}
