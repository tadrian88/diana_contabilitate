package accountinganalysis_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/llmusage/llmusagetest"
)

var analysisScope = llmusage.Scope{RunKind: llmusage.RunAccountingAnalysis, RunID: "analysis-1", ClientID: "client-a"}

func TestGeminiAnalyzerRecordsUsageOnEveryProviderAnswer(t *testing.T) {
	usage := `"usage":{"total_input_tokens":4000,"total_output_tokens":600,"total_thought_tokens":900}`
	for _, tc := range []struct {
		name, body, code, outcome string
		status                    int
	}{
		{"proposal", `{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"{}"}]}],` + usage + `}`, "", llmusage.OutcomeCompleted, 200},
		{"invalid schema", `{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"not json"}]}],` + usage + `}`, "INVALID_PROVIDER_SCHEMA", llmusage.OutcomeCompleted, 200},
		{"invalid envelope", `{"status":"failed",` + usage + `}`, "INVALID_PROVIDER_RESPONSE", llmusage.OutcomeProviderFailed, 200},
		{"rate limited", `{` + usage + `}`, "PROVIDER_HTTP_429", llmusage.OutcomeHTTPError, 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			recorder := &llmusagetest.MemoryRecorder{}
			result, err := accountinganalysis.NewGeminiAnalyzer("key", "gemini-3.8-flash", server.URL, server.Client()).WithUsageRecorder(recorder).
				Analyze(llmusage.WithScope(context.Background(), analysisScope), accountinganalysis.AnalysisRequest{})
			if (tc.code == "" && err != nil) || (tc.code != "" && accountinganalysis.FailureCode(err) != tc.code) {
				t.Fatalf("err=%v", err)
			}
			events := recorder.Events()
			if len(events) != 1 || events[0].Outcome != tc.outcome || events[0].Scope != analysisScope || events[0].Operation != llmusage.OperationAccountingAnalysis ||
				!events[0].Usage.Reported || events[0].Usage.ThoughtTokens != 900 {
				t.Fatalf("events=%+v", events)
			}
			if tc.code == "" && (result.InputTokens == nil || *result.InputTokens != 4000 || *result.OutputTokens != 600) {
				t.Fatal("legacy run tokens changed meaning")
			}
		})
	}
}

func TestGeminiAnalyzerNetworkFailureStaysRetryableAndRecorded(t *testing.T) {
	recorder := &llmusagetest.MemoryRecorder{}
	client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("dial failed") })}
	_, err := accountinganalysis.NewGeminiAnalyzer("key", "m", "http://provider.invalid", client).WithUsageRecorder(recorder).
		Analyze(llmusage.WithScope(context.Background(), analysisScope), accountinganalysis.AnalysisRequest{})
	if accountinganalysis.FailureCode(err) != "PROVIDER_NETWORK" || !accountinganalysis.IsRetryable(err) {
		t.Fatalf("err=%v", err)
	}
	if events := recorder.Events(); len(events) != 1 || events[0].Outcome != llmusage.OutcomeTransportError {
		t.Fatalf("events=%+v", events)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type executionStore struct {
	accountinganalysis.WorkflowStore
	execution accountinganalysis.Execution
}

func (s executionStore) LoadAnalysisExecution(context.Context, string) (accountinganalysis.Execution, error) {
	return s.execution, nil
}

type scopeCapturingAnalyzer struct{ scope llmusage.Scope }

func (a *scopeCapturingAnalyzer) Analyze(ctx context.Context, _ accountinganalysis.AnalysisRequest) (accountinganalysis.ProviderResult, error) {
	a.scope, _ = llmusage.ScopeFrom(ctx)
	return accountinganalysis.ProviderResult{}, &accountinganalysis.ProviderError{Code: "PROVIDER_HTTP_429", Retryable: true, Err: errors.New("rate limited")}
}

func TestWorkflowBillsEveryAttemptToTheAnalysisRun(t *testing.T) {
	profile := &accounting.Profile{ID: "profile-a", ClientID: "client-a", Version: 1, EffectiveFrom: "2026-01-01", Framework: "OMFP_1802_2014", AccountCodes: []string{"626"},
		TaxRegime: "PROFIT_TAX", VATRegistration: "ORDINARY_REGISTERED", DeductionActivity: "WITH_DEDUCTION_RIGHT", CashAccounting: "NO", ProRata: "NO",
		Approval: accounting.Approval{Actor: "TEST_ONLY", At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Evidence: []string{"TEST_ONLY"}}}
	execution := accountinganalysis.Execution{
		Run:   accountinganalysis.Run{ID: "analysis-1", ClientID: "client-a", InvoiceID: "invoice-a", ClassificationRunID: "run-a", Status: "RUNNING"},
		Input: accountinganalysis.Input{ClientID: "client-a", InvoiceID: "invoice-a", ClassificationRunID: "run-a", InvoiceRevision: 1, IssueDate: "2026-09-01", Profile: profile, Lines: []accountinganalysis.Line{{ID: "line-1", Description: "Servicii", UnresolvedDimensions: []string{"ACCOUNT"}}}},
	}
	analyzer := &scopeCapturingAnalyzer{}
	service := accountinganalysis.NewWorkflowService(executionStore{execution: execution}, nil, analyzer, "gemini", "m", nil)
	for attempt := 0; attempt < 2; attempt++ {
		analyzer.scope = llmusage.Scope{}
		err := service.Process(context.Background(), accountinganalysis.AnalysisJob{TenantID: "client-a", InvoiceID: "invoice-a", ClassificationRunID: "run-a", AnalysisRunID: "analysis-1"})
		if accountinganalysis.FailureCode(err) != "PROVIDER_HTTP_429" {
			t.Fatalf("attempt %d err=%v", attempt, err)
		}
		if analyzer.scope != analysisScope {
			t.Fatalf("attempt %d scope=%+v", attempt, analyzer.scope)
		}
	}
}
