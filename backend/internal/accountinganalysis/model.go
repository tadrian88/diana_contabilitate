// Package accountinganalysis orchestrates proposals over Diana's existing
// source facts, dated fiscal profile and approved tenant knowledge. A proposal
// is never an authority and cannot be exported before explicit review.
package accountinganalysis

import (
	"context"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

const SchemaVersion = "ACCOUNTING_ANALYSIS_V1"
const PromptVersion = "ACCOUNTING_ANALYSIS_PROMPT_V1"

type Direction string

const (
	Incoming Direction = "INCOMING"
	Outgoing Direction = "OUTGOING"
)

type Phase string

const (
	InvoicePhase    Phase = "INVOICE"
	PaymentPhase    Phase = "PAYMENT"
	CollectionPhase Phase = "COLLECTION"
)

type Confidence string

const (
	ConfidenceHigh    Confidence = "HIGH"
	ConfidenceMedium  Confidence = "MEDIUM"
	ConfidenceLow     Confidence = "LOW"
	ConfidenceUnknown Confidence = "UNKNOWN"
)

type Source string

const (
	SourceApprovedRule      Source = "APPROVED_RULE"
	SourceDeterministicRule Source = "DETERMINISTIC_RULE"
	SourceLLMLegislation    Source = "LLM_LEGISLATION_ANALYSIS"
	SourceManual            Source = "MANUAL"
)

type Line struct {
	ID, Description string
	Facts           *accounting.LineFacts
	Net, VAT, Gross money.Amount
}

type Input struct {
	ClientID, InvoiceID      string
	SupplierID, SupplierName string
	InvoiceRevision          uint64
	IssueDate                accountingdate.Date
	Direction                Direction
	Currency                 string
	Total                    money.Amount
	SourceFacts              *accounting.SourceFacts
	Profile                  *accounting.Profile
	Lines                    []Line
}

type Entry struct {
	Phase          Phase        `json:"phase"`
	DebitAccount   string       `json:"debitAccount"`
	CreditAccount  string       `json:"creditAccount"`
	Amount         money.Amount `json:"amount"`
	Currency       string       `json:"currency"`
	InvoiceLineIDs []string     `json:"invoiceLineIds"`
	Explanation    string       `json:"explanation"`
}

type LineTreatment struct {
	InvoiceLineID          string        `json:"invoiceLineId"`
	VATKind                string        `json:"vatKind"`
	VATRate                *money.Amount `json:"vatRate,omitempty"`
	VATBase                money.Amount  `json:"vatBase"`
	VATAmount              money.Amount  `json:"vatAmount"`
	VATTiming              string        `json:"vatTiming"`
	VATAccount             string        `json:"vatAccount,omitempty"`
	VATDeductibility       string        `json:"vatDeductibility"`
	ExpenseDeductibility   string        `json:"expenseDeductibility"`
	DeductibilityCondition string        `json:"deductibilityCondition,omitempty"`
}

type Citation struct {
	FragmentID  string `json:"fragmentId"`
	VersionID   string `json:"versionId"`
	CitationKey string `json:"citationKey"`
	ContentHash string `json:"contentHash"`
}

type Proposal struct {
	SchemaVersion    string          `json:"schemaVersion"`
	ClientID         string          `json:"clientId"`
	InvoiceID        string          `json:"invoiceId"`
	InvoiceRevision  uint64          `json:"invoiceRevision"`
	Entries          []Entry         `json:"entries"`
	LineTreatments   []LineTreatment `json:"lineTreatments"`
	Citations        []Citation      `json:"citations"`
	ReasoningSummary string          `json:"reasoningSummary"`
	Confidence       Confidence      `json:"confidence"`
	Source           Source          `json:"source"`
	RequiresReview   bool            `json:"requiresReview"`
}

type AnalysisRequest struct {
	Input             Input
	Fragments         []legislation.Fragment
	ApprovedKnowledge []ApprovedKnowledge
}

type ApprovedKnowledge struct {
	ID, ClientID, SupplierID, SemanticKind, SemanticValue string
	ProfileID                                             string
	LegislationVersionIDs                                 []string
	EffectiveFrom                                         accountingdate.Date
	EffectiveTo                                           *accountingdate.Date
	Proposal                                              Proposal
}

type ProviderResult struct {
	Proposal                  Proposal
	Provider, Model           string
	InputTokens, OutputTokens *int64
}

type Analyzer interface {
	Analyze(context.Context, AnalysisRequest) (ProviderResult, error)
}
