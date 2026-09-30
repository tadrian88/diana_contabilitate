package llmusage

import (
	"time"

	"diana-contabilitate/backend/internal/money"
)

const (
	SourceLive     = "LIVE"
	SourceBackfill = "BACKFILL"

	CostPriced  = "PRICED"
	CostNoPrice = "NO_PRICE"
	CostNoUsage = "NO_USAGE"
)

// Totals aggregates calls. CostUSD sums only priced calls; UnpricedCalls and
// UnreportedCalls say how many calls it leaves out.
type Totals struct {
	Runs, Calls, FailedCalls, UnpricedCalls, UnreportedCalls            int64
	InputTokens, OutputTokens, ThoughtTokens, CachedTokens, TotalTokens int64
	CostUSD                                                             money.Amount
	IncludesBackfill                                                    bool
}

type OperationTotals struct {
	Operation string
	Totals    Totals
}

type ModelTotals struct {
	Provider, Model string
	Totals          Totals
}

type ClientSummary struct {
	ClientID    string
	Period      Period
	Totals      Totals
	ByOperation []OperationTotals
	ByModel     []ModelTotals
}

type RunSummary struct {
	RunKind RunKind
	RunID   string
	// Status is the run's own status (PROPOSED, FAILED, ...), never derived here.
	Status                string
	Label                 string
	InvoiceID, DocumentID string
	// AttemptNumber is the 1-based attempt of a contract document; 0 for analyses.
	AttemptNumber int
	FirstCallAt   time.Time
	LastCallAt    time.Time
	Models        []string
	Backfill      bool
	Totals        Totals
}

type RunPage struct {
	Items                []RunSummary
	Total, Limit, Offset int
}

type Call struct {
	Ordinal                                                                            int
	ID                                                                                 string
	OccurredAt                                                                         time.Time
	Operation, Provider, Model, Outcome, ProviderStatus, CostStatus, Source            string
	HTTPStatus                                                                         *int
	LatencyMS                                                                          *int64
	UsageReported                                                                      bool
	InputTokens, OutputTokens, ThoughtTokens, CachedTokens, ToolUseTokens, TotalTokens int64
	CostUSD                                                                            *money.Amount
}

type RunDetail struct {
	ClientID string
	Run      RunSummary
	Calls    []Call
}

type ClientTotals struct {
	ClientID, ClientName string
	Totals               Totals
}

// Overview is the account view: every client the accountant can access.
type Overview struct {
	Period  Period
	Totals  Totals
	Clients []ClientTotals
}

// Access is the set of clients an overview covers.
type Access struct {
	All       bool
	ClientIDs []string
}
