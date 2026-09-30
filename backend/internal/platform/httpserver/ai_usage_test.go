package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

type aiUsageReaderStub struct{ access llmusage.Access }

func sampleTotals() llmusage.Totals {
	return llmusage.Totals{Runs: 2, Calls: 3, FailedCalls: 1, InputTokens: 5000, OutputTokens: 700, ThoughtTokens: 900, TotalTokens: 6600, CostUSD: money.Amount("0.0071625"), IncludesBackfill: true}
}

func (r *aiUsageReaderStub) LLMUsageClientSummary(_ context.Context, clientID string, period llmusage.Period) (llmusage.ClientSummary, error) {
	return llmusage.ClientSummary{ClientID: clientID, Period: period, Totals: sampleTotals(), ByOperation: []llmusage.OperationTotals{{Operation: llmusage.OperationAccountingAnalysis, Totals: sampleTotals()}}}, nil
}
func (r *aiUsageReaderStub) LLMUsageRuns(_ context.Context, _ string, _ llmusage.Period, limit, offset int) (llmusage.RunPage, error) {
	run := llmusage.RunSummary{RunKind: llmusage.RunContractExtraction, RunID: "attempt-1", Status: "SUCCEEDED", Label: "contract.pdf", DocumentID: "doc-1", AttemptNumber: 2, FirstCallAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), LastCallAt: time.Date(2026, 9, 1, 8, 1, 0, 0, time.UTC), Models: []string{"gemini-3.8-flash"}, Totals: sampleTotals()}
	return llmusage.RunPage{Items: []llmusage.RunSummary{run}, Total: 1, Limit: limit, Offset: offset}, nil
}
func (r *aiUsageReaderStub) LLMUsageRunCalls(_ context.Context, clientID string, kind llmusage.RunKind, runID string) (llmusage.RunDetail, error) {
	if runID != "attempt-1" || kind != llmusage.RunContractExtraction {
		return llmusage.RunDetail{}, apperrors.ErrNotFound
	}
	status := 429
	return llmusage.RunDetail{ClientID: clientID, Run: llmusage.RunSummary{RunKind: kind, RunID: runID, Totals: sampleTotals()}, Calls: []llmusage.Call{{Ordinal: 1, ID: "llmcall-1", Outcome: llmusage.OutcomeHTTPError, HTTPStatus: &status, CostStatus: llmusage.CostNoUsage, Source: llmusage.SourceLive}}}, nil
}
func (r *aiUsageReaderStub) LLMUsageOverview(_ context.Context, access llmusage.Access, period llmusage.Period) (llmusage.Overview, error) {
	r.access = access
	return llmusage.Overview{Period: period, Totals: sampleTotals(), Clients: []llmusage.ClientTotals{{ClientID: "a", ClientName: "Alfa", Totals: sampleTotals()}}}, nil
}

func aiUsageHandler(reader llmusage.Reader) http.Handler {
	var service *llmusage.Service
	if reader != nil {
		service = llmusage.NewService(reader)
	}
	return NewWithAIUsage(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, service, readyStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
}

func aiUsageCall(t *testing.T, handler http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request = request.WithContext(requestactor.WithActor(request.Context(), requestactor.Actor{ID: "accountant-a", Display: "A", Persona: "CONTABIL", AuthorizedClientIDs: []string{"a"}}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var body map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	return response, body
}

func TestAIUsageOverviewIsTheCurrentAccountOnly(t *testing.T) {
	reader := &aiUsageReaderStub{}
	response, body := aiUsageCall(t, aiUsageHandler(reader), "/api/v1/ai-usage?from=2026-09-01&to=2026-09-30")
	if response.Code != http.StatusOK || reader.access.All || len(reader.access.ClientIDs) != 1 || reader.access.ClientIDs[0] != "a" {
		t.Fatalf("status=%d access=%+v body=%s", response.Code, reader.access, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"cost":{"amount":0.0071625,"currency":"USD"}`) {
		t.Fatalf("cost must be an exact JSON number in USD: %s", response.Body)
	}
	period := body["period"].(map[string]any)
	if period["from"] != "2026-09-01" || period["to"] != "2026-09-30" || period["timeZone"] != "Europe/Bucharest" {
		t.Fatalf("period=%v", period)
	}
	assertNoClientSecrets(t, body)
}

func TestAIUsageClientRoutesEnforceGrantsAndValidation(t *testing.T) {
	handler := aiUsageHandler(&aiUsageReaderStub{})
	for path, want := range map[string]int{
		"/api/v1/clients/a/ai-usage":                                         http.StatusOK,
		"/api/v1/clients/b/ai-usage":                                         http.StatusNotFound,
		"/api/v1/clients/b/ai-usage/runs":                                    http.StatusNotFound,
		"/api/v1/clients/b/ai-usage/runs/contract-extraction/attempt-1":      http.StatusNotFound,
		"/api/v1/clients/a/ai-usage?from=2026-13-01":                         http.StatusBadRequest,
		"/api/v1/clients/a/ai-usage?from=2026-09-10&to=2026-09-01":           http.StatusBadRequest,
		"/api/v1/clients/a/ai-usage/runs?limit=-1":                           http.StatusBadRequest,
		"/api/v1/clients/a/ai-usage/runs?offset=x":                           http.StatusBadRequest,
		"/api/v1/clients/a/ai-usage/runs?limit=10&offset=0":                  http.StatusOK,
		"/api/v1/clients/a/ai-usage/runs/contract-extraction/attempt-1":      http.StatusOK,
		"/api/v1/clients/a/ai-usage/runs/unknown-kind/attempt-1":             http.StatusNotFound,
		"/api/v1/clients/a/ai-usage/runs/accounting-analysis/missing-run-id": http.StatusNotFound,
	} {
		response, body := aiUsageCall(t, handler, path)
		if response.Code != want {
			t.Fatalf("%s status=%d want=%d body=%s", path, response.Code, want, response.Body)
		}
		assertNoClientSecrets(t, body)
	}
}

func TestAIUsageRunDTOs(t *testing.T) {
	handler := aiUsageHandler(&aiUsageReaderStub{})
	_, page := aiUsageCall(t, handler, "/api/v1/clients/a/ai-usage/runs?limit=5")
	items := page["items"].([]any)
	run := items[0].(map[string]any)
	if page["limit"] != float64(5) || run["runKind"] != "CONTRACT_EXTRACTION" || run["attemptNumber"] != float64(2) || run["documentId"] != "doc-1" || run["firstCallAt"] != "2026-09-01T08:00:00Z" {
		t.Fatalf("page=%v", page)
	}
	_, detail := aiUsageCall(t, handler, "/api/v1/clients/a/ai-usage/runs/contract-extraction/attempt-1")
	call := detail["calls"].([]any)[0].(map[string]any)
	if call["httpStatus"] != float64(429) || call["cost"] != nil || call["costStatus"] != "NO_USAGE" || call["latencyMs"] != nil {
		t.Fatalf("call=%v", call)
	}
}

func TestAIUsageDisabledWithoutService(t *testing.T) {
	response, _ := aiUsageCall(t, aiUsageHandler(nil), "/api/v1/ai-usage")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", response.Code)
	}
}
