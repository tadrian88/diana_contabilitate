package invoicing

import (
	"diana-contabilitate/backend/internal/accounting"
	"time"

	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/validationtasks"
)

type PipelineStatus string

const (
	StatusDownloaded               PipelineStatus = "DOWNLOADED"
	StatusArchived                 PipelineStatus = "ARCHIVED"
	StatusMatching                 PipelineStatus = "MATCHING"
	StatusAwaitingContract         PipelineStatus = "AWAITING_CONTRACT"
	StatusAwaitingMatchConfirm     PipelineStatus = "AWAITING_MATCH_CONFIRM"
	StatusDedupeChecked            PipelineStatus = "DEDUPE_CHECKED"
	StatusHeaderRead               PipelineStatus = "HEADER_READ"
	StatusLinesRead                PipelineStatus = "LINES_READ"
	StatusCommercialValidating     PipelineStatus = "COMMERCIAL_VALIDATING"
	StatusAwaitingCommercialReview PipelineStatus = "AWAITING_COMMERCIAL_REVIEW"
	StatusCommerciallyValidated    PipelineStatus = "COMMERCIALLY_VALIDATED"
	StatusClassified               PipelineStatus = "CLASSIFIED"
	StatusAwaitingReview           PipelineStatus = "AWAITING_REVIEW"
	StatusReadyForSAGA             PipelineStatus = "READY_FOR_SAGA"
	StatusExporting                PipelineStatus = "EXPORTING"
	StatusExported                 PipelineStatus = "EXPORTED"
	StatusDuplicate                PipelineStatus = "DUPLICATE"
)

var PipelineStatuses = []PipelineStatus{
	StatusDownloaded, StatusArchived, StatusMatching, StatusAwaitingContract,
	StatusAwaitingMatchConfirm, StatusDedupeChecked, StatusHeaderRead, StatusLinesRead,
	StatusCommercialValidating, StatusAwaitingCommercialReview, StatusCommerciallyValidated,
	StatusClassified, StatusAwaitingReview, StatusReadyForSAGA, StatusExporting,
	StatusExported, StatusDuplicate,
}

type SagaStatus string

const (
	SagaNotReady  SagaStatus = "NOT_READY"
	SagaReady     SagaStatus = "READY"
	SagaExporting SagaStatus = "EXPORTING"
	SagaExported  SagaStatus = "EXPORTED"
	SagaFailed    SagaStatus = "FAILED"
)

type DocumentType string

const (
	DocumentTypeInvoice    DocumentType = "INVOICE"
	DocumentTypeCreditNote DocumentType = "CREDIT_NOTE"
)

// Direction tells whether the client received the invoice or issued it (D-124).
type Direction string

const (
	DirectionIncoming Direction = "INCOMING"
	DirectionOutgoing Direction = "OUTGOING"
)

type Filter struct {
	ClientID  string
	Direction Direction
}

type Invoice struct {
	ModelVersion               string
	SourceFacts                *accounting.SourceFacts
	AccountingSnapshot         *accounting.Snapshot
	ReadinessReason            string
	CurrentClassificationRunID *string
	AccountingWorkflowStatus   string
	ClassificationContext      *ClassificationContext
	ID                         string
	ClientID                   string
	SupplierName               string
	SupplierCUI                *string
	NormalizedSupplierCUI      *string
	Direction                  Direction
	CustomerName               *string
	CustomerIdentifier         *string
	NormalizedCustomerID       *string
	CustomerIdentifierKind     *string
	DocumentNumber             string
	NormalizedDocumentNumber   string
	IssueDate                  time.Time
	IssueDay                   time.Time
	DueDate                    *time.Time
	Total                      money.Money
	SPVReference               string
	IngestionSource            string
	ExternalDeliveryID         string
	DuplicateOfInvoiceID       *string
	DuplicateAmountMatches     *bool
	DuplicateCurrencyMatches   *bool
	DocumentType               DocumentType
	PipelineStatus             PipelineStatus
	SagaStatus                 SagaStatus
	Revision                   uint64
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	Activity                   []audit.Event
	Lines                      []Line
	ActiveTask                 *validationtasks.Task
	ContractAssociation        *contracts.AssociationSnapshot
	ContractWaiver             *contracts.WaiverSnapshot
}

// Party is the other side of the invoice: the supplier of a received
// invoice, the customer of an issued one.
type Party struct {
	Name                 string
	Identifier           *string
	NormalizedIdentifier *string
	IdentifierKind       string
}

func (i Invoice) Outgoing() bool { return i.Direction == DirectionOutgoing }

// Counterparty returns the supplier of a received invoice and the customer of
// an issued one. The identifier is raw (a CNP is not masked here).
func (i Invoice) Counterparty() Party {
	if i.Outgoing() {
		party := Party{Identifier: i.CustomerIdentifier, NormalizedIdentifier: i.NormalizedCustomerID}
		if i.CustomerName != nil {
			party.Name = *i.CustomerName
		}
		if i.CustomerIdentifierKind != nil {
			party.IdentifierKind = *i.CustomerIdentifierKind
		}
		return party
	}
	return Party{Name: i.SupplierName, Identifier: i.SupplierCUI, NormalizedIdentifier: i.NormalizedSupplierCUI, IdentifierKind: "CUI"}
}

type ClassificationContext struct {
	RunID          string
	ProfileID      string
	ProfileVersion int
	ContextStale   bool
	StaleReasons   []string
	CreatedAt      time.Time
}

type Line struct {
	SourceFacts     *accounting.LineFacts
	ID              string
	Position        int
	Description     string
	Unit            string
	VATRate         money.Amount
	VATValue        money.Amount
	Quantity        money.Amount
	UnitPrice       money.Amount
	NetValue        money.Amount
	TotalValue      money.Amount
	AdditionalInfo  *string
	Classifications []classification.Decision
}
