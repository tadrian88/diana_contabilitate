package workerruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"diana-contabilitate/backend/internal/accountinganalysis"

	"github.com/hibiken/asynq"
)

type AccountingAnalysisProcessor interface {
	Process(context.Context, accountinganalysis.AnalysisJob) error
	Fail(context.Context, string, string) error
}
type AccountingAnalysisHandler struct{ processor AccountingAnalysisProcessor }

func NewAccountingAnalysisHandler(processor AccountingAnalysisProcessor) *AccountingAnalysisHandler {
	return &AccountingAnalysisHandler{processor: processor}
}
func (h *AccountingAnalysisHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload accountinganalysis.AnalysisJob
	if task.Type() != AccountingAnalysisTask || json.Unmarshal(task.Payload(), &payload) != nil || payload.AnalysisRunID == "" || payload.TenantID == "" || payload.InvoiceID == "" || payload.ClassificationRunID == "" {
		return fmt.Errorf("%w: malformed accounting analysis job", asynq.SkipRetry)
	}
	err := h.processor.Process(ctx, payload)
	if err != nil {
		if !accountinganalysis.IsRetryable(err) {
			if failErr := h.processor.Fail(context.WithoutCancel(ctx), payload.AnalysisRunID, accountinganalysis.FailureCode(err)); failErr != nil {
				return fmt.Errorf("analysis failed: %v; terminal status update: %w", err, failErr)
			}
			return nil
		}
		attempt, attemptOK := asynq.GetRetryCount(ctx)
		maximum, maximumOK := asynq.GetMaxRetry(ctx)
		if attemptOK && maximumOK && attempt >= maximum {
			if failErr := h.processor.Fail(context.WithoutCancel(ctx), payload.AnalysisRunID, accountinganalysis.FailureCode(err)); failErr != nil {
				return fmt.Errorf("analysis failed: %v; terminal status update: %w", err, failErr)
			}
		}
	}
	return err
}
