package contractingestion_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/llmusage/llmusagetest"
)

var contractScope = llmusage.Scope{RunKind: llmusage.RunContractExtraction, RunID: "contractextract-1", ClientID: "client"}

func interactionResponse(status int, text string, usage map[string]int64) *http.Response {
	body := map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": text}}}}}
	if usage != nil {
		body["usage"] = usage
	}
	encoded, _ := json.Marshal(body)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(encoded))), Header: make(http.Header)}
}

func proposalWithPendingClause(t *testing.T) string {
	t.Helper()
	proposal := fixtures.Proposal("romanian")
	var rule map[string]any
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	delete(rule, "expression")
	proposal.CommercialClauses[0].Rule, _ = json.Marshal(rule)
	encoded, _ := json.Marshal(proposal)
	return string(encoded)
}

func TestGeminiContractExtractionRecordsExtractionAndNormalizationCalls(t *testing.T) {
	first := proposalWithPendingClause(t)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return interactionResponse(200, first, map[string]int64{"total_input_tokens": 1000, "total_output_tokens": 200, "total_thought_tokens": 300}), nil
		}
		return interactionResponse(200, `{"rules":[{"id":"fixture-fixed","expression":{"op":"literal","value":"125000.00","scale":4}}]}`, map[string]int64{"total_input_tokens": 50, "total_output_tokens": 10, "total_thought_tokens": 5}), nil
	})}
	recorder := &llmusagetest.MemoryRecorder{}
	result, err := ci.NewGeminiContractExtractor("key", "gemini-3.8-flash", "http://provider.invalid", client).WithUsageRecorder(recorder).
		Extract(llmusage.WithScope(context.Background(), contractScope), fixtures.PDF("romanian"), "application/pdf")
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	events := recorder.Events()
	if len(events) != 2 || events[0].Operation != llmusage.OperationContractExtraction || events[1].Operation != llmusage.OperationContractClauseNormalization {
		t.Fatalf("events=%+v", events)
	}
	for _, event := range events {
		if event.Scope != contractScope || event.Outcome != llmusage.OutcomeCompleted || !event.Usage.Reported {
			t.Fatalf("event=%+v", event)
		}
	}
	if events[0].Usage.ThoughtTokens != 300 || events[1].Usage.ThoughtTokens != 5 {
		t.Fatal("thinking tokens must be recorded per call")
	}
	// The legacy attempt columns keep their previous meaning: input/output only.
	if *result.InputTokens != 1050 || *result.OutputTokens != 210 {
		t.Fatalf("legacy tokens=%d/%d", *result.InputTokens, *result.OutputTokens)
	}
}

func TestGeminiContractExtractionRecordsBilledFailures(t *testing.T) {
	usage := map[string]int64{"total_input_tokens": 900, "total_output_tokens": 40}
	for _, tc := range []struct {
		name     string
		response func(call int) *http.Response
		calls    int
		outcomes []string
		wantErr  bool
	}{
		{"invalid structured output", func(int) *http.Response { return interactionResponse(200, `{"unexpected":"field"}`, usage) }, 1, []string{llmusage.OutcomeCompleted}, true},
		{"rate limited", func(int) *http.Response { return interactionResponse(429, "", nil) }, 1, []string{llmusage.OutcomeHTTPError}, true},
		{"normalization unavailable", func(call int) *http.Response {
			if call == 1 {
				return interactionResponse(200, proposalWithPendingClause(t), usage)
			}
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("down")), Header: make(http.Header)}
		}, 2, []string{llmusage.OutcomeCompleted, llmusage.OutcomeHTTPError}, false},
		{"normalization invalid output", func(call int) *http.Response {
			if call == 1 {
				return interactionResponse(200, proposalWithPendingClause(t), usage)
			}
			return interactionResponse(200, `not json`, usage)
		}, 2, []string{llmusage.OutcomeCompleted, llmusage.OutcomeCompleted}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return tc.response(calls), nil
			})}
			recorder := &llmusagetest.MemoryRecorder{}
			_, err := ci.NewGeminiContractExtractor("key", "model", "http://provider.invalid", client).WithUsageRecorder(recorder).
				Extract(llmusage.WithScope(context.Background(), contractScope), fixtures.PDF("romanian"), "application/pdf")
			if (err != nil) != tc.wantErr || calls != tc.calls {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			events := recorder.Events()
			if len(events) != len(tc.outcomes) {
				t.Fatalf("events=%+v", events)
			}
			for index, outcome := range tc.outcomes {
				if events[index].Outcome != outcome || events[index].Scope != contractScope {
					t.Fatalf("event %d=%+v", index, events[index])
				}
			}
		})
	}
}

type scopeCapturingExtractor struct{ scope llmusage.Scope }

func (*scopeCapturingExtractor) Provider() string { return "FAKE" }
func (*scopeCapturingExtractor) Model() string    { return "fake-v1" }
func (e *scopeCapturingExtractor) Extract(ctx context.Context, _ []byte, _ string) (ci.ExtractionResult, error) {
	e.scope, _ = llmusage.ScopeFrom(ctx)
	return ci.ExtractionResult{Proposal: fixtures.Proposal("romanian")}, nil
}

func TestContractExtractionBillsProviderCallsToTheAttemptAndClient(t *testing.T) {
	store := &memoryStore{}
	extractor := &scopeCapturingExtractor{}
	service := ci.NewService(store, extractor, &availability{}, 0, nil)
	doc, _, err := service.Upload(context.Background(), ci.Upload{ClientID: "client", Filename: "contract.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
	if err != nil {
		t.Fatal(err)
	}
	store.doc.MIMEType = "application/pdf"
	if err = service.Extract(context.Background(), doc.ID); err != nil {
		t.Fatal(err)
	}
	if store.doc.LatestExtractionID == nil || extractor.scope != (llmusage.Scope{RunKind: llmusage.RunContractExtraction, RunID: *store.doc.LatestExtractionID, ClientID: "client"}) {
		t.Fatalf("scope=%+v attempt=%v", extractor.scope, store.doc.LatestExtractionID)
	}
}
