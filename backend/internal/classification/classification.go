package classification

import (
	"diana-contabilitate/backend/internal/accounting"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/rules"
)

const BaselinePolicyVersion = "MODULE5_BASELINE_V1"

type Dimension string

const (
	DimensionVATreatment      Dimension = "VAT_TREATMENT"
	DimensionVATDeductibility Dimension = "VAT_DEDUCTIBILITY"
	DimensionExpenseTax       Dimension = "EXPENSE_TAX_TREATMENT"
	DimensionAccount          Dimension = "ACCOUNT"
	DimensionVAT              Dimension = "VAT"
	DimensionDeductibility    Dimension = "DEDUCTIBILITY"
)

var Dimensions = []Dimension{DimensionAccount, DimensionVAT, DimensionDeductibility}

type ReviewStatus string

const (
	ReviewPending   ReviewStatus = "PENDING"
	ReviewAccepted  ReviewStatus = "ACCEPTED"
	ReviewCorrected ReviewStatus = "CORRECTED"
	ReviewRejected  ReviewStatus = "REJECTED"
)

type Source string

const (
	SourceRule           Source = "RULE"
	SourceNoMatch        Source = "NO_MATCH"
	SourceAmbiguous      Source = "AMBIGUOUS"
	SourceLearnedMapping Source = "LEARNED_MAPPING"
	SourceAIProposal     Source = "AI_PROPOSAL"
	// SourceProfile marks a decision derived deterministically from the
	// approved client accounting profile (see Profile.ProfileDerivedExpenseTaxValue).
	SourceProfile Source = "PROFILE"
)

type MappingReference struct {
	MappingID            string `json:"mappingId"`
	Version              int    `json:"version"`
	AccountCode          string `json:"accountCode"`
	ServiceIdentityKind  string `json:"serviceIdentityKind"`
	ServiceIdentityValue string `json:"serviceIdentityValue"`
	NormalizerVersion    string `json:"normalizerVersion"`
	Revision             uint64 `json:"revision"`
}

// MappingScopePreview is the exact scope the backend will persist when the
// accountant explicitly chooses to reuse an ACCOUNT decision.
type MappingScopePreview struct {
	ClientDisplay        string `json:"clientDisplay"`
	SupplierDisplay      string `json:"supplierDisplay"`
	ServiceIdentityKind  string `json:"serviceIdentityKind"`
	ServiceIdentityValue string `json:"serviceIdentityValue"`
	NormalizerVersion    string `json:"normalizerVersion"`
}

type MappingCandidate struct {
	MappingReference
	Status string
}

// KnowledgeCandidate is reusable approved knowledge for the three dimensions
// which cannot be represented by the ACCOUNT-only account_mappings model.
// Candidates are tenant-filtered by the store; the matcher still verifies all
// exact invoice, line and accounting-context predicates.
type KnowledgeCandidate struct {
	ID                     string
	Version                int
	Dimension              Dimension
	Value                  accounting.Value
	NormalizedSupplierID   string
	ServiceIdentityKind    string
	ServiceIdentityValue   string
	NormalizerVersion      string
	Currency               string
	DocumentType           string
	VATRate                string
	ProfileID              string
	ProfileVersion         int
	SourceInvoiceID        string
	SourceInvoiceLineID    string
	SourceClassificationID string
	PromotedBy             string
	PromotedAt             time.Time
	Status                 string
}

type KnowledgeReference struct {
	ID                     string    `json:"id"`
	Version                int       `json:"version"`
	SourceInvoiceID        string    `json:"sourceInvoiceId"`
	SourceInvoiceLineID    string    `json:"sourceInvoiceLineId"`
	SourceClassificationID string    `json:"sourceClassificationId"`
	PromotedBy             string    `json:"promotedBy"`
	PromotedAt             time.Time `json:"promotedAt"`
}

type ReanalysisCommand struct {
	InvoiceID        string
	ClientID         string
	CommandID        string
	CorrelationID    string
	ActorID          string
	ActorDisplay     string
	ExpectedRevision uint64
}

type RuleReference struct {
	ProductionEligible bool
	RulePackVersion    string
	Provenance         *rules.Provenance
	EffectiveFrom      accountingdate.Date
	EffectiveTo        *accountingdate.Date
	RuleID             string
	RuleVersionID      string
	Reference          string
	Version            int
	Origin             rules.Scope
}

type Decision struct {
	ProposedTypedValue *accounting.Value
	ModelVersion       string
	TypedValue         *accounting.Value
	Evidence           *accounting.Evidence
	LegalCitations     []accounting.LegalCitation
	ValidationResults  []accounting.ValidationResult
	ProposalProvenance *accounting.ProposalProvenance
	ReviewReason       string
	InvoiceDateUsed    accountingdate.Date
	HumanReviewed      bool
	ID                 string
	ClientID           string
	InvoiceID          string
	InvoiceLineID      string
	LineLabel          string
	Dimension          Dimension
	ProposedValue      string
	EffectiveValue     *string
	EffectiveSource    *string
	Confidence         string
	Explanation        string
	LegalBasis         string
	Status             ReviewStatus
	Source             Source
	Rule               *RuleReference
	Mapping            *MappingReference
	Knowledge          *KnowledgeReference
	MappingScope       *MappingScopePreview
	PolicyVersion      string
	Revision           uint64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type InvoiceContext struct {
	ModelVersion          string
	SourceFacts           *accounting.SourceFacts
	Snapshot              *accounting.Snapshot
	Currency              string
	SupplierID            string
	NormalizedSupplierID  string
	IssueDate             accountingdate.Date
	DocumentType          string
	ID                    string
	ClientID              string
	PipelineStatus        string
	Revision              uint64
	Lines                 []LineContext
	Rules                 []RuleCandidate
	Mappings              []MappingCandidate
	Knowledge             []KnowledgeCandidate
	SelectableAccounts    map[string]bool
	ContextBlocker        string
	ContextBlockerMessage string
}

type LineContext struct {
	SourceFacts *accounting.LineFacts
	VATRate     money.Amount
	VATValue    money.Amount
	ID          string
	Position    int
	Description string
}

type RuleCandidate struct {
	ProductionEligible bool
	RulePackVersion    string
	Provenance         *rules.Provenance
	EffectiveFrom      accountingdate.Date
	EffectiveTo        *accountingdate.Date
	RuleID             string
	RuleVersionID      string
	Reference          string
	Version            int
	Category           rules.Category
	Scope              rules.Scope
	ParentRuleID       *string
	Result             string
	Explanation        string
	LegalBasis         string
	MatchKind          rules.MatchKind
	MatchValue         *string
}

type Proposal struct {
	ModelVersion      string
	TypedValue        *accounting.Value
	Evidence          *accounting.Evidence
	ReviewReason      string
	InvoiceDateUsed   accountingdate.Date
	InvoiceLineID     string
	Dimension         Dimension
	ProposedValue     string
	Confidence        string
	Explanation       string
	LegalBasis        string
	RequiresReview    bool
	Source            Source
	Rule              *RuleReference
	Mapping           *MappingReference
	Knowledge         *KnowledgeReference
	KnowledgeConflict bool
}

type Result struct {
	ModelVersion     string
	Snapshot         *accounting.Snapshot
	PolicyVersion    string
	Proposals        []Proposal
	DeferReviewForAI bool
}

func (r Result) NeedsAI() bool {
	if r.ModelVersion != accounting.ModelVersion {
		return false
	}
	for _, proposal := range r.Proposals {
		if proposal.RequiresReview && (proposal.Source == SourceNoMatch || proposal.Source == SourceAmbiguous && !proposal.KnowledgeConflict) {
			return true
		}
	}
	return false
}

type ProcessCommand struct {
	InvoiceID        string
	ExpectedRevision uint64
	CommandID        string
	CorrelationID    string
}

type ReviewCommand struct {
	TypedValue                     *accounting.Value
	Reason                         string
	InvoiceID                      string
	TaskID                         string
	ClassificationID               string
	ExpectedInvoiceRevision        uint64
	ExpectedTaskRevision           uint64
	ExpectedClassificationRevision uint64
	CorrectedValue                 *string
	CommandID                      string
	ActorID                        string
	ActorDisplay                   string
	CorrelationID                  string
	MappingAction                  string
	ExpectedMappingRevision        uint64
	Action                         string
}

type ExpectedClassification struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
}

type ApproveAllCommand struct {
	InvoiceID               string
	TaskID                  string
	ExpectedInvoiceRevision uint64
	ExpectedTaskRevision    uint64
	Expected                []ExpectedClassification
	CommandID               string
	ActorID                 string
	ActorDisplay            string
	CorrelationID           string
}

var ErrStaleReview = errors.New("stale classification review")
var ErrInvalidPolicyResult = errors.New("invalid classification policy result")
