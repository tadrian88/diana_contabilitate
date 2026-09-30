// Package gemini is the shared transport for the Gemini Interactions API. It
// records the usage of every call before callers interpret the response.
package gemini

import (
	"bytes"
	"encoding/json"
	"strings"

	"diana-contabilitate/backend/internal/llmusage"
)

// Envelope is the provider-neutral part of an interaction response. Text is
// the concatenated model_output text; thought steps are never read.
type Envelope struct {
	Status string
	Text   string
	Usage  llmusage.Usage
	// InputTokens and OutputTokens keep the legacy nil-when-absent values of
	// total_input_tokens and total_output_tokens stored on the run tables.
	InputTokens, OutputTokens *int64
}

// ParseEnvelope decodes the response. Usage is decoded independently, so it
// survives a malformed or failed envelope; the error covers only the envelope.
func ParseEnvelope(body []byte) (Envelope, error) {
	var envelope Envelope
	envelope.Usage, envelope.InputTokens, envelope.OutputTokens = parseUsage(body)
	var decoded struct {
		Status string `json:"status"`
		Steps  []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return envelope, err
	}
	envelope.Status = decoded.Status
	var text strings.Builder
	for _, step := range decoded.Steps {
		if step.Type == "model_output" {
			for _, part := range step.Content {
				if part.Type == "text" {
					text.WriteString(part.Text)
				}
			}
		}
	}
	envelope.Text = text.String()
	return envelope, nil
}

func parseUsage(body []byte) (llmusage.Usage, *int64, *int64) {
	var outer struct {
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(body, &outer) != nil || len(outer.Usage) == 0 || bytes.Equal(outer.Usage, []byte("null")) {
		return llmusage.Usage{}, nil, nil
	}
	var reported struct {
		Input   *int64 `json:"total_input_tokens"`
		Output  *int64 `json:"total_output_tokens"`
		Thought *int64 `json:"total_thought_tokens"`
		Cached  *int64 `json:"total_cached_tokens"`
		ToolUse *int64 `json:"total_tool_use_tokens"`
		Total   *int64 `json:"total_tokens"`
	}
	if json.Unmarshal(outer.Usage, &reported) != nil {
		return llmusage.Usage{}, nil, nil
	}
	count := func(value *int64) int64 {
		if value == nil || *value < 0 {
			return 0
		}
		return *value
	}
	usage := llmusage.Usage{
		InputTokens: count(reported.Input), OutputTokens: count(reported.Output), ThoughtTokens: count(reported.Thought),
		CachedTokens: count(reported.Cached), ToolUseTokens: count(reported.ToolUse), TotalTokens: count(reported.Total),
		Reported: reported.Input != nil || reported.Output != nil || reported.Thought != nil || reported.Total != nil,
	}
	if reported.Total == nil {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens + usage.ThoughtTokens + usage.ToolUseTokens
	}
	var compact bytes.Buffer
	if usage.Reported && json.Compact(&compact, outer.Usage) == nil && compact.Len() <= llmusage.MaxDetailBytes && bytes.HasPrefix(compact.Bytes(), []byte("{")) {
		usage.Detail = json.RawMessage(compact.Bytes())
	}
	return usage, reported.Input, reported.Output
}
