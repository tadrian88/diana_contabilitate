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

const SchemaVersion = "UNIFIED_ACCOUNTING_PROPOSAL_V2"
const PromptVersion = "UNIFIED_ACCOUNTING_PROMPT_V2"
const LegacySchemaVersion = "ACCOUNTING_ANALYSIS_V1"

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
	SourceLLMLegislation    Source = "LLM_LEGISLATION_ANALYSIS" // legacy artifact only
	SourceAIProposal        Source = "AI_PROPOSAL"
	SourceManual            Source = "MANUAL"
)

type Line struct {
	ID                   string                `json:"id"`
	Description          string                `json:"description"`
	Facts                *accounting.LineFacts `json:"facts,omitempty"`
	Net                  money.Amount          `json:"net"`
	VAT                  money.Amount          `json:"vat"`
	Gross                money.Amount          `json:"gross"`
	UnresolvedDimensions []string              `json:"unresolvedDimensions"`
}

type Input struct {
	ClientID            string                  `json:"clientId"`
	InvoiceID           string                  `json:"invoiceId"`
	ClassificationRunID string                  `json:"classificationRunId"`
	SupplierID          string                  `json:"supplierId"`
	SupplierName        string                  `json:"supplierName"`
	InvoiceRevision     uint64                  `json:"invoiceRevision"`
	IssueDate           accountingdate.Date     `json:"issueDate"`
	Direction           Direction               `json:"direction"`
	Currency            string                  `json:"currency"`
	Total               money.Amount            `json:"total"`
	SourceFacts         *accounting.SourceFacts `json:"sourceFacts"`
	Profile             *accounting.Profile     `json:"profile"`
	AccountCatalog      Catalog                 `json:"accountCatalog,omitempty"`
	AccountCandidates   []AccountCandidate      `json:"accountCandidates"`
	ResolvedDimensions  []ResolvedDimension     `json:"resolvedDimensions"`
	Lines               []Line                  `json:"lines"`
}

type AccountCandidate struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	AccountType string `json:"accountType"`
}

type ResolvedDimension struct {
	InvoiceLineID string           `json:"invoiceLineId"`
	Dimension     string           `json:"dimension"`
	Value         accounting.Value `json:"value"`
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

func (c Citation) Evidence(verified bool) accounting.LegalCitation {
	return accounting.LegalCitation{FragmentID: c.FragmentID, VersionID: c.VersionID, CitationKey: c.CitationKey, ContentHash: c.ContentHash, Verified: verified}
}

type DimensionProposal struct {
	Dimension     string           `json:"dimension"`
	ProposedValue accounting.Value `json:"proposedValue"`
	Explanation   string           `json:"explanation"`
	Citations     []Citation       `json:"citations"`
	Confidence    Confidence       `json:"confidence"`
	Insufficient  bool             `json:"insufficient"`
}

type LineProposal struct {
	InvoiceLineID string              `json:"invoiceLineId"`
	Decisions     []DimensionProposal `json:"decisions"`
}

type Proposal struct {
	SchemaVersion string         `json:"schemaVersion"`
	Lines         []LineProposal `json:"lines,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Source        Source         `json:"source"`

	// V1 fields are retained only so historical immutable runs remain readable.
	ClientID         string          `json:"clientId,omitempty"`
	InvoiceID        string          `json:"invoiceId,omitempty"`
	InvoiceRevision  uint64          `json:"invoiceRevision,omitempty"`
	Entries          []Entry         `json:"entries,omitempty"`
	LineTreatments   []LineTreatment `json:"lineTreatments,omitempty"`
	Citations        []Citation      `json:"citations,omitempty"`
	ReasoningSummary string          `json:"reasoningSummary,omitempty"`
	Confidence       Confidence      `json:"confidence,omitempty"`
	RequiresReview   bool            `json:"requiresReview,omitempty"`
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

func HasDimensionsNeedingAI(input Input) bool {
	for _, line := range input.Lines {
		if len(line.UnresolvedDimensions) > 0 {
			return true
		}
	}
	return false
}
