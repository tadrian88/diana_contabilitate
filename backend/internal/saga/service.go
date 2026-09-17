package saga

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/invoicing"
)

type AttemptStatus string

const (
	AttemptGenerated AttemptStatus = "GENERATED"
	AttemptFailed    AttemptStatus = "FAILED"
)

type Attempt struct {
	ID                     string
	InvoiceID              string
	ClientID               string
	InvoiceRevision        uint64
	ExporterVersion        string
	Status                 AttemptStatus
	Artifact               *Artifact
	ClassificationSnapshot map[string]string
	FailureCategory        *FailureCategory
	SafeError              *string
	StartedAt              time.Time
	CompletedAt            time.Time
	ConfirmedAt            *time.Time
	ConfirmedByID          *string
	ConfirmedByDisplay     *string
	ConfirmationType       *ConfirmationType
	ConfirmationNote       *string
}

type Store interface {
	LoadExportInput(context.Context, string) (*invoicing.Invoice, ClientIdentity, error)
	FindAttempt(context.Context, string, uint64, string) (*Attempt, error)
	SaveAttempt(context.Context, Attempt) (*Attempt, bool, error)
}

type ArtifactReader interface {
	LatestGeneratedAttempt(context.Context, string, string) (*Attempt, error)
}

type Clock func() time.Time

// FileExporter generates and durably records the real SAGA import file. Its
// result is deliberately unconfirmed: local/manual SAGA import has no remote
// acknowledgement contract.
type FileExporter struct {
	allowTestOnly bool
	store         Store
	clock         Clock
}

func NewFileExporter(store Store, clock Clock) *FileExporter {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &FileExporter{store: store, clock: clock}
}

// Artifact returns a generated payload only when both invoice and client match.
// It is the application boundary for a future authenticated HTTP/local bridge.
func (e *FileExporter) Artifact(ctx context.Context, invoiceID, clientID string) (Artifact, error) {
	reader, ok := e.store.(ArtifactReader)
	if !ok || invoiceID == "" || clientID == "" {
		return Artifact{}, ErrAttemptNotFound
	}
	attempt, err := reader.LatestGeneratedAttempt(ctx, invoiceID, clientID)
	if err != nil {
		return Artifact{}, err
	}
	if attempt == nil || attempt.Artifact == nil {
		return Artifact{}, ErrAttemptNotFound
	}
	return *attempt.Artifact, nil
}

func (e *FileExporter) Export(ctx context.Context, invoice *invoicing.Invoice, _ string) (invoicing.SagaExportResult, error) {
	if invoice == nil || e.store == nil {
		return invoicing.SagaExportResult{}, invalid("Exportatorul SAGA nu este configurat.", nil)
	}
	item, client, err := e.store.LoadExportInput(ctx, invoice.ID)
	if err != nil {
		return invoicing.SagaExportResult{}, err
	}
	if item.Revision != invoice.Revision || item.ClientID != invoice.ClientID {
		return invoicing.SagaExportResult{}, invalid("Versiunea facturii s-a schimbat înaintea exportului SAGA.", nil)
	}
	if item.ModelVersion == accounting.ModelVersion {
		ready := EvaluateReadiness(item, client, e.allowTestOnly, false)
		if !ready.Ready {
			return invoicing.SagaExportResult{}, invalid(ready.Reason, nil)
		}
	}
	if previous, findErr := e.store.FindAttempt(ctx, item.ID, item.Revision, exportVersion(item)); findErr == nil {
		if previous.Status == AttemptGenerated {
			return invoicing.SagaExportResult{Confirmed: false, Reference: previous.ID}, nil
		}
		return invoicing.SagaExportResult{}, &Failure{Category: dereferenceCategory(previous.FailureCategory), Safe: dereferenceString(previous.SafeError)}
	} else if !errors.Is(findErr, ErrAttemptNotFound) {
		return invoicing.SagaExportResult{}, findErr
	}

	started := e.clock()
	artifact, generateErr := generate(item, client, e.allowTestOnly)
	attempt := Attempt{
		ID: attemptIDVersion(item.ID, item.Revision, exportVersion(item)), InvoiceID: item.ID, ClientID: item.ClientID,
		InvoiceRevision: item.Revision, ExporterVersion: exportVersion(item),
		StartedAt: started, CompletedAt: e.clock(),
	}
	if generateErr != nil {
		category, safe := Category(generateErr), generateErr.Error()
		attempt.Status, attempt.FailureCategory, attempt.SafeError = AttemptFailed, &category, &safe
	} else {
		attempt.Status, attempt.Artifact, attempt.ClassificationSnapshot = AttemptGenerated, &artifact, artifact.ClassificationSnapshot
	}
	persisted, _, saveErr := e.store.SaveAttempt(ctx, attempt)
	if saveErr != nil {
		return invoicing.SagaExportResult{}, saveErr
	}
	if persisted.Status == AttemptFailed {
		return invoicing.SagaExportResult{}, &Failure{Category: dereferenceCategory(persisted.FailureCategory), Safe: dereferenceString(persisted.SafeError), Cause: generateErr}
	}
	return invoicing.SagaExportResult{Confirmed: false, Reference: persisted.ID}, nil
}

var ErrAttemptNotFound = errors.New("SAGA export attempt not found")

func attemptID(invoiceID string, revision uint64) string {
	return attemptIDVersion(invoiceID, revision, ExporterVersion)
}
func attemptIDVersion(invoiceID string, revision uint64, version string) string {
	digest := sha256.Sum256([]byte(invoiceID + "\x00" + version + "\x00" + strconv.FormatUint(revision, 10)))
	return "saga-" + hex.EncodeToString(digest[:16])
}

func dereferenceCategory(value *FailureCategory) FailureCategory {
	if value == nil {
		return FailureSerialization
	}
	return *value
}

func dereferenceString(value *string) string {
	if value == nil || *value == "" {
		return "Exportul SAGA a eșuat."
	}
	return *value
}

func NewTestOnlyFileExporter(store Store, clock Clock) *FileExporter {
	e := NewFileExporter(store, clock)
	e.allowTestOnly = true
	return e
}
