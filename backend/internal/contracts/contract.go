package contracts

import (
	"errors"
	"time"

	"diana-contabilitate/backend/internal/money"
)

const BaselinePolicyVersion = "MODULE4_BASELINE_V1"

type Contract struct {
	ID                    string
	ClientID              string
	SupplierName          string
	SupplierCUI           string
	NormalizedSupplierCUI string
	Reference             string
	EffectiveFrom         time.Time
	EffectiveTo           *time.Time
	PeriodType            string
	Value                 money.Money
	HasLegacyTotalValue   bool
	UnitType              string
	PaymentTerms          string
	SourceReference       *string
	SourceMetadata        *string
	SourceDocumentID      *string
	ExtractionAttemptID   *string
	Revision              uint64
	LifecycleState        string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ServiceTerms          []ServiceTerm
}

type ServiceTerm struct {
	ID                 string
	Position           int
	ServiceDescription string
	PricingModel       string
	UnitPrice          *money.Amount
	Currency           string
	Unit               string
	QuantitySource     string
	QuantityValue      *money.Amount
	QuantityDriver     string
	BillingFrequency   string
	EvidenceJSON       []byte
}

type AssociationSnapshot struct {
	ContractID       string
	Reference        string
	SupplierName     string
	EffectiveFrom    time.Time
	EffectiveTo      *time.Time
	Value            money.Money
	UnitType         string
	PaymentTerms     string
	PolicyVersion    string
	AssociationKind  AssociationKind
	AssociatedAt     time.Time
	AssociatedByID   *string
	AssociatedByName *string
}

type AssociationKind string

const (
	AssociationAutomatic AssociationKind = "AUTOMATIC"
	AssociationHuman     AssociationKind = "HUMAN_CONFIRMED"
)

type MatchOutcome string

const (
	OutcomeUniqueCompatible   MatchOutcome = "UNIQUE_COMPATIBLE"
	OutcomeMultiplePlausible  MatchOutcome = "MULTIPLE_PLAUSIBLE"
	OutcomeUniqueIncompatible MatchOutcome = "UNIQUE_INCOMPATIBLE"
	OutcomeNoMatch            MatchOutcome = "NO_MATCH"
)

type Compatibility string

const (
	Compatible   Compatibility = "COMPATIBLE"
	Incompatible Compatibility = "INCOMPATIBLE"
)

type Candidate struct {
	ContractID       string
	ContractRevision uint64
	Rank             int
	Recommended      bool
	Compatibility    Compatibility
	Confidence       string
	Reasons          []string
}

type MatchDecision struct {
	Outcome       MatchOutcome
	PolicyVersion string
	Candidates    []Candidate
}

type Filter struct {
	ClientID string
	Query    string
}

type AssociatedInvoice struct {
	ID             string
	ClientID       string
	SupplierName   string
	DocumentNumber string
	IssueDate      time.Time
	Total          money.Money
	SPVReference   string
	PipelineStatus string
	SagaStatus     string
}

type InvoiceContext struct {
	ID                    string
	ClientID              string
	NormalizedSupplierCUI *string
	IssueDay              time.Time
	Currency              string
	PipelineStatus        string
	Revision              uint64
}

type BlockedInvoice struct {
	ID       string
	Revision uint64
}

type ResumeOutcome string

const (
	ResumeAutomaticallyAssociated ResumeOutcome = "AUTOMATICALLY_RESUMED"
	ResumeNeedsConfirmation       ResumeOutcome = "MATCH_CONFIRMATION_REQUIRED"
	ResumeStillMissing            ResumeOutcome = "STILL_MISSING_CONTRACT"
	ResumeStaleNoop               ResumeOutcome = "STALE_NOOP"
)

type ResumeSummary struct {
	Evaluated            int
	AutomaticallyResumed int
	ConfirmationRequired int
	StillMissingContract int
	Stale                int
}

var ErrExpiredContractSemantics = errors.New("expired contract semantics require a product decision")
var ErrContractInUse = errors.New("contract has historical use")
var ErrStaleMatchResult = errors.New("stale match result")
var ErrInvalidMatchDecision = errors.New("invalid match decision")
