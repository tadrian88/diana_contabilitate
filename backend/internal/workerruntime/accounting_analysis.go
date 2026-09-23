package workerruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

type AccountingAnalysisProcessor interface {
	Process(context.Context, string) error
	Fail(context.Context, string) error
}
type AccountingAnalysisHandler struct{ processor AccountingAnalysisProcessor }

func NewAccountingAnalysisHandler(processor AccountingAnalysisProcessor) *AccountingAnalysisHandler {
	return &AccountingAnalysisHandler{processor: processor}
}
func (h *AccountingAnalysisHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload struct {
		RunID string `json:"runId"`
	}
	if task.Type() != AccountingAnalysisTask || json.Unmarshal(task.Payload(), &payload) != nil || payload.RunID == "" {
		return fmt.Errorf("%w: malformed accounting analysis job", asynq.SkipRetry)
	}
	err := h.processor.Process(ctx, payload.RunID)
	if err != nil {
		attempt, attemptOK := asynq.GetRetryCount(ctx)
		maximum, maximumOK := asynq.GetMaxRetry(ctx)
		if attemptOK && maximumOK && attempt >= maximum {
			if failErr := h.processor.Fail(context.WithoutCancel(ctx), payload.RunID); failErr != nil {
				return fmt.Errorf("analysis failed: %v; terminal status update: %w", err, failErr)
			}
		}
	}
	return err
}
