package invoicing

import (
	"context"
	"crypto/sha256"
	"diana-contabilitate/backend/internal/accounting"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/outbox"
)

type IngestionInput struct {
	ModelVersion       string
	SourceFacts        *accounting.SourceFacts
	ID                 string
	ClientID           string
	Source             string
	ExternalDeliveryID string
	DocumentType       DocumentType
	SupplierName       string
	SupplierCUI        *string
	DocumentNumber     string
	IssueDate          time.Time
	DueDate            *time.Time
	Total              money.Money
	SPVReference       string
	Lines              []Line
}

type TransitionCommand struct {
	InvoiceID        string
	From             PipelineStatus
	To               PipelineStatus
	ExpectedRevision uint64
	Trigger          TransitionTrigger
	CommandID        string
	Actor            audit.ActorKind
	ActorDisplay     string
	CorrelationID    string
}

type OutboxEntry = outbox.Entry

type ContinuationOutcome string

const (
	ContinuationProcessed ContinuationOutcome = "PROCESSED"
	ContinuationStale     ContinuationOutcome = "STALE_NOOP"
)

type PipelineStore interface {
	Reader
	IngestInvoice(context.Context, IngestionInput, string, time.Time) (*Invoice, bool, error)
	ApplyTransition(context.Context, TransitionCommand, TransitionDefinition, time.Time) (*Invoice, bool, error)
	RecordSagaFailure(context.Context, string, uint64, string, string, time.Time) (*Invoice, bool, error)
	PendingOutbox(context.Context, int, time.Time) ([]OutboxEntry, error)
	MarkOutboxProcessed(context.Context, string, time.Time) error
}

type SagaExporter interface {
	Export(context.Context, *Invoice, string) (SagaExportResult, error)
}

type SagaExportResult struct {
	// Confirmed is true only when the adapter has authoritative evidence of the
	// external success represented by EXPORTED.
	Confirmed bool
	Reference string
}

type ContractMatchingProcessor interface {
	ProcessContractMatching(context.Context, string, uint64, string, string) error
}

type ClassificationProcessor interface {
	ProcessClassification(context.Context, string, uint64, string, string) error
}

type Clock func() time.Time

func NewPipelineService(store PipelineStore, exporter SagaExporter, clock Clock) *Service {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	service := &Service{reader: store, pipeline: store, exporter: exporter, clock: clock}
	if lister, ok := store.(Lister); ok {
		service.lister = lister
	}
	return service
}

func (s *Service) Ingest(ctx context.Context, input IngestionInput) (*Invoice, bool, error) {
	if s.pipeline == nil {
		return nil, false, errors.New("pipeline store is not configured")
	}
	if input.DocumentType == "" {
		input.DocumentType = DocumentTypeInvoice
	}
	if input.ClientID == "" || input.Source == "" || input.ExternalDeliveryID == "" || input.SupplierName == "" || input.SupplierCUI == nil || NormalizeBusinessIdentifier(*input.SupplierCUI) == "" || input.SPVReference == "" || NormalizeBusinessIdentifier(input.DocumentNumber) == "" || !input.Total.Amount.Valid() || len(input.Total.Currency) != 3 || input.Total.Currency != strings.ToUpper(input.Total.Currency) || (input.DocumentType != DocumentTypeInvoice && input.DocumentType != DocumentTypeCreditNote) {
		return nil, false, apperrors.ErrValidation
	}
	for index, line := range input.Lines {
		if line.Position <= 0 || line.Description == "" || line.Unit == "" || !validLine(line) {
			return nil, false, fmt.Errorf("line %d: %w", index, apperrors.ErrValidation)
		}
	}
	key := ingestionKey(input.ClientID, input.SPVReference)
	return s.pipeline.IngestInvoice(ctx, input, key, s.clock())
}

func (s *Service) TransitionInternally(ctx context.Context, command TransitionCommand) (*Invoice, bool, error) {
	if s.pipeline == nil || command.CommandID == "" || command.ExpectedRevision == 0 {
		return nil, false, apperrors.ErrValidation
	}
	definition, err := ValidateModule2Transition(command.From, command.To, command.Trigger)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrValidation, err)
	}
	return s.pipeline.ApplyTransition(ctx, command, definition, s.clock())
}

func (s *Service) DispatchPending(ctx context.Context, limit int) (int, error) {
	if s.pipeline == nil || s.exporter == nil {
		return 0, errors.New("pipeline dependencies are not configured")
	}
	entries, err := s.pipeline.PendingOutbox(ctx, limit, s.clock())
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, entry := range entries {
		if err := s.processContinuation(ctx, entry); err != nil {
			return processed, err
		}
		if err := s.pipeline.MarkOutboxProcessed(ctx, entry.ID, s.clock()); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *Service) Drain(ctx context.Context, limit int) error {
	for {
		count, err := s.DispatchPending(ctx, limit)
		if err != nil || count == 0 {
			return err
		}
	}
}

func (s *Service) processContinuation(ctx context.Context, entry OutboxEntry) error {
	_, err := s.ProcessContinuation(ctx, entry)
	return err
}

// ProcessContinuation is the application boundary used by asynchronous
// transports. PostgreSQL remains authoritative; payload snapshots are never
// trusted. Harmless stale deliveries are acknowledged as no-ops.
func (s *Service) ProcessContinuation(ctx context.Context, entry OutboxEntry) (ContinuationOutcome, error) {
	if entry.ID == "" || entry.AggregateID == "" || entry.IdempotencyKey == "" || entry.EventType != outbox.EventInvoiceContinue {
		return ContinuationStale, apperrors.ErrValidation
	}
	item, err := s.Get(ctx, entry.AggregateID)
	if err != nil {
		return ContinuationStale, err
	}
	correlationID := entry.CorrelationID
	if correlationID == "" {
		correlationID = entry.ID
	}
	command := TransitionCommand{InvoiceID: item.ID, From: item.PipelineStatus, ExpectedRevision: item.Revision, CommandID: entry.IdempotencyKey + ":transition", Actor: audit.ActorSystem, ActorDisplay: "Sistem pipeline", CorrelationID: correlationID}
	switch item.PipelineStatus {
	case StatusDownloaded:
		command.To, command.Trigger = StatusArchived, TriggerArchiveCompleted
	case StatusArchived:
		command.To, command.Trigger = StatusMatching, TriggerStartMatching
	case StatusMatching:
		if s.matcher == nil {
			return ContinuationStale, nil
		}
		err = s.matcher.ProcessContractMatching(ctx, item.ID, item.Revision, entry.IdempotencyKey+":contract-match", correlationID)
		if errors.Is(err, apperrors.ErrConflict) {
			return ContinuationStale, nil
		}
		return ContinuationProcessed, err
	case StatusDedupeChecked:
		command.To, command.Trigger = StatusHeaderRead, TriggerDuplicateCleared
	case StatusHeaderRead:
		command.To, command.Trigger = StatusLinesRead, TriggerLinesParsed
	case StatusLinesRead:
		if s.classifier == nil {
			return ContinuationStale, nil
		}
		err = s.classifier.ProcessClassification(ctx, item.ID, item.Revision, entry.IdempotencyKey+":classification", correlationID)
		if errors.Is(err, apperrors.ErrConflict) {
			return ContinuationStale, nil
		}
		return ContinuationProcessed, err
	case StatusReadyForSAGA:
		command.To, command.Trigger = StatusExporting, TriggerSagaHandoff
	case StatusExporting:
		result, exportErr := s.exporter.Export(ctx, item, entry.IdempotencyKey+":export")
		if exportErr != nil {
			var permanent interface{ Permanent() bool }
			if !errors.As(exportErr, &permanent) || !permanent.Permanent() {
				return ContinuationProcessed, exportErr
			}
			_, _, recordErr := s.pipeline.RecordSagaFailure(ctx, item.ID, item.Revision, entry.IdempotencyKey+":failure", correlationID, s.clock())
			if errors.Is(recordErr, apperrors.ErrConflict) {
				return ContinuationStale, nil
			}
			return ContinuationProcessed, recordErr
		}
		if !result.Confirmed {
			// A generated/downloadable file is not evidence that desktop SAGA
			// imported it. Leave the operational state at EXPORTING until an
			// explicitly approved acknowledgement transition exists.
			return ContinuationProcessed, nil
		}
		command.To, command.Trigger = StatusExported, TriggerSagaExported
	default:
		return ContinuationStale, nil
	}
	_, _, err = s.TransitionInternally(ctx, command)
	if errors.Is(err, apperrors.ErrConflict) {
		return ContinuationStale, nil
	}
	return ContinuationProcessed, err
}

func validLine(line Line) bool {
	return line.VATRate.Valid() && line.VATValue.Valid() && line.Quantity.Valid() && line.UnitPrice.Valid() && line.NetValue.Valid() && line.TotalValue.Valid()
}

func ingestionKey(clientID, spvReference string) string {
	sum := sha256.Sum256([]byte(clientID + "\x00" + spvReference))
	return hex.EncodeToString(sum[:])
}

type FakeSagaExporter struct {
	mu         sync.Mutex
	failures   map[string]bool
	operations map[string]error
}

type fakeSagaFailure struct{}

func (fakeSagaFailure) Error() string   { return "deterministic fake SAGA failure" }
func (fakeSagaFailure) Permanent() bool { return true }

func NewFakeSagaExporter(failureInvoiceIDs ...string) *FakeSagaExporter {
	failures := make(map[string]bool, len(failureInvoiceIDs))
	for _, id := range failureInvoiceIDs {
		failures[id] = true
	}
	return &FakeSagaExporter{failures: failures, operations: make(map[string]error)}
}

func (e *FakeSagaExporter) Export(_ context.Context, invoice *Invoice, idempotencyKey string) (SagaExportResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if result, ok := e.operations[idempotencyKey]; ok {
		return SagaExportResult{Confirmed: result == nil, Reference: idempotencyKey}, result
	}
	var result error
	if e.failures[invoice.ID] {
		result = fakeSagaFailure{}
	}
	e.operations[idempotencyKey] = result
	return SagaExportResult{Confirmed: result == nil, Reference: idempotencyKey}, result
}

func (e *FakeSagaExporter) OperationCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.operations)
}
