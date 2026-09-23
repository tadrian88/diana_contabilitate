package workerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	ContinueInvoiceTask    = "workflow:continue_invoice"
	ContractAvailableTask  = "workflow:contract_available"
	ContractExtractionTask = "workflow:contract_extraction"
	ContractActivationTask = "workflow:activate_ingested_contract"
	AccountingAnalysisTask = "workflow:accounting_analysis"
)

type Job struct {
	OutboxID       string `json:"outbox_id"`
	InvoiceID      string `json:"invoice_id"`
	ContractID     string `json:"contract_id,omitempty"`
	DocumentID     string `json:"document_id,omitempty"`
	EventType      string `json:"event_type"`
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
}

func JobFromOutbox(entry outbox.Entry) Job {
	job := Job{OutboxID: entry.ID, EventType: entry.EventType, IdempotencyKey: entry.IdempotencyKey, CorrelationID: entry.CorrelationID}
	if entry.EventType == outbox.EventContractAvailable || entry.EventType == outbox.EventContractActivationRequested {
		job.ContractID = entry.AggregateID
	} else if entry.EventType == outbox.EventContractExtractionRequested {
		job.DocumentID = entry.AggregateID
	} else {
		job.InvoiceID = entry.AggregateID
	}
	return job
}

func (j Job) Entry() outbox.Entry {
	return outbox.Entry{ID: j.OutboxID, EventType: j.EventType, AggregateID: j.InvoiceID, IdempotencyKey: j.IdempotencyKey, CorrelationID: j.CorrelationID}
}

func (j Job) Valid() bool {
	return j.OutboxID != "" && j.InvoiceID != "" && j.EventType == outbox.EventInvoiceContinue && j.IdempotencyKey != ""
}

type ContinuationProcessor interface {
	ProcessContinuation(context.Context, invoicing.OutboxEntry) (invoicing.ContinuationOutcome, error)
}

type Handler struct {
	processor ContinuationProcessor
	logger    *slog.Logger
	metrics   *observability.Metrics
}

func NewHandler(processor ContinuationProcessor, logger *slog.Logger, metrics *observability.Metrics) *Handler {
	return &Handler{processor: processor, logger: logger, metrics: metrics}
}

func (h *Handler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	started := time.Now()
	var job Job
	if task.Type() != ContinueInvoiceTask || json.Unmarshal(task.Payload(), &job) != nil || !job.Valid() {
		h.metrics.JobFailed()
		return fmt.Errorf("%w: malformed internal workflow job", asynq.SkipRetry)
	}
	jobID, _ := asynq.GetTaskID(ctx)
	attempt, _ := asynq.GetRetryCount(ctx)
	ctx, span := otel.Tracer("diana/worker").Start(ctx, "workflow.continue_invoice")
	span.SetAttributes(attribute.String("job.id", jobID), attribute.String("outbox.id", job.OutboxID), attribute.String("invoice.id", job.InvoiceID), attribute.String("correlation.id", job.CorrelationID), attribute.Int("job.attempt", attempt+1))
	defer span.End()
	outcome, err := h.processor.ProcessContinuation(ctx, job.Entry())
	if err == nil {
		h.metrics.JobCompleted(string(outcome), time.Since(started))
		h.logger.Info("workflow job acknowledged", "job_id", jobID, "outbox_id", job.OutboxID, "invoice_id", job.InvoiceID, "correlation_id", job.CorrelationID, "attempt", attempt+1, "duration_ms", time.Since(started).Milliseconds(), "result", outcome)
		return nil
	}
	span.RecordError(err)
	if permanent(err) {
		h.metrics.JobFailed()
		h.logger.Warn("workflow job permanently rejected", "job_id", jobID, "outbox_id", job.OutboxID, "invoice_id", job.InvoiceID, "correlation_id", job.CorrelationID, "attempt", attempt+1, "error_category", "permanent")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	h.metrics.JobRetry()
	h.logger.Error("workflow job failed transiently", "job_id", jobID, "outbox_id", job.OutboxID, "invoice_id", job.InvoiceID, "correlation_id", job.CorrelationID, "attempt", attempt+1, "error_category", "transient", "error", err)
	return err
}

func permanent(err error) bool {
	return errors.Is(err, apperrors.ErrValidation) || errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, contracts.ErrExpiredContractSemantics) || errors.Is(err, contracts.ErrInvalidMatchDecision) ||
		errors.Is(err, classification.ErrInvalidPolicyResult)
}
