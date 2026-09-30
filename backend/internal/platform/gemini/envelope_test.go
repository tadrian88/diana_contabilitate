package gemini_test

import (
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/platform/gemini"
)

func TestParseEnvelopeReadsEveryUsageField(t *testing.T) {
	body := `{"status":"completed","steps":[{"type":"thought","content":[{"type":"text","text":"private"}]},{"type":"model_output","content":[{"type":"text","text":"{\"a\":"},{"type":"text","text":"1}"}]}],
	"usage":{"total_input_tokens":1200,"total_output_tokens":300,"total_thought_tokens":450,"total_cached_tokens":200,"total_tool_use_tokens":7,"total_tokens":1957,
	"input_tokens_by_modality":[{"modality":"text","tokens":1200}]}}`
	envelope, err := gemini.ParseEnvelope([]byte(body))
	if err != nil || envelope.Status != "completed" || envelope.Text != `{"a":1}` {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
	usage := envelope.Usage
	if !usage.Reported || usage.InputTokens != 1200 || usage.OutputTokens != 300 || usage.ThoughtTokens != 450 || usage.CachedTokens != 200 || usage.ToolUseTokens != 7 || usage.TotalTokens != 1957 {
		t.Fatalf("usage=%+v", usage)
	}
	if envelope.InputTokens == nil || *envelope.InputTokens != 1200 || envelope.OutputTokens == nil || *envelope.OutputTokens != 300 {
		t.Fatal("legacy token pointers must mirror total_input_tokens/total_output_tokens")
	}
	if !strings.Contains(string(usage.Detail), "input_tokens_by_modality") || strings.Contains(string(usage.Detail), "private") {
		t.Fatalf("detail must keep the usage object only: %s", usage.Detail)
	}
}

func TestParseEnvelopeKeepsUsageWhenEnvelopeIsUnusable(t *testing.T) {
	for name, body := range map[string]string{
		"failed status":   `{"status":"failed","usage":{"total_input_tokens":10,"total_output_tokens":0}}`,
		"incomplete":      `{"status":"incomplete","steps":[],"usage":{"total_input_tokens":10,"total_output_tokens":4}}`,
		"malformed steps": `{"status":"completed","steps":"not-an-array","usage":{"total_input_tokens":10,"total_output_tokens":4}}`,
	} {
		t.Run(name, func(t *testing.T) {
			envelope, _ := gemini.ParseEnvelope([]byte(body))
			if !envelope.Usage.Reported || envelope.Usage.InputTokens != 10 {
				t.Fatalf("usage lost: %+v", envelope.Usage)
			}
		})
	}
}

func TestParseEnvelopeWithoutUsage(t *testing.T) {
	for _, body := range []string{`{"status":"completed","steps":[]}`, `{"status":"completed","usage":null}`, `not json`, ``} {
		envelope, _ := gemini.ParseEnvelope([]byte(body))
		if envelope.Usage.Reported || envelope.InputTokens != nil || envelope.OutputTokens != nil || envelope.Usage.Detail != nil {
			t.Fatalf("body %q reported usage %+v", body, envelope.Usage)
		}
	}
}

func TestParseEnvelopeDerivesMissingTotalAndClampsNegatives(t *testing.T) {
	envelope, _ := gemini.ParseEnvelope([]byte(`{"status":"completed","usage":{"total_input_tokens":10,"total_output_tokens":5,"total_thought_tokens":3,"total_cached_tokens":-4}}`))
	if envelope.Usage.TotalTokens != 18 || envelope.Usage.CachedTokens != 0 {
		t.Fatalf("usage=%+v", envelope.Usage)
	}
}

func TestParseEnvelopeDropsOversizedDetail(t *testing.T) {
	body := `{"status":"completed","usage":{"total_input_tokens":1,"total_output_tokens":1,"pad":"` + strings.Repeat("x", llmusage.MaxDetailBytes) + `"}}`
	envelope, _ := gemini.ParseEnvelope([]byte(body))
	if !envelope.Usage.Reported || envelope.Usage.Detail != nil {
		t.Fatalf("oversized detail kept: reported=%v len=%d", envelope.Usage.Reported, len(envelope.Usage.Detail))
	}
}
