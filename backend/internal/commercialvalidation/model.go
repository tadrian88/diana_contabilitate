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
	EngineVersion     = "COMMERCIAL_VALIDATION_V2"
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
	// Origin is server-owned provenance. RuleOriginSourceText marks a rule
	// completed deterministically from its cited clause and auto-confirmed.
	Origin string `json:"origin,omitempty"`
	// Unit is the contractual unit of measure of a priced service ("lună",
	// "oră"). It is descriptive only and never changes the calculation.
	Unit string `json:"unit,omitempty"`
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

// LearnedAlias is a confirmed invoice wording → contractual service
// association as a reviewer sees it. InvoiceID is set when the association
// applies to one invoice only; revoked associations remain as history.
type LearnedAlias struct {
	ID              string     `json:"id"`
	DossierID       string     `json:"dossierId"`
	ServiceID       string     `json:"serviceId"`
	ServiceLabel    string     `json:"serviceLabel,omitempty"`
	NormalizedLabel string     `json:"normalizedLabel"`
	InvoiceID       string     `json:"invoiceId,omitempty"`
	ConfirmedBy     string     `json:"confirmedBy"`
	ConfirmedAt     time.Time  `json:"confirmedAt"`
	RevokedAt       *time.Time `json:"revokedAt,omitempty"`
	RevokedBy       string     `json:"revokedBy,omitempty"`
}

type AliasRevocation struct {
	ClientID, AliasID, ActorID, ActorDisplay, CommandID string
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

// ServiceCandidate is one contractual service a reviewer may map an uncovered
// invoice line to. Ranking signals are wording and unit of measure only; the
// tariff is never a signal (see suggest.go).
type ServiceCandidate struct {
	RuleID           string     `json:"ruleId"`
	Label            string     `json:"label"`
	PricingKind      RuleKind   `json:"pricingKind,omitempty"`
	Unit             string     `json:"unit,omitempty"`
	BillingFrequency string     `json:"billingFrequency,omitempty"`
	Evidence         []Evidence `json:"evidence,omitempty"`
	Score            float64    `json:"score"`
	SharedWords      []string   `json:"sharedWords,omitempty"`
	LineWordCount    int        `json:"lineWordCount,omitempty"`
	LineUnit         string     `json:"lineUnit,omitempty"`
	UnitMatch        UnitMatch  `json:"unitMatch,omitempty"`
	Suggested        bool       `json:"suggested,omitempty"`
}

type UnitMatch string

const (
	UnitCompatible   UnitMatch = "COMPATIBLE"
	UnitIncompatible UnitMatch = "INCOMPATIBLE"
	UnitUnknown      UnitMatch = "UNKNOWN"
)

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
	// ActiveSnapshotVersion is read with the run, never stored: the dossier's
	// version now, so a reader sees the contract changed since the run.
	ActiveSnapshotVersion uint64 `json:"activeSnapshotVersion,omitempty"`
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
	// PendingClauses cites the dossier's clauses still to settle, which keep
	// its coverage partial, so the reviewer is sent to them.
	PendingClauses []Evidence
	// ContractWaived is set only when no contract snapshot exists and the
	// accountant chose to continue without a contract (D-120); the reasoned
	// decision is Invoice.ContractWaiver.
	ContractWaived bool
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
	// Revise replaces an already-confirmed narrative-completed rule with the
	// reviewer's correction instead of confirming a proposal.
	Revise bool
	// Automatic records a source-text recognition confirmed without a click.
	Automatic bool
}

// ServicePriceActivation reports which confirmed service prices became
// enforceable rules and which could not, so none disappears silently.
type ServicePriceActivation struct {
	Activated int                   `json:"activated"`
	Skipped   []SkippedServicePrice `json:"skipped,omitempty"`
}

type SkippedServicePrice struct {
	Position    int    `json:"position"`
	Description string `json:"description"`
	UnitPrice   string `json:"unitPrice,omitempty"`
	Currency    string `json:"currency,omitempty"`
	Reason      string `json:"reason"`
}

const (
	SkipPriceMissing            = "PRICE_MISSING"
	SkipPricingModelUnsupported = "PRICING_MODEL_UNSUPPORTED"
	SkipSourceMissing           = "SOURCE_EVIDENCE_MISSING"
	SkipPriceChangedFromSource  = "PRICE_CHANGED_FROM_SOURCE"
	SkipRuleInvalid             = "RULE_INVALID"
	SkipPriceNotInSource        = "PRICE_NOT_IN_SOURCE"
	// SkipPricedByClauses: the dossier already checks prices with rules
	// confirmed from contract clauses, so service tariffs are not added.
	SkipPricedByClauses = "PRICED_BY_CONTRACT_CLAUSES"
)

// A proposed clause closed without becoming a rule keeps why it was closed.
// ClauseCoveredByPartyIdentity: an identity clause naming the client's CUI
// (alone or next to the supplier's). ClauseCoveredByServiceTariff: a pricing
// clause restating a reviewed service tariff; only the system closes it.
const (
	ClauseCoveredByContractReference = "COVERED_BY_CONTRACT_REFERENCE"
	ClauseCoveredBySupplierIdentity  = "COVERED_BY_SUPPLIER_IDENTITY"
	ClauseCoveredByPartyIdentity     = "COVERED_BY_PARTY_IDENTITY"
	ClauseCoveredByServiceTariff     = "COVERED_BY_SERVICE_TARIFF"
	ClauseNotInvoiceVerifiable       = "NOT_INVOICE_VERIFIABLE"
)

// ClauseDismissal closes one proposed clause of a confirmed document so it no
// longer keeps the dossier's coverage partial.
type ClauseDismissal struct {
	ClientID, DocumentID, RuleID, CommandID string
	ActorID, ActorDisplay                   string
	ReasonCode                              string
	Reason                                  string
}

// DocumentCommercialState is what a confirmed contract document contributes
// to invoice checks right now, read from the dossier's active snapshot.
type DocumentCommercialState struct {
	DossierID       string                 `json:"dossierId"`
	SnapshotVersion uint64                 `json:"snapshotVersion"`
	Coverage        Coverage               `json:"coverage"`
	ActiveRuleIDs   []string               `json:"activeRuleIds"`
	Services        []DocumentServiceState `json:"services"`
	Clauses         []DocumentClauseState  `json:"clauses"`
}

type DocumentServiceState struct {
	Position   int    `json:"position"`
	RuleID     string `json:"ruleId,omitempty"`
	Active     bool   `json:"active"`
	SkipReason string `json:"skipReason,omitempty"`
}

type DocumentClauseState struct {
	RuleID     string     `json:"ruleId"`
	Kind       RuleKind   `json:"kind"`
	Status     string     `json:"status"`
	ReasonCode string     `json:"reasonCode,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	ReviewedBy string     `json:"reviewedBy,omitempty"`
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`
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
	ListAliases(context.Context, string, string) ([]LearnedAlias, error)
	DocumentDossierID(context.Context, string, string) (string, error)
	RevokeAlias(context.Context, AliasRevocation, time.Time) (bool, error)
	ConfirmProposedRule(context.Context, RuleConfirmation, time.Time) (bool, error)
	DismissProposedClause(context.Context, ClauseDismissal, time.Time) (bool, error)
	DocumentCommercialState(context.Context, string, string) (*DocumentCommercialState, error)
	ActivateReviewedServicePrices(context.Context, string, string, string, string, time.Time) (ServicePriceActivation, error)
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
