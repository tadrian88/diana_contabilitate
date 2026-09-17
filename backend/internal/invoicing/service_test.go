package invoicing

import (
	"context"
	"errors"
	"testing"

	"diana-contabilitate/backend/internal/apperrors"
)

type invoiceReaderStub struct {
	invoice *Invoice
	items   []Invoice
	err     error
}

func (s invoiceReaderStub) GetInvoice(context.Context, string) (*Invoice, error) {
	return s.invoice, s.err
}

func (s invoiceReaderStub) ListInvoices(context.Context, Filter) ([]Invoice, error) {
	return s.items, s.err
}

func TestGetInvoiceReturnsWorkspace(t *testing.T) {
	want := &Invoice{ID: "inv-resolved", ClientID: "client-beta", PipelineStatus: StatusExported}
	got, err := NewService(invoiceReaderStub{invoice: want}).Get(context.Background(), want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestListInvoicesUsesCompleteBackendReader(t *testing.T) {
	want := []Invoice{{ID: "inv-1", ClientID: "client-alfa"}, {ID: "inv-2", ClientID: "client-alfa"}}
	got, err := NewService(invoiceReaderStub{items: want}).List(context.Background(), Filter{ClientID: "client-alfa"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "inv-1" || got[1].ID != "inv-2" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestGetInvoicePreservesNotFound(t *testing.T) {
	_, err := NewService(invoiceReaderStub{err: apperrors.ErrNotFound}).Get(context.Background(), "missing")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("got %v, want not found", err)
	}
}
