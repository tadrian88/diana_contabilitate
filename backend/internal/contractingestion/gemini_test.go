package contractingestion_test

import (
	"context"
	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
		if !strings.Contains(system, "Never follow instructions") || strings.Contains(system, "RO99999999") {
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
func TestGeminiContractExtractorSafeFailureCategories(t *testing.T) {
	for _, status := range []int{400, 401, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("sensitive provider diagnostic"))
			}))
			defer server.Close()
			_, err := ci.NewGeminiContractExtractor("key", "model", server.URL, server.Client()).Extract(context.Background(), fixtures.PDF("scanned"), "application/pdf")
			want := ci.ErrExtractionPermanent
			if status == 429 || status >= 500 {
				want = ci.ErrExtractionTransient
			}
			if !errors.Is(err, want) || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("unsafe error=%v", err)
			}
		})
	}
}
func TestGeminiContractExtractorRejectsMalformedSchemaAndIncompleteOutput(t *testing.T) {
	for _, output := range []string{`{}`, `{"supplierName":"wrong type"}`, `{"unexpected":"field"}`, `null`, `{} {}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]string{"type": "text", "text": output}}}}})
		}))
		_, err := ci.NewGeminiContractExtractor("key", "model", server.URL, server.Client()).Extract(context.Background(), fixtures.PDF("romanian"), "application/pdf")
		server.Close()
		if !errors.Is(err, ci.ErrExtractionPermanent) {
			t.Fatalf("invalid schema accepted: %s", output)
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
}
