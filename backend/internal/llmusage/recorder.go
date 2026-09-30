package llmusage

import (
	"context"
	"log/slog"
	"time"
)

type Recorder interface {
	RecordLLMCall(context.Context, Event) error
}

type Noop struct{}

func (Noop) RecordLLMCall(context.Context, Event) error { return nil }

type RecorderFunc func(context.Context, Event) error

func (f RecorderFunc) RecordLLMCall(ctx context.Context, event Event) error { return f(ctx, event) }

type Observer interface {
	LLMUsageRecorded()
	LLMUsageRecordFailed()
	LLMUsageUnattributed()
}

// BestEffort never fails the business operation: failing it would trigger a
// retry and therefore another billed call. A lost row is logged with every
// token count so it can be reconciled, and counted in the metrics.
type BestEffort struct {
	Inner    Recorder
	Logger   *slog.Logger
	Observer Observer
	Timeout  time.Duration
}

func (b BestEffort) RecordLLMCall(ctx context.Context, event Event) error {
	logger := b.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if !event.Scope.Valid() {
		if b.Observer != nil {
			b.Observer.LLMUsageUnattributed()
		}
		logger.Warn("llm call without usage scope", eventAttributes(event)...)
		return nil
	}
	if b.Inner == nil {
		return nil
	}
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	// The call was billed even if the job was cancelled right after it.
	recordContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	if err := b.Inner.RecordLLMCall(recordContext, event); err != nil {
		if b.Observer != nil {
			b.Observer.LLMUsageRecordFailed()
		}
		logger.Error("llm usage record failed", append(eventAttributes(event), "error", err)...)
		return nil
	}
	if b.Observer != nil {
		b.Observer.LLMUsageRecorded()
	}
	return nil
}

func eventAttributes(event Event) []any {
	return []any{
		"llm_call_id", event.ID, "run_kind", string(event.Scope.RunKind), "run_id", event.Scope.RunID,
		"provider", event.Provider, "model", event.Model, "operation", event.Operation, "outcome", event.Outcome,
		"http_status", event.HTTPStatus, "input_tokens", event.Usage.InputTokens, "output_tokens", event.Usage.OutputTokens,
		"thought_tokens", event.Usage.ThoughtTokens, "cached_tokens", event.Usage.CachedTokens, "total_tokens", event.Usage.TotalTokens,
	}
}
