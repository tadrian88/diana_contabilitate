package workerruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ContractAvailableProcessor interface {
	ProcessContractAvailable(context.Context, string, string, string) (contracts.ResumeSummary, error)
}

type ContractAvailableHandler struct {
	processor ContractAvailableProcessor
	logger    *slog.Logger
	metrics   *observability.Metrics
}

func NewContractAvailableHandler(processor ContractAvailableProcessor, logger *slog.Logger, metrics *observability.Metrics) *ContractAvailableHandler {
	return &ContractAvailableHandler{processor: processor, logger: logger, metrics: metrics}
}

func (h *ContractAvailableHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	started := time.Now()
	var job Job
	if task.Type() != ContractAvailableTask || json.Unmarshal(task.Payload(), &job) != nil ||
		job.OutboxID == "" || job.ContractID == "" || job.EventType != outbox.EventContractAvailable || job.IdempotencyKey == "" {
		h.metrics.ContractResumeFailed()
		return fmt.Errorf("%w: malformed contract-available job", asynq.SkipRetry)
	}
	jobID, _ := asynq.GetTaskID(ctx)
	attempt, _ := asynq.GetRetryCount(ctx)
	ctx, span := otel.Tracer("diana/worker").Start(ctx, "workflow.contract_available")
	span.SetAttributes(attribute.String("job.id", jobID), attribute.String("outbox.id", job.OutboxID), attribute.String("contract.id", job.ContractID), attribute.String("correlation.id", job.CorrelationID), attribute.Int("job.attempt", attempt+1))
	defer span.End()
	summary, err := h.processor.ProcessContractAvailable(ctx, job.ContractID, job.IdempotencyKey, job.CorrelationID)
	if err != nil {
		span.RecordError(err)
		h.metrics.ContractResumeFailed()
		if permanent(err) {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		return err
	}
	h.metrics.ContractResumeCompleted(summary.Evaluated, summary.AutomaticallyResumed, summary.ConfirmationRequired, summary.StillMissingContract)
	h.logger.Info("contract availability processed", "job_id", jobID, "outbox_id", job.OutboxID, "contract_id", job.ContractID, "correlation_id", job.CorrelationID, "attempt", attempt+1, "duration_ms", time.Since(started).Milliseconds(), "evaluated", summary.Evaluated, "automatically_resumed", summary.AutomaticallyResumed, "confirmation_required", summary.ConfirmationRequired, "still_missing", summary.StillMissingContract, "stale", summary.Stale)
	return nil
}
