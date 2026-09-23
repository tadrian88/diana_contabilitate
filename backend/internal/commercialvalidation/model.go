package commercialvalidation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
)

const (
	RuleSchemaVersion = "COMMERCIAL_RULE_V1"
	EngineVersion     = "COMMERCIAL_VALIDATION_V1"
)

type Outcome string

const (
	Conform      Outcome = "CONFORM"
	Nonconform   Outcome = "NECONFORM"
	Unverifiable Outcome = "NEVERIFICABIL"
)

type Coverage string

const (
	CoverageComplete   Coverage = "COMPLETE"
	CoveragePartial    Coverage = "PARTIAL"
	CoverageConflicted Coverage = "CONFLICTED"
)

type DocumentRole string

const (
	RoleBaseContract DocumentRole = "BASE_CONTRACT"
	RoleAnnex        DocumentRole = "ANNEX"
	RoleAmendment    DocumentRole = "AMENDMENT"
	RoleSOW          DocumentRole = "SOW"
	RoleOrder        DocumentRole = "ORDER"
	RolePriceList    DocumentRole = "PRICE_LIST"
	RoleOther        DocumentRole = "OTHER"
)

type RuleKind string

const (
	RuleIdentity          RuleKind = "IDENTITY"
	RuleContractReference RuleKind = "CONTRACT_REFERENCE"
	RuleFixedPrice        RuleKind = "FIXED_PRICE"
	RuleUnitRate          RuleKind = "UNIT_RATE"
	RuleTieredPrice       RuleKind = "TIERED_PRICE"
	RuleDiscount          RuleKind = "DISCOUNT"
	RuleTranche           RuleKind = "TRANCHE"
	RuleProrata           RuleKind = "PRORATA"
	RuleMinimum           RuleKind = "MINIMUM"
	RuleMaximum           RuleKind = "MAXIMUM"
	RuleCostPlus          RuleKind = "COST_PLUS"
	RuleFX                RuleKind = "FX"
	RuleVAT               RuleKind = "VAT"
	RulePaymentDue        RuleKind = "PAYMENT_DUE"
	RuleFrequency         RuleKind = "FREQUENCY"
	RuleCreditNote        RuleKind = "CREDIT_NOTE"
)

type DateBasis string

const (
	DateInvoiceIssue DateBasis = "INVOICE_ISSUE_DATE"
	DateServiceStart DateBasis = "SERVICE_PERIOD_START"
	DateServiceEnd   DateBasis = "SERVICE_PERIOD_END"
	DateReceipt      DateBasis = "RECEIPT_DATE"
	DateAcceptance   DateBasis = "ACCEPTANCE_DATE"
	DateAnniversary  DateBasis = "CONTRACT_ANNIVERSARY"
)

// Expression is a closed, data-only calculation tree. Op is validated before
// evaluation; arbitrary code and provider-generated expression strings are not
// executable.
type Expression struct {
	Op       string       `json:"op"`
	Value    string       `json:"value,omitempty"`
	Variable string       `json:"variable,omitempty"`
	Args     []Expression `json:"args,omitempty"`
	Tiers    []Tier       `json:"tiers,omitempty"`
	Scale    int          `json:"scale,omitempty"`
}

type Tier struct {
	UpTo  *string `json:"upTo,omitempty"`
	Value string  `json:"value"`
}

type Applicability struct {
	ServiceID        string   `json:"serviceId,omitempty"`
	SKUs             []string `json:"skus,omitempty"`
	Aliases          []string `json:"aliases,omitempty"`
	DocumentTypes    []string `json:"documentTypes,omitempty"`
	BillingFrequency string   `json:"billingFrequency,omitempty"`
	ContractYear     *int     `json:"contractYear,omitempty"`
	Tranche          *int     `json:"tranche,omitempty"`
}

type Evidence struct {
	DocumentID string `json:"documentId"`
	Page       *int   `json:"page,omitempty"`
	Snippet    string `json:"snippet"`
}

type Rule struct {
	ID                string        `json:"id"`
	Kind              RuleKind      `json:"kind"`
	Narrative         string        `json:"narrative"`
	Applicability     Applicability `json:"applicability"`
	DateBasis         DateBasis     `json:"dateBasis"`
	Currency          string        `json:"currency,omitempty"`
	Expression        *Expression   `json:"expression,omitempty"`
	RequiredVariables []string      `json:"requiredVariables,omitempty"`
	Evidence          []Evidence    `json:"evidence"`
	Blocking          bool          `json:"blocking"`
}

type Snapshot struct {
	ID, DossierID, ContractID string
	Version                   uint64
	SchemaVersion             string
	Coverage                  Coverage
	EffectiveFrom             time.Time
	EffectiveTo               *time.Time
	Rules                     []Rule
	ConfirmedByID             string
	ConfirmedAt               time.Time
}

type VariableValue struct {
	Name, Value, Source, SourceReference string
	PeriodStart, PeriodEnd               *time.Time
	RecordedByID                         string
	RecordedAt                           time.Time
}

type Alias struct {
	ID, ClientID, InvoiceID, LineID, DossierID, SupplierCUI, ServiceID, NormalizedLabel string
	ReuseForDossier                                                                     bool
	EffectiveFrom                                                                       *time.Time
	EffectiveTo                                                                         *time.Time
	ConfirmedByID                                                                       string
	ConfirmedAt                                                                         time.Time
}

type Finding struct {
	ID                string             `json:"id"`
	RuleID            string             `json:"ruleId"`
	Code              string             `json:"code"`
	Outcome           Outcome            `json:"outcome"`
	LineID            string             `json:"lineId,omitempty"`
	Actual            string             `json:"actual,omitempty"`
	ActualSource      string             `json:"actualSource,omitempty"`
	Expected          string             `json:"expected,omitempty"`
	Calculation       string             `json:"calculation,omitempty"`
	Reason            string             `json:"reason"`
	MissingInputs     []string           `json:"missingInputs,omitempty"`
	ServiceCandidates []ServiceCandidate `json:"serviceCandidates,omitempty"`
	Evidence          []Evidence         `json:"evidence,omitempty"`
	Override          *Override          `json:"override,omitempty"`
}

type ServiceCandidate struct {
	RuleID string `json:"ruleId"`
	Label  string `json:"label"`
}

type Override struct {
	Reason       string    `json:"reason"`
	ActorID      string    `json:"actorId"`
	ActorDisplay string    `json:"actorDisplay,omitempty"`
	At           time.Time `json:"at"`
}

type Run struct {
	ID              string    `json:"id"`
	InvoiceID       string    `json:"invoiceId"`
	SnapshotID      string    `json:"snapshotId,omitempty"`
	DossierID       string    `json:"dossierId,omitempty"`
	EngineVersion   string    `json:"engineVersion"`
	InvoiceRevision uint64    `json:"invoiceRevision"`
	SnapshotVersion uint64    `json:"snapshotVersion"`
	Outcome         Outcome   `json:"outcome"`
	Findings        []Finding `json:"findings"`
	CreatedAt       time.Time `json:"createdAt"`
	CompletedAt     time.Time `json:"completedAt"`
}

type Input struct {
	Invoice              invoicing.Invoice
	Snapshot             Snapshot
	Variables            map[string]VariableValue
	UnavailableVariables map[string]VariableValue
	Aliases              []Alias
	Original             *invoicing.Invoice
	Reference            string
	ReferenceSource      string
	ReceiptDate          *time.Time
	RemittanceDate       *time.Time
	RemittanceSource     string
	AcceptanceDate       *time.Time
}

type InvoiceDateFact struct {
	ClientID, InvoiceID, Kind, SourceReference, ActorID, CommandID string
	Date                                                           time.Time
}

type ReviewResolution struct {
	ClientID, InvoiceID, RunID, FindingID, CommandID string
	ExpectedInvoiceRevision                          uint64
	Action                                           string
	Reason, ActorID, ActorDisplay                    string
	CorrelationID                                    string
}

type RuleConfirmation struct {
	ClientID, DocumentID, RuleID, CommandID string
	ActorID, ActorDisplay                   string
	Rule                                    *Rule
}

type RevalidationCandidate struct {
	InvoiceID     string    `json:"invoiceId"`
	IssueDate     time.Time `json:"issueDate"`
	PreviousRunID string    `json:"previousRunId,omitempty"`
}

type Dossier struct {
	ID               string    `json:"id"`
	ClientID         string    `json:"clientId"`
	ContractID       string    `json:"contractId,omitempty"`
	SupplierCUI      string    `json:"supplierCui"`
	BuyerCUI         string    `json:"buyerCui"`
	PrimaryReference string    `json:"primaryReference"`
	Status           string    `json:"status"`
	ActiveSnapshotID string    `json:"activeSnapshotId,omitempty"`
	Coverage         Coverage  `json:"coverage,omitempty"`
	SnapshotVersion  uint64    `json:"snapshotVersion,omitempty"`
	Revision         uint64    `json:"revision"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type Store interface {
	LoadInput(context.Context, string) (Input, error)
	SaveRun(context.Context, Run, string) (bool, error)
	GetRun(context.Context, string, string) (*Run, error)
	Resolve(context.Context, ReviewResolution, time.Time) (bool, error)
	PutVariable(context.Context, string, string, VariableValue, string) (bool, error)
	ConfirmAlias(context.Context, Alias, string) (bool, error)
	ConfirmProposedRule(context.Context, RuleConfirmation, time.Time) (bool, error)
	ActivateReviewedServicePrices(context.Context, string, string, string, string, time.Time) (int, error)
	PutInvoiceDateFact(context.Context, InvoiceDateFact, time.Time) (bool, error)
	PreviewRevalidation(context.Context, string, string) ([]RevalidationCandidate, error)
	CreateDossier(context.Context, Dossier, string) (Dossier, bool, error)
	ListDossiers(context.Context, string) ([]Dossier, error)
	GetDossier(context.Context, string, string) (Dossier, error)
}

type Clock func() time.Time

var (
	ErrNoSnapshot        = errors.New("commercial snapshot unavailable")
	ErrAmbiguousService  = errors.New("commercial service mapping is ambiguous")
	ErrMissingOriginal   = errors.New("original invoice unavailable")
	ErrInvalidRule       = errors.New("invalid commercial rule")
	ErrInvalidExpression = errors.New("invalid commercial expression")
)

func MarshalRules(rules []Rule) json.RawMessage {
	value, _ := json.Marshal(rules)
	return value
}
