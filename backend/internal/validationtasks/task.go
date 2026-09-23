package validationtasks

import (
	"encoding/json"
	"time"

	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/classification"
)

type Type string

const (
	TypeContractMatch    Type = "CONTRACT_MATCH"
	TypeMissingContract  Type = "MISSING_CONTRACT"
	TypeCommercialReview Type = "COMMERCIAL_REVIEW"
	TypeClassification   Type = "CLASSIFICATION"
)

var Types = []Type{TypeContractMatch, TypeMissingContract, TypeCommercialReview, TypeClassification}

type Status string

const (
	StatusOpen     Status = "OPEN"
	StatusWaiting  Status = "WAITING"
	StatusResolved Status = "RESOLVED"
)

var Statuses = []Status{StatusOpen, StatusWaiting, StatusResolved}

type Task struct {
	ID                  string
	ClientID            string
	InvoiceID           string
	Type                Type
	Status              Status
	Title               string
	Reason              string
	BlockerCode         *string
	ResolutionMetadata  json.RawMessage
	CreatedByKind       audit.ActorKind
	CreatedByID         *string
	CreatedByDisplay    *string
	Revision            uint64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	WaitingSince        *time.Time
	ResolvedAt          *time.Time
	ContractMatchRunID  *string
	ContractCandidates  []ContractCandidate
	ClassificationItems []classification.Decision
}

type ContractCandidate struct {
	ID            string
	Reference     string
	SupplierName  string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	ValueAmount   string
	Currency      string
	Confidence    string
	Reasons       []string
	UnitType      string
	PaymentTerms  string
	Recommended   bool
	Compatibility string
}

type InvoiceSummary struct {
	ID             string
	ClientID       string
	SupplierName   string
	SupplierCUI    *string
	DocumentNumber string
	IssueDate      time.Time
	TotalAmount    string
	Currency       string
	SPVReference   string
	PipelineStatus string
	SagaStatus     string
}

type ClientSummary struct {
	ID   string
	Name string
	CUI  string
}

type InboxItem struct {
	Task    Task
	Invoice InvoiceSummary
	Client  ClientSummary
}

type Filter struct {
	ClientID  string
	InvoiceID string
	Type      *Type
	Status    *Status
}

func ValidType(value Type) bool {
	for _, candidate := range Types {
		if value == candidate {
			return true
		}
	}
	return false
}

func ValidStatus(value Status) bool {
	for _, candidate := range Statuses {
		if value == candidate {
			return true
		}
	}
	return false
}
