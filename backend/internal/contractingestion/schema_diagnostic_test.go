package contractingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestSchemaDiagnostic(t *testing.T) {
	if os.Getenv("GEMINI_SCHEMA_DIAGNOSTIC_LIVE") != "1" {
		t.Skip("live Gemini schema diagnostic is opt-in; set GEMINI_SCHEMA_DIAGNOSTIC_LIVE=1")
	}
	for _, name := range []string{"GEMINI_API_BASE_URL", "GEMINI_API_KEY", "GEMINI_CONTRACT_MODEL"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s must be set for the live Gemini schema diagnostic", name)
		}
	}
	scalar := map[string]any{
		"id": map[string]any{"type": "string"}, "kind": map[string]any{"type": "string"},
		"narrative": map[string]any{"type": "string"}, "dateBasis": map[string]any{"type": "string"},
		"currency": map[string]any{"type": "string"}, "blocking": map[string]any{"type": "boolean"},
	}
	applicability := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"serviceId": map[string]any{"type": "string"}, "skus": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "aliases": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "documentTypes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "billingFrequency": map[string]any{"type": "string"}, "contractYear": map[string]any{"type": []string{"integer", "null"}}, "tranche": map[string]any{"type": []string{"integer", "null"}},
	}, "required": []string{"serviceId", "skus", "aliases", "documentTypes", "billingFrequency", "contractYear", "tranche"}}
	evidence := map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"documentId": map[string]any{"type": "string"}, "page": map[string]any{"type": []string{"integer", "null"}}, "snippet": map[string]any{"type": "string"}}, "required": []string{"documentId", "page", "snippet"}}}
	expression := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"op": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}, "variable": map[string]any{"type": "string"}, "scale": map[string]any{"type": "integer"}, "args": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/expression"}}, "tiers": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"upTo": map[string]any{"type": []string{"string", "null"}}, "value": map[string]any{"type": "string"}}, "required": []string{"upTo", "value"}}}}, "required": []string{"op", "value", "variable", "args", "tiers", "scale"}}
	for _, variant := range []string{"scalar", "applicability", "evidence", "expression"} {
		schema := proposalJSONSchema()
		defs := schema["$defs"].(map[string]any)
		props := map[string]any{}
		for k, v := range scalar {
			props[k] = v
		}
		required := []string{"id", "kind", "narrative", "dateBasis", "currency", "blocking"}
		if variant == "applicability" || variant == "evidence" || variant == "expression" {
			props["applicability"] = applicability
			required = append(required, "applicability")
		}
		if variant == "evidence" || variant == "expression" {
			props["evidence"] = evidence
			props["requiredVariables"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
			required = append(required, "evidence", "requiredVariables")
		}
		if variant == "expression" {
			defs["expression"] = expression
			props["expression"] = map[string]any{"$ref": "#/$defs/expression"}
			required = append(required, "expression")
		}
		defs["rule"] = map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
		payload := map[string]any{"model": os.Getenv("GEMINI_CONTRACT_MODEL"), "store": false, "input": "Return an empty contract extraction.", "response_format": map[string]any{"type": "text", "mime_type": "application/json", "schema": schema}}
		body, _ := json.Marshal(payload)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, os.Getenv("GEMINI_API_BASE_URL")+"/interactions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", os.Getenv("GEMINI_API_KEY"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		cancel()
		if resp.StatusCode >= 300 {
			t.Logf("variant=%s status=%d body=%s", variant, resp.StatusCode, b)
		} else {
			t.Logf("variant=%s status=%d", variant, resp.StatusCode)
		}
	}
}
