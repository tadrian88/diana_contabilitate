package workerruntime

import (
	"context"
	"errors"
	"testing"

	"diana-contabilitate/backend/internal/accountinganalysis"
	"github.com/hibiken/asynq"
)

type analysisProcessorStub struct {
	processed int
	failed    int
	err       error
}

func (s *analysisProcessorStub) Process(context.Context, accountinganalysis.AnalysisJob) error {
	s.processed++
	return s.err
}
func (s *analysisProcessorStub) Fail(context.Context, string, string) error { s.failed++; return nil }

func TestAccountingAnalysisNonRetryableFailureFallsBackToManualReview(t *testing.T) {
	stub := &analysisProcessorStub{err: &accountinganalysis.ProviderError{Code: "INVALID_PROVIDER_SCHEMA", Err: errors.New("bad schema")}}
	handler := NewAccountingAnalysisHandler(stub)
	task := asynq.NewTask(AccountingAnalysisTask, []byte(`{"tenantId":"client","invoiceId":"invoice","classificationRunId":"run","analysisRunId":"analysis"}`))
	if err := handler.ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if stub.processed != 1 || stub.failed != 1 {
		t.Fatalf("processed=%d failed=%d", stub.processed, stub.failed)
	}
}

func TestAccountingAnalysisMalformedPayloadSkipsRetry(t *testing.T) {
	err := NewAccountingAnalysisHandler(&analysisProcessorStub{}).ProcessTask(context.Background(), asynq.NewTask(AccountingAnalysisTask, []byte(`{"analysisRunId":"analysis"}`)))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry, got %v", err)
	}
}

func TestAccountingAnalysisRetryableFailureIsReturnedToAsynq(t *testing.T) {
	providerErr := &accountinganalysis.ProviderError{Code: "PROVIDER_HTTP_429", Retryable: true, Err: errors.New("rate limited")}
	stub := &analysisProcessorStub{err: providerErr}
	task := asynq.NewTask(AccountingAnalysisTask, []byte(`{"tenantId":"client","invoiceId":"invoice","classificationRunId":"run","analysisRunId":"analysis"}`))
	err := NewAccountingAnalysisHandler(stub).ProcessTask(context.Background(), task)
	if !errors.Is(err, providerErr) || stub.failed != 0 {
		t.Fatalf("retryable error=%v terminal calls=%d", err, stub.failed)
	}
}
