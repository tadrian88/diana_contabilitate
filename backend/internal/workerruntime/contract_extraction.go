package workerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ContractExtractionProcessor interface {
	Extract(context.Context, string) error
}

type ContractActivationHandler struct {
	availability contractingestion.ContractAvailability
}

func NewContractActivationHandler(availability contractingestion.ContractAvailability) *ContractActivationHandler {
	return &ContractActivationHandler{availability: availability}
}
func (h *ContractActivationHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var job Job
	if task.Type() != ContractActivationTask || json.Unmarshal(task.Payload(), &job) != nil || job.OutboxID == "" || job.ContractID == "" || job.EventType != outbox.EventContractActivationRequested || job.IdempotencyKey == "" {
		return fmt.Errorf("%w: malformed contract activation job", asynq.SkipRetry)
	}
	_, err := h.availability.ContractAvailable(ctx, contracts.AvailableCommand{ContractID: job.ContractID, CommandID: job.IdempotencyKey, CorrelationID: job.CorrelationID})
	return err
}

type ContractExtractionHandler struct {
	processor ContractExtractionProcessor
	logger    *slog.Logger
	metrics   *observability.Metrics
}

func NewContractExtractionHandler(processor ContractExtractionProcessor, logger *slog.Logger, metrics *observability.Metrics) *ContractExtractionHandler {
	return &ContractExtractionHandler{processor: processor, logger: logger, metrics: metrics}
}
func (h *ContractExtractionHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	started := time.Now()
	var job Job
	if task.Type() != ContractExtractionTask || json.Unmarshal(task.Payload(), &job) != nil || job.OutboxID == "" || job.DocumentID == "" || job.EventType != outbox.EventContractExtractionRequested || job.IdempotencyKey == "" {
		h.metrics.ContractExtractionFailed()
		return fmt.Errorf("%w: malformed contract extraction job", asynq.SkipRetry)
	}
	jobID, _ := asynq.GetTaskID(ctx)
	attempt, _ := asynq.GetRetryCount(ctx)
	ctx, span := otel.Tracer("diana/worker").Start(ctx, "workflow.contract_extraction")
	span.SetAttributes(attribute.String("job.id", jobID), attribute.String("outbox.id", job.OutboxID), attribute.String("contract_document.id", job.DocumentID), attribute.String("correlation.id", job.CorrelationID), attribute.Int("job.attempt", attempt+1))
	defer span.End()
	err := h.processor.Extract(ctx, job.DocumentID)
	if err == nil {
		h.metrics.ContractExtractionCompleted(time.Since(started))
		h.logger.Info("contract extraction completed", "job_id", jobID, "document_id", job.DocumentID, "duration_ms", time.Since(started).Milliseconds())
		return nil
	}
	span.RecordError(err)
	h.metrics.ContractExtractionFailed()
	if errors.Is(err, contractingestion.ErrExtractionPermanent) {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return err
}
