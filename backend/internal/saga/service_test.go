package saga

import (
	"context"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
)

type memoryStore struct {
	invoice  *invoicing.Invoice
	client   ClientIdentity
	attempts map[string]*Attempt
	saves    int
}

func (s *memoryStore) LoadExportInput(context.Context, string) (*invoicing.Invoice, ClientIdentity, error) {
	return s.invoice, s.client, nil
}

func (s *memoryStore) FindAttempt(_ context.Context, _ string, revision uint64, version string) (*Attempt, error) {
	item := s.attempts[attemptID(s.invoice.ID, revision)]
	if item == nil || item.ExporterVersion != version {
		return nil, ErrAttemptNotFound
	}
	return item, nil
}

func (s *memoryStore) SaveAttempt(_ context.Context, attempt Attempt) (*Attempt, bool, error) {
	if existing := s.attempts[attempt.ID]; existing != nil {
		return existing, false, nil
	}
	s.saves++
	s.attempts[attempt.ID] = &attempt
	return &attempt, true, nil
}

func TestFileExporterPersistsOneImmutableUnconfirmedArtifact(t *testing.T) {
	invoice := validInvoice()
	store := &memoryStore{invoice: invoice, client: ClientIdentity{ID: invoice.ClientID, Name: "Client", CUI: "RO9876543"}, attempts: map[string]*Attempt{}}
	exporter := NewFileExporter(store, func() time.Time { return invoice.IssueDate })
	for range 2 {
		result, err := exporter.Export(context.Background(), invoice, "delivery-can-repeat")
		if err != nil || result.Confirmed || result.Reference == "" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	if store.saves != 1 || len(store.attempts) != 1 {
		t.Fatalf("saves=%d attempts=%d", store.saves, len(store.attempts))
	}
}

func TestFileExporterPersistsPermanentDataFailure(t *testing.T) {
	invoice := validInvoice()
	invoice.DocumentType = invoicing.DocumentTypeCreditNote
	store := &memoryStore{invoice: invoice, client: ClientIdentity{ID: invoice.ClientID, Name: "Client", CUI: "RO9876543"}, attempts: map[string]*Attempt{}}
	_, err := NewFileExporter(store, nil).Export(context.Background(), invoice, "once")
	if err == nil || Category(err) != FailureUnsupportedDocumentType || store.saves != 1 {
		t.Fatalf("saves=%d category=%s err=%v", store.saves, Category(err), err)
	}
}
