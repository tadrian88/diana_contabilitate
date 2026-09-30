package llmusage_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/llmusage"
)

type countingObserver struct{ recorded, failed, unattributed int }

func (o *countingObserver) LLMUsageRecorded()     { o.recorded++ }
func (o *countingObserver) LLMUsageRecordFailed() { o.failed++ }
func (o *countingObserver) LLMUsageUnattributed() { o.unattributed++ }

var quietLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func scopedEvent() llmusage.Event {
	return llmusage.Event{ID: "llmcall-1", Scope: llmusage.Scope{RunKind: llmusage.RunContractExtraction, RunID: "attempt-1", ClientID: "client-a"}, Outcome: llmusage.OutcomeCompleted}
}

func TestBestEffortNeverReturnsTheInnerError(t *testing.T) {
	observer := &countingObserver{}
	recorder := llmusage.BestEffort{Inner: llmusage.RecorderFunc(func(context.Context, llmusage.Event) error { return errors.New("database down") }), Logger: quietLogger, Observer: observer}
	if err := recorder.RecordLLMCall(context.Background(), scopedEvent()); err != nil {
		t.Fatal(err)
	}
	if observer.failed != 1 || observer.recorded != 0 {
		t.Fatalf("observer=%+v", observer)
	}
}

func TestBestEffortCountsCallsWithoutScopeAndSkipsTheStore(t *testing.T) {
	observer := &countingObserver{}
	called := false
	recorder := llmusage.BestEffort{Inner: llmusage.RecorderFunc(func(context.Context, llmusage.Event) error { called = true; return nil }), Logger: quietLogger, Observer: observer}
	if err := recorder.RecordLLMCall(context.Background(), llmusage.Event{ID: "llmcall-2"}); err != nil || called || observer.unattributed != 1 {
		t.Fatalf("err=%v called=%v observer=%+v", err, called, observer)
	}
}

func TestBestEffortRecordsAfterTheCallerWasCancelled(t *testing.T) {
	observer := &countingObserver{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var innerErr error
	var deadline time.Time
	recorder := llmusage.BestEffort{Inner: llmusage.RecorderFunc(func(inner context.Context, _ llmusage.Event) error {
		innerErr = inner.Err()
		deadline, _ = inner.Deadline()
		return nil
	}), Logger: quietLogger, Observer: observer}
	_ = recorder.RecordLLMCall(ctx, scopedEvent())
	if innerErr != nil || deadline.IsZero() || observer.recorded != 1 {
		t.Fatalf("inner context err=%v deadline=%v observer=%+v", innerErr, deadline, observer)
	}
}

func TestScopeRequiresKnownKindAndRunID(t *testing.T) {
	if _, ok := llmusage.ScopeFrom(context.Background()); ok {
		t.Fatal("empty context has no scope")
	}
	if _, ok := llmusage.ScopeFrom(llmusage.WithScope(context.Background(), llmusage.Scope{RunKind: "OTHER", RunID: "x"})); ok {
		t.Fatal("unknown run kind accepted")
	}
	scope := llmusage.Scope{RunKind: llmusage.RunAccountingAnalysis, RunID: "analysis-1", ClientID: "client-a"}
	if got, ok := llmusage.ScopeFrom(llmusage.WithScope(context.Background(), scope)); !ok || got != scope {
		t.Fatalf("scope=%+v ok=%v", got, ok)
	}
}

func TestNormalizeProviderAndModel(t *testing.T) {
	if llmusage.NormalizeProvider(" gemini ") != "GEMINI" || llmusage.NormalizeModel(" Models/Gemini-3.8-Flash ") != "gemini-3.8-flash" {
		t.Fatal("normalization changed")
	}
}
