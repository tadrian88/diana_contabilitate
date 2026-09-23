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
)

type Source string

const (
	SourceRule           Source = "RULE"
	SourceNoMatch        Source = "NO_MATCH"
	SourceAmbiguous      Source = "AMBIGUOUS"
	SourceLearnedMapping Source = "LEARNED_MAPPING"
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
	Confidence         string
	Explanation        string
	LegalBasis         string
	Status             ReviewStatus
	Source             Source
	Rule               *RuleReference
	Mapping            *MappingReference
	MappingScope       *MappingScopePreview
	PolicyVersion      string
	Revision           uint64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type InvoiceContext struct {
	ModelVersion         string
	SourceFacts          *accounting.SourceFacts
	Snapshot             *accounting.Snapshot
	Currency             string
	SupplierID           string
	NormalizedSupplierID string
	IssueDate            accountingdate.Date
	DocumentType         string
	ID                   string
	ClientID             string
	PipelineStatus       string
	Revision             uint64
	Lines                []LineContext
	Rules                []RuleCandidate
	Mappings             []MappingCandidate
	SelectableAccounts   map[string]bool
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
	ModelVersion    string
	TypedValue      *accounting.Value
	Evidence        *accounting.Evidence
	ReviewReason    string
	InvoiceDateUsed accountingdate.Date
	InvoiceLineID   string
	Dimension       Dimension
	ProposedValue   string
	Confidence      string
	Explanation     string
	LegalBasis      string
	RequiresReview  bool
	Source          Source
	Rule            *RuleReference
	Mapping         *MappingReference
}

type Result struct {
	ModelVersion  string
	Snapshot      *accounting.Snapshot
	PolicyVersion string
	Proposals     []Proposal
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
}

var ErrStaleReview = errors.New("stale classification review")
var ErrInvalidPolicyResult = errors.New("invalid classification policy result")
