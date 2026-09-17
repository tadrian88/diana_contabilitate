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
	StatusDownloaded           PipelineStatus = "DOWNLOADED"
	StatusArchived             PipelineStatus = "ARCHIVED"
	StatusMatching             PipelineStatus = "MATCHING"
	StatusAwaitingContract     PipelineStatus = "AWAITING_CONTRACT"
	StatusAwaitingMatchConfirm PipelineStatus = "AWAITING_MATCH_CONFIRM"
	StatusDedupeChecked        PipelineStatus = "DEDUPE_CHECKED"
	StatusHeaderRead           PipelineStatus = "HEADER_READ"
	StatusLinesRead            PipelineStatus = "LINES_READ"
	StatusClassified           PipelineStatus = "CLASSIFIED"
	StatusAwaitingReview       PipelineStatus = "AWAITING_REVIEW"
	StatusReadyForSAGA         PipelineStatus = "READY_FOR_SAGA"
	StatusExporting            PipelineStatus = "EXPORTING"
	StatusExported             PipelineStatus = "EXPORTED"
	StatusDuplicate            PipelineStatus = "DUPLICATE"
)

var PipelineStatuses = []PipelineStatus{
	StatusDownloaded, StatusArchived, StatusMatching, StatusAwaitingContract,
	StatusAwaitingMatchConfirm, StatusDedupeChecked, StatusHeaderRead, StatusLinesRead,
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

type Filter struct {
	ClientID string
}

type Invoice struct {
	ModelVersion             string
	SourceFacts              *accounting.SourceFacts
	AccountingSnapshot       *accounting.Snapshot
	ReadinessReason          string
	ID                       string
	ClientID                 string
	SupplierName             string
	SupplierCUI              *string
	NormalizedSupplierCUI    *string
	DocumentNumber           string
	NormalizedDocumentNumber string
	IssueDate                time.Time
	IssueDay                 time.Time
	DueDate                  *time.Time
	Total                    money.Money
	SPVReference             string
	IngestionSource          string
	ExternalDeliveryID       string
	DuplicateOfInvoiceID     *string
	DuplicateAmountMatches   *bool
	DuplicateCurrencyMatches *bool
	DocumentType             DocumentType
	PipelineStatus           PipelineStatus
	SagaStatus               SagaStatus
	Revision                 uint64
	CreatedAt                time.Time
	UpdatedAt                time.Time
	Activity                 []audit.Event
	Lines                    []Line
	ActiveTask               *validationtasks.Task
	ContractAssociation      *contracts.AssociationSnapshot
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
