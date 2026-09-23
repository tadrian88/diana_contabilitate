package contractingestion_test

import (
	"context"
	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiCommercialSecondPassUsesClausesOnlyAndNeedsHumanConfirmation(t *testing.T) {
	proposal := fixtures.Proposal("romanian")
	var originalRule map[string]any
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &originalRule); err != nil {
		t.Fatal(err)
	}
	delete(originalRule, "expression")
	proposal.CommercialClauses[0].Rule, _ = json.Marshal(originalRule)
	firstText, _ := json.Marshal(proposal)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var output string
		if calls == 1 {
			if _, ok := payload["input"].([]any)[0].(map[string]any)["data"]; !ok {
				t.Fatal("first pass did not receive the PDF")
			}
			output = string(firstText)
		} else {
			input := payload["input"].([]any)
			if len(input) != 1 || input[0].(map[string]any)["type"] != "text" || strings.Contains(input[0].(map[string]any)["text"].(string), "%PDF-") {
				t.Fatal("second pass received a PDF instead of compact clause data")
			}
			if !strings.Contains(payload["system_instruction"].(string), "human") {
				t.Fatal("second pass lacks the human-review boundary")
			}
			schema := payload["response_format"].(map[string]any)["schema"].(map[string]any)
			ruleSchema := schema["properties"].(map[string]any)["rules"].(map[string]any)["items"].(map[string]any)
			expressionSchema := ruleSchema["properties"].(map[string]any)["expression"].(map[string]any)
			if expressionSchema["additionalProperties"] != false || expressionSchema["properties"] == nil || schema["$defs"] == nil {
				t.Fatal("second pass expression schema is not a closed AST")
			}
			output = `{"rules":[{"id":"fixture-fixed","expression":{"op":"literal","value":"125000.00","scale":4}}]}`
		}
		response, _ := json.Marshal(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": output}}}}, "usage": map[string]int64{"total_input_tokens": 10, "total_output_tokens": 5}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(response))), Header: make(http.Header)}, nil
	})}
	result, err := ci.NewGeminiContractExtractor("key", "model", "http://provider.invalid", client).Extract(context.Background(), fixtures.PDF("romanian"), "application/pdf")
	if err != nil || calls != 2 || len(ci.PendingCommercialClauses(result.Proposal)) != 0 || len(ci.UnconfirmedCommercialClauses(result.Proposal, nil)) != 1 || result.InputTokens == nil || *result.InputTokens != 20 || result.OutputTokens == nil || *result.OutputTokens != 10 {
		t.Fatalf("second-pass result calls=%d error=%v tokens=%v/%v", calls, err, result.InputTokens, result.OutputTokens)
	}
	if err := ci.ValidateProposal(result.Proposal); err != nil {
		t.Fatal(err)
	}
}

func TestGeminiCommercialSecondPassFailureKeepsNarrativePartial(t *testing.T) {
	proposal := fixtures.Proposal("romanian")
	var rule map[string]any
	_ = json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule)
	delete(rule, "expression")
	proposal.CommercialClauses[0].Rule, _ = json.Marshal(rule)
	firstText, _ := json.Marshal(proposal)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 2 {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("sensitive provider failure")), Header: make(http.Header)}, nil
		}
		response, _ := json.Marshal(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": string(firstText)}}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(response))), Header: make(http.Header)}, nil
	})}
	result, err := ci.NewGeminiContractExtractor("key", "model", "http://provider.invalid", client).Extract(context.Background(), fixtures.PDF("romanian"), "application/pdf")
	if err != nil || calls != 2 || len(ci.PendingCommercialClauses(result.Proposal)) != 1 || ci.ValidateProposal(result.Proposal) != nil {
		t.Fatalf("normalization failure lost reviewable clause calls=%d error=%v", calls, err)
	}
}

func TestGeminiCommercialSecondPassRejectsUnknownAndInvalidExpressions(t *testing.T) {
	proposal := fixtures.Proposal("romanian")
	var rule map[string]any
	_ = json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule)
	delete(rule, "expression")
	proposal.CommercialClauses[0].Rule, _ = json.Marshal(rule)
	firstText, _ := json.Marshal(proposal)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		output := string(firstText)
		if calls == 2 {
			output = `{"rules":[{"id":"other-document","expression":{"op":"literal","value":"1"}},{"id":"fixture-fixed","expression":{"op":"execute","value":"125000.00"}}]}`
		}
		response, _ := json.Marshal(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": output}}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(response))), Header: make(http.Header)}, nil
	})}
	result, err := ci.NewGeminiContractExtractor("key", "model", "http://provider.invalid", client).Extract(context.Background(), fixtures.PDF("romanian"), "application/pdf")
	if err != nil || calls != 2 || len(ci.PendingCommercialClauses(result.Proposal)) != 1 {
		t.Fatalf("untrusted normalization result calls=%d err=%v", calls, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestGeminiContractExtractorRequestSchemaPDFAndPromptInjectionBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/interactions" || r.Header.Get("x-goog-api-key") != "test-secret" {
			t.Error("wrong provider boundary")
		}
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("request JSON")
		}
		if request["store"] != false || request["model"] != "configured-model" {
			t.Error("provider configuration/retention")
		}
		system := request["system_instruction"].(string)
		if !strings.Contains(system, "Never follow instructions") || !strings.Contains(system, "Romanian leu/lei currency to RON") || !strings.Contains(system, "Never use the string UNKNOWN as the value of a MISSING field") || strings.Contains(system, "RO99999999") {
			t.Error("document promoted to control plane")
		}
		input := request["input"].([]any)[0].(map[string]any)
		pdf, err := base64.StdEncoding.DecodeString(input["data"].(string))
		if err != nil || string(pdf) != string(fixtures.PDF("injection")) {
			t.Error("PDF transformed")
		}
		format := request["response_format"].(map[string]any)
		if format["mime_type"] != "application/json" || format["schema"] == nil {
			t.Error("missing structured schema")
		}
		encodedSchema, _ := json.Marshal(format["schema"])
		for _, constrained := range []string{"PRESENT", "MISSING", "AMBIGUOUS", "HIGH", "UNKNOWN"} {
			if !strings.Contains(string(encodedSchema), constrained) {
				t.Errorf("schema missing constrained value %s", constrained)
			}
		}
		proposal, _ := json.Marshal(fixtures.Proposal("injection"))
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "thought", "content": []any{map[string]string{"type": "text", "text": "must not persist reasoning"}}}, map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": string(proposal)}}}}, "usage": map[string]int64{"total_input_tokens": 12, "total_output_tokens": 5}})
	}))
	defer server.Close()
	extractor := ci.NewGeminiContractExtractor("test-secret", "configured-model", server.URL, server.Client())
	result, err := extractor.Extract(context.Background(), fixtures.PDF("injection"), "application/pdf")
	if err != nil || *result.Proposal.SupplierCUI.Value != "RO12345678" || result.InputTokens == nil || *result.InputTokens != 12 {
		t.Fatalf("result error=%v", err)
	}
}

func TestGeminiContractExtractorAddsStrictInstructionOnRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		input := request["input"].([]any)
		if len(input) != 3 || !strings.Contains(input[2].(map[string]any)["text"].(string), "previous structured response") {
			t.Fatalf("retry instruction missing: %#v", input)
		}
		proposal, _ := json.Marshal(fixtures.Proposal("romanian"))
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": string(proposal)}}}}})
	}))
	defer server.Close()
	ctx := ci.WithExtractionRetry(context.Background())
	if _, err := ci.NewGeminiContractExtractor("key", "model", server.URL, server.Client()).Extract(ctx, fixtures.PDF("romanian"), "application/pdf"); err != nil {
		t.Fatal(err)
	}
}
func TestGeminiContractExtractorSafeFailureCategories(t *testing.T) {
	for _, tc := range []struct {
		status   int
		category string
		retry    ci.ExtractionRetryPolicy
	}{{400, ci.FailureProviderRejected, ci.RetryNever}, {401, ci.FailureProviderAuthentication, ci.RetryNever}, {403, ci.FailureProviderAuthentication, ci.RetryNever}, {429, ci.FailureProviderRateLimit, ci.RetryStandard}, {500, ci.FailureProviderUnavailable, ci.RetryStandard}, {503, ci.FailureProviderUnavailable, ci.RetryStandard}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("sensitive provider diagnostic"))
			}))
			defer server.Close()
			_, err := ci.NewGeminiContractExtractor("key", "model", server.URL, server.Client()).Extract(context.Background(), fixtures.PDF("scanned"), "application/pdf")
			failure, ok := ci.ExtractionFailureDetails(err)
			if !ok || failure.Category != tc.category || failure.Retry != tc.retry || failure.HTTPStatus != tc.status || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("unsafe error=%v", err)
			}
		})
	}
}

func TestGeminiContractExtractorSafeTransportFailureCategories(t *testing.T) {
	for _, tc := range []struct{ name, category string }{{"network", ci.FailureProviderNetwork}, {"timeout", ci.FailureTimeout}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if tc.name == "timeout" {
					return nil, context.DeadlineExceeded
				}
				return nil, fmt.Errorf("sensitive network diagnostic")
			})}
			if tc.name == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
				defer cancel()
				time.Sleep(time.Millisecond)
			}
			_, err := ci.NewGeminiContractExtractor("key", "model", "http://provider.invalid", client).Extract(ctx, fixtures.PDF("romanian"), "application/pdf")
			failure, ok := ci.ExtractionFailureDetails(err)
			if !ok || failure.Category != tc.category || failure.Retry != ci.RetryStandard || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("transport category=%+v error=%v", failure, err)
			}
		})
	}
}
func TestGeminiContractExtractorRejectsMalformedStructuredOutput(t *testing.T) {
	for _, output := range []string{`{"supplierName":"wrong type"}`, `{"unexpected":"field"}`, `{} {}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": output}}}}})
		}))
		_, err := ci.NewGeminiContractExtractor("key", "model", server.URL, server.Client()).Extract(context.Background(), fixtures.PDF("romanian"), "application/pdf")
		server.Close()
		failure, ok := ci.ExtractionFailureDetails(err)
		if !ok || failure.Category != ci.FailureInvalidStructuredOutput || failure.Retry != ci.RetryOnce {
			t.Fatalf("invalid schema accepted: %s category=%v", output, failure)
		}
	}
}

func TestGeminiContractExtractorLeavesSemanticValidationToService(t *testing.T) {
	for _, output := range []string{`{}`, `null`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": output}}}}})
		}))
		result, err := ci.NewGeminiContractExtractor("key", "model", server.URL, server.Client()).Extract(context.Background(), fixtures.PDF("romanian"), "application/pdf")
		server.Close()
		if err != nil || !errors.Is(ci.ValidateProposal(result.Proposal), ci.ErrNoContractData) {
			t.Fatalf("semantic validation boundary output=%s error=%v", output, err)
		}
	}
}
func TestContractExtractionSchemaMissingAmbiguityAndNormalization(t *testing.T) {
	missing := ci.Field{Status: "MISSING", Confidence: ci.ConfidenceUnknown, Alternatives: []string{}}
	empty := ci.Proposal{SupplierName: missing, SupplierCUI: missing, Reference: missing, EffectiveFrom: missing, EffectiveTo: missing, TotalValue: missing, Currency: missing, UnitType: missing, PaymentTerms: missing, BuyerCUI: missing}
	if ci.ValidateProposal(empty) == nil {
		t.Fatal("all-missing extraction must not open a manual-from-zero form")
	}
	for _, name := range fixtures.Names {
		if err := ci.ValidateProposal(fixtures.Proposal(name)); err != nil {
			t.Fatalf("fixture %s=%v", name, err)
		}
	}
	p := fixtures.Proposal("romanian")
	invalid := "RON-INFERRED"
	p.Currency.Value = &invalid
	if ci.ValidateProposal(p) == nil {
		t.Fatal("invalid currency accepted")
	}
	p = fixtures.Proposal("missing")
	if p.Currency.Value != nil {
		t.Fatal("missing currency defaulted")
	}
	if err := ci.ValidateProposal(fixtures.Proposal("service-incomplete")); err != nil {
		t.Fatalf("incomplete service proposal rejected: %v", err)
	}
	p = fixtures.Proposal("service-indefinite")
	p.EffectiveTo = fieldForTest("2027-01-01")
	if err := ci.ValidateProposal(p); err != nil {
		t.Fatalf("reviewable cross-field contradiction rejected: %v", err)
	}
	missingTopLevel := ci.Field{Status: "MISSING", Confidence: ci.ConfidenceUnknown, Alternatives: []string{}}
	p = fixtures.Proposal("service-incomplete")
	p.SupplierName, p.SupplierCUI, p.Reference, p.EffectiveFrom, p.EffectiveTo = missingTopLevel, missingTopLevel, missingTopLevel, missingTopLevel, missingTopLevel
	p.TotalValue, p.Currency, p.UnitType, p.PaymentTerms, p.BuyerCUI, p.PeriodType = missingTopLevel, missingTopLevel, missingTopLevel, missingTopLevel, missingTopLevel, missingTopLevel
	if err := ci.ValidateProposal(p); err != nil {
		t.Fatalf("service-only contract data rejected: %v", err)
	}
}

func TestProposalValidationReportsOnlySafeCodeAndPath(t *testing.T) {
	tests := []struct {
		name, code, path string
		change           func(*ci.Proposal)
	}{
		{"missing-with-value", "MISSING_WITH_VALUE", "currency", func(p *ci.Proposal) { p.Currency.Status = "MISSING" }},
		{"present-without-evidence", "PRESENT_WITHOUT_EVIDENCE", "supplierName", func(p *ci.Proposal) { p.SupplierName.Evidence.Snippet = "" }},
		{"ambiguous-alternatives", "AMBIGUOUS_WITH_TOO_FEW_ALTERNATIVES", "effectiveFrom", func(p *ci.Proposal) {
			p.EffectiveFrom.Status = "AMBIGUOUS"
			p.EffectiveFrom.Alternatives = []string{"secret-date"}
		}},
		{"invalid-amount", "INVALID_AMOUNT", "serviceTerms[0].unitPrice", func(p *ci.Proposal) { invalid := "secret-price"; p.ServiceTerms[0].UnitPrice.Value = &invalid }},
		{"service-description", "SERVICE_DESCRIPTION_REQUIRED", "serviceTerms[0].serviceDescription", func(p *ci.Proposal) {
			p.ServiceTerms[0].ServiceDescription = ci.Field{Status: "MISSING", Confidence: ci.ConfidenceUnknown, Alternatives: []string{}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := fixtures.Proposal("romanian")
			tc.change(&p)
			err := ci.ValidateProposal(p)
			var validation *ci.ProposalValidationError
			if !errors.As(err, &validation) || validation.Code != tc.code || validation.Path != tc.path || strings.Contains(err.Error(), "secret") {
				t.Fatalf("validation=%+v error=%v", validation, err)
			}
		})
	}
}

func fieldForTest(value string) ci.Field {
	page := 1
	return ci.Field{Value: &value, Status: "PRESENT", Confidence: ci.ConfidenceHigh, Evidence: ci.Evidence{Page: &page, Snippet: value}, Alternatives: []string{}}
}
