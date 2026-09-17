package saga

import (
	"context"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/invoicing"
)

type handoffStoreStub struct {
	view       *ExportView
	download   *Download
	invoice    *invoicing.Invoice
	confirmErr error
	command    ConfirmCommand
}

func (s *handoffStoreStub) GetExportView(context.Context, string, string) (*ExportView, error) {
	return s.view, nil
}
func (s *handoffStoreStub) DownloadExportArtifact(context.Context, string, string, Actor, time.Time) (*Download, error) {
	return s.download, nil
}
func (s *handoffStoreStub) ConfirmSagaImport(_ context.Context, command ConfirmCommand, _ time.Time) (*invoicing.Invoice, *ExportView, bool, error) {
	s.command = command
	return s.invoice, s.view, s.confirmErr == nil, s.confirmErr
}

func TestHandoffRequiresAuthenticatedActor(t *testing.T) {
	service := NewHandoffService(&handoffStoreStub{}, nil)
	if _, err := service.View(context.Background(), "client-a", "invoice-a", Actor{}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected concealed authorization failure, got %v", err)
	}
}

func TestHandoffConfirmValidatesCommandAndForwardsHumanActor(t *testing.T) {
	store := &handoffStoreStub{invoice: &invoicing.Invoice{ID: "invoice-a"}, view: &ExportView{AttemptID: "attempt-a"}}
	service := NewHandoffService(store, func() time.Time { return time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC) })
	_, _, changed, err := service.Confirm(context.Background(), ConfirmCommand{ClientID: "client-a", InvoiceID: "invoice-a", AttemptID: "attempt-a", ExpectedInvoiceRevision: 3, CommandID: "confirm-a", Actor: Actor{ID: "accountant-a", Display: "Contabil A", AuthorizedClientIDs: []string{"client-a"}}})
	if err != nil || !changed {
		t.Fatalf("confirm failed: changed=%v err=%v", changed, err)
	}
	if store.command.Actor.ID != "accountant-a" || store.command.AttemptID != "attempt-a" {
		t.Fatalf("authoritative command was not forwarded: %#v", store.command)
	}
}

func TestHandoffConfirmRejectsMissingRevisionAndOversizedNote(t *testing.T) {
	service := NewHandoffService(&handoffStoreStub{}, nil)
	base := ConfirmCommand{ClientID: "client-a", InvoiceID: "invoice-a", AttemptID: "attempt-a", CommandID: "confirm-a", Actor: Actor{ID: "accountant-a", AuthorizedClientIDs: []string{"client-a"}}}
	if _, _, _, err := service.Confirm(context.Background(), base); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("expected validation error for missing revision, got %v", err)
	}
	base.ExpectedInvoiceRevision = 3
	for len(base.Note) <= 500 {
		base.Note += "x"
	}
	if _, _, _, err := service.Confirm(context.Background(), base); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("expected validation error for note, got %v", err)
	}
}

func TestHandoffConcealsClientOutsideActorGrant(t *testing.T) {
	service := NewHandoffService(&handoffStoreStub{}, nil)
	_, err := service.View(context.Background(), "client-b", "invoice-b", Actor{ID: "accountant-a", AuthorizedClientIDs: []string{"client-a"}})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected concealed cross-client denial, got %v", err)
	}
}
