package saga

import (
	"context"
	"errors"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/invoicing"
)

type ConfirmationType string

const ConfirmationHuman ConfirmationType = "HUMAN"

type Actor struct {
	ID                  string
	Display             string
	CorrelationID       string
	AuthorizedClientIDs []string
	AllClients          bool
}

type ExportView struct {
	AttemptID        string
	ArtifactStatus   AttemptStatus
	Filename         string
	GeneratedAt      time.Time
	DownloadedAt     *time.Time
	ConfirmedAt      *time.Time
	ConfirmedBy      *string
	ConfirmationType *ConfirmationType
	InvoiceRevision  uint64
}

type Download struct {
	Artifact  Artifact
	AttemptID string
}

type ConfirmCommand struct {
	ClientID                string
	InvoiceID               string
	AttemptID               string
	ExpectedInvoiceRevision uint64
	Note                    string
	CommandID               string
	Actor                   Actor
}

type HandoffStore interface {
	GetExportView(context.Context, string, string) (*ExportView, error)
	DownloadExportArtifact(context.Context, string, string, Actor, time.Time) (*Download, error)
	ConfirmSagaImport(context.Context, ConfirmCommand, time.Time) (*invoicing.Invoice, *ExportView, bool, error)
}

type HandoffService struct {
	store HandoffStore
	clock Clock
}

func NewHandoffService(store HandoffStore, clock Clock) *HandoffService {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &HandoffService{store: store, clock: clock}
}

func (s *HandoffService) View(ctx context.Context, clientID, invoiceID string, actor Actor) (*ExportView, error) {
	if err := authorize(clientID, invoiceID, actor); err != nil {
		return nil, err
	}
	return s.store.GetExportView(ctx, clientID, invoiceID)
}

func (s *HandoffService) Download(ctx context.Context, clientID, invoiceID string, actor Actor) (*Download, error) {
	if err := authorize(clientID, invoiceID, actor); err != nil {
		return nil, err
	}
	return s.store.DownloadExportArtifact(ctx, clientID, invoiceID, actor, s.clock())
}

func (s *HandoffService) Confirm(ctx context.Context, command ConfirmCommand) (*invoicing.Invoice, *ExportView, bool, error) {
	if err := authorize(command.ClientID, command.InvoiceID, command.Actor); err != nil {
		return nil, nil, false, err
	}
	command.AttemptID = strings.TrimSpace(command.AttemptID)
	command.CommandID = strings.TrimSpace(command.CommandID)
	command.Note = strings.TrimSpace(command.Note)
	if command.AttemptID == "" || command.CommandID == "" || command.ExpectedInvoiceRevision == 0 || len(command.Note) > 500 {
		return nil, nil, false, apperrors.ErrValidation
	}
	return s.store.ConfirmSagaImport(ctx, command, s.clock())
}

func authorize(clientID, invoiceID string, actor Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return apperrors.ErrNotFound
	}
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(invoiceID) == "" {
		return apperrors.ErrValidation
	}
	if !actor.AllClients {
		allowed := false
		for _, allowedClientID := range actor.AuthorizedClientIDs {
			if allowedClientID == clientID {
				allowed = true
				break
			}
		}
		if !allowed {
			return apperrors.ErrNotFound
		}
	}
	return nil
}

var ErrArtifactUnavailable = errors.New("SAGA artifact unavailable")
