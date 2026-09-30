// Package llmusage records the tokens and the estimated cost of every model
// call, attributed to the AI run (accounting analysis or contract extraction
// attempt) and therefore to the client that owns it.
package llmusage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Usage is the provider-reported token accounting of one call. Counts are
// never negative; Reported is false when the response carried no usage.
type Usage struct {
	InputTokens, OutputTokens, ThoughtTokens, CachedTokens, ToolUseTokens, TotalTokens int64
	Reported                                                                           bool
	// Detail is the raw provider usage object (counts and modality breakdowns
	// only, never content), kept so a later audit can re-derive a cost.
	Detail json.RawMessage
}

const MaxDetailBytes = 16 << 10

type RunKind string

const (
	RunAccountingAnalysis RunKind = "ACCOUNTING_ANALYSIS"
	RunContractExtraction RunKind = "CONTRACT_EXTRACTION"
)

func (k RunKind) Valid() bool { return k == RunAccountingAnalysis || k == RunContractExtraction }

const (
	OperationAccountingAnalysis          = "ACCOUNTING_ANALYSIS"
	OperationContractExtraction          = "CONTRACT_EXTRACTION"
	OperationContractClauseNormalization = "CONTRACT_CLAUSE_NORMALIZATION"
)

const (
	OutcomeCompleted       = "COMPLETED"
	OutcomeIncomplete      = "PROVIDER_INCOMPLETE"
	OutcomeProviderFailed  = "PROVIDER_FAILED"
	OutcomeHTTPError       = "HTTP_ERROR"
	OutcomeInvalidEnvelope = "INVALID_ENVELOPE"
	OutcomeTransportError  = "TRANSPORT_ERROR"
)

// Scope attributes provider calls to the run that caused them. The service
// owning the run sets it before invoking a provider adapter.
type Scope struct {
	RunKind  RunKind
	RunID    string
	ClientID string
}

func (s Scope) Valid() bool { return s.RunKind.Valid() && strings.TrimSpace(s.RunID) != "" }

type scopeContextKey struct{}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeContextKey{}, scope)
}

func ScopeFrom(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeContextKey{}).(Scope)
	return scope, ok && scope.Valid()
}

// Event is one provider call, successful or not.
type Event struct {
	ID             string
	OccurredAt     time.Time
	Scope          Scope
	Provider       string
	Model          string
	Operation      string
	Outcome        string
	HTTPStatus     int
	ProviderStatus string
	Latency        time.Duration
	Usage          Usage
}

func NewEventID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return "llmcall-" + hex.EncodeToString(raw[:])
}

func NormalizeProvider(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func NormalizeModel(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "models/")
}

var ErrRunNotFound = errors.New("llm usage run not found")
