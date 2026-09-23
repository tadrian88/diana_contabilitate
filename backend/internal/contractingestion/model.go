package contractingestion

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contracts"
)

const (
	ExtractionSchemaVersion = "CONTRACT_EXTRACTION_V4"
	ExtractionPromptVersion = "CONTRACT_EXTRACTION_PROMPT_V4_2"
	ProviderGemini          = "GEMINI"
)

type Status string

const (
	StatusUploaded         Status = "UPLOADED"
	StatusExtracting       Status = "EXTRACTING"
	StatusReadyForReview   Status = "READY_FOR_REVIEW"
	StatusExtractionFailed Status = "EXTRACTION_FAILED"
	StatusConfirmed        Status = "CONFIRMED"
)

type Confidence string

const (
	ConfidenceHigh    Confidence = "HIGH"
	ConfidenceMedium  Confidence = "MEDIUM"
	ConfidenceLow     Confidence = "LOW"
	ConfidenceUnknown Confidence = "UNKNOWN"
)

type Evidence struct {
	Page    *int   `json:"page"`
	Snippet string `json:"snippet,omitempty"`
}

type Field struct {
	Value        *string    `json:"value"`
	Status       string     `json:"status"`
	Confidence   Confidence `json:"confidence"`
	Evidence     Evidence   `json:"evidence"`
	Alternatives []string   `json:"alternatives"`
}

// Proposal carries legacy matching fields and V4 commercial clauses. Provider
// output remains a proposal until a human activates an immutable snapshot.
type Proposal struct {
	SupplierName      Field                      `json:"supplierName"`
	SupplierCUI       Field                      `json:"supplierCui"`
	Reference         Field                      `json:"reference"`
	EffectiveFrom     Field                      `json:"effectiveFrom"`
	EffectiveTo       Field                      `json:"effectiveTo"`
	TotalValue        Field                      `json:"totalValue"`
	Currency          Field                      `json:"currency"`
	UnitType          Field                      `json:"unitType"`
	PaymentTerms      Field                      `json:"paymentTerms"`
	BuyerCUI          Field                      `json:"buyerCui"`
	PeriodType        Field                      `json:"periodType"`
	ServiceTerms      []ProposedServiceTerm      `json:"serviceTerms"`
	DocumentRole      Field                      `json:"documentRole"`
	RelatedReference  Field                      `json:"relatedReference"`
	CommercialClauses []ProposedCommercialClause `json:"commercialClauses"`
}

type ProposedCommercialClause struct {
	Kind       Field           `json:"kind"`
	Narrative  Field           `json:"narrative"`
	Rule       json.RawMessage `json:"rule"`
	Evidence   Evidence        `json:"evidence"`
	Confidence Confidence      `json:"confidence"`
}

type ProposedServiceTerm struct {
	ServiceDescription Field `json:"serviceDescription"`
	PricingModel       Field `json:"pricingModel"`
	UnitPrice          Field `json:"unitPrice"`
	Currency           Field `json:"currency"`
	Unit               Field `json:"unit"`
	QuantitySource     Field `json:"quantitySource"`
	QuantityValue      Field `json:"quantityValue"`
	QuantityDriver     Field `json:"quantityDriver"`
	BillingFrequency   Field `json:"billingFrequency"`
}

type Attempt struct {
	ID, Provider, Model, SchemaVersion, PromptVersion, Status string
	Proposal                                                  *Proposal
	SafeErrorCategory                                         string
	InputTokens, OutputTokens                                 *int64
	StartedAt                                                 time.Time
	CompletedAt                                               *time.Time
}

type Document struct {
	ID, ClientID, OriginalFilename, MIMEType, SHA256 string
	SizeBytes                                        int64
	Status                                           Status
	LifecycleState                                   string
	LatestExtractionID, ConfirmedContractID          *string
	UploadedByID, UploadedByDisplay                  *string
	UploadedAt, UpdatedAt                            time.Time
	ConfirmedByID, ConfirmedByDisplay                *string
	ConfirmedAt                                      *time.Time
	Revision                                         uint64
	LatestAttempt                                    *Attempt
	Attempts                                         []Attempt
	ConfirmedValues                                  *ReviewedContract
	BuyerMismatch                                    bool
	ClientCUI                                        string
}

type Source struct {
	Document Document
	Bytes    []byte
}
type Actor struct {
	ID, Display, CorrelationID string
	AuthorizedClientIDs        []string
	AllClients                 bool
}
type Upload struct {
	ClientID, Filename, ContentType string
	Bytes                           []byte
	Actor                           Actor
}
type ExtractionResult struct {
	Proposal                  Proposal
	InputTokens, OutputTokens *int64
}

type ReviewedContract struct {
	SupplierName             string                        `json:"supplierName"`
	SupplierCUI              string                        `json:"supplierCui"`
	Reference                string                        `json:"reference"`
	EffectiveFrom            string                        `json:"effectiveFrom"`
	EffectiveTo              string                        `json:"effectiveTo"`
	TotalValue               string                        `json:"totalValue"`
	Currency                 string                        `json:"currency"`
	UnitType                 string                        `json:"unitType"`
	PaymentTerms             string                        `json:"paymentTerms"`
	BuyerCUI                 string                        `json:"buyerCui"`
	PeriodType               string                        `json:"periodType"`
	ServiceTerms             []ReviewedServiceTerm         `json:"serviceTerms"`
	DocumentRole             string                        `json:"documentRole"`
	RelatedReference         string                        `json:"relatedReference"`
	Coverage                 commercialvalidation.Coverage `json:"coverage"`
	CommercialRules          []commercialvalidation.Rule   `json:"commercialRules"`
	PendingCommercialClauses int                           `json:"-"`
}

type ReviewedServiceTerm struct {
	ServiceDescription string   `json:"serviceDescription"`
	PricingModel       string   `json:"pricingModel"`
	UnitPrice          string   `json:"unitPrice"`
	Currency           string   `json:"currency"`
	Unit               string   `json:"unit"`
	QuantitySource     string   `json:"quantitySource"`
	QuantityValue      string   `json:"quantityValue"`
	QuantityDriver     string   `json:"quantityDriver"`
	BillingFrequency   string   `json:"billingFrequency"`
	Evidence           Evidence `json:"evidence"`
}

type ConfirmationBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ConfirmationReadiness struct {
	CanConfirm bool                  `json:"canConfirm"`
	Blockers   []ConfirmationBlocker `json:"blockers"`
}

type ConfirmCommand struct {
	ClientID, DocumentID, ExtractionAttemptID string
	ExpectedDocumentRevision                  uint64
	Contract                                  ReviewedContract
	CommandID                                 string
	Actor                                     Actor
}

type ContractExtractor interface {
	Provider() string
	Model() string
	Extract(context.Context, []byte, string) (ExtractionResult, error)
}

type extractionRetryContextKey struct{}

// WithExtractionRetry marks a worker retry without exposing a provider
// response or validation value to the next extraction attempt.
func WithExtractionRetry(ctx context.Context) context.Context {
	return context.WithValue(ctx, extractionRetryContextKey{}, true)
}

func isExtractionRetry(ctx context.Context) bool {
	retry, _ := ctx.Value(extractionRetryContextKey{}).(bool)
	return retry
}

type DocumentStore interface {
	CreateDocument(context.Context, Upload, string, string, time.Time) (Document, bool, error)
	GetSource(context.Context, string) (Source, error)
}
type Store interface {
	DocumentStore
	ListDocuments(context.Context, string) ([]Document, error)
	GetDocument(context.Context, string, string) (Document, error)
	BeginExtraction(context.Context, string, string, string, string, time.Time) (Document, bool, error)
	CompleteExtraction(context.Context, string, string, ExtractionResult, time.Time) error
	FailExtraction(context.Context, string, string, string, time.Time) error
	RetryExtraction(context.Context, string, string, uint64, Actor, time.Time) error
	DiscardDocument(context.Context, string, string, uint64, Actor, time.Time) error
	ConfirmDocument(context.Context, ConfirmCommand, contracts.Contract, time.Time) (string, bool, error)
}

type DossierActivator interface {
	ActivateConfirmedContract(context.Context, ConfirmCommand, string, time.Time) error
}

type ContractAvailability interface {
	ContractAvailable(context.Context, contracts.AvailableCommand) (bool, error)
}

var (
	ErrInvalidPDF          = errors.New("invalid PDF")
	ErrDocumentTooLarge    = errors.New("contract document too large")
	ErrExtractionPermanent = errors.New("permanent extraction failure")
	ErrExtractionTransient = errors.New("transient extraction failure")
	ErrNoContractData      = errors.New("no contract data")
	ErrBuyerMismatch       = errors.New("buyer identity does not match client")
	ErrExtractionBusy      = errors.New("extraction already in progress")
)
