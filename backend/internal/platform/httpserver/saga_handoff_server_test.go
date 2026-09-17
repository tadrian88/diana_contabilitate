package httpserver

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"diana-contabilitate/backend/internal/saga"
)

type sagaHTTPStore struct {
	downloads int
	confirms  int
	missing   bool
}

func (s *sagaHTTPStore) GetExportView(context.Context, string, string) (*saga.ExportView, error) {
	if s.missing {
		return nil, apperrors.ErrNotFound
	}
	return sagaHTTPView(), nil
}
func (s *sagaHTTPStore) DownloadExportArtifact(_ context.Context, clientID, invoiceID string, _ saga.Actor, _ time.Time) (*saga.Download, error) {
	if s.missing || clientID != "client-a" || invoiceID != "invoice-a" {
		return nil, apperrors.ErrNotFound
	}
	s.downloads++
	return &saga.Download{AttemptID: "attempt-a", Artifact: saga.Artifact{Filename: "Factura 1.xml", ContentType: saga.ContentType, Payload: []byte("<?xml version=\"1.0\"?><Facturi/>\n")}}, nil
}
func (s *sagaHTTPStore) ConfirmSagaImport(_ context.Context, command saga.ConfirmCommand, _ time.Time) (*invoicing.Invoice, *saga.ExportView, bool, error) {
	if command.ClientID != "client-a" || command.InvoiceID != "invoice-a" || command.AttemptID != "attempt-a" {
		return nil, nil, false, apperrors.ErrNotFound
	}
	s.confirms++
	view := sagaHTTPView()
	now := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	actor := "Contabil demo"
	kind := saga.ConfirmationHuman
	view.ConfirmedAt, view.ConfirmedBy, view.ConfirmationType = &now, &actor, &kind
	return &invoicing.Invoice{ID: "invoice-a", ClientID: "client-a", SupplierName: "Furnizor", DocumentNumber: "1", Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, PipelineStatus: invoicing.StatusExported, SagaStatus: invoicing.SagaExported, Revision: 4, IssueDate: now, CreatedAt: now, UpdatedAt: now}, view, true, nil
}
func sagaHTTPView() *saga.ExportView {
	return &saga.ExportView{AttemptID: "attempt-a", ArtifactStatus: saga.AttemptGenerated, Filename: "Factura 1.xml", GeneratedAt: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC), InvoiceRevision: 3}
}

func sagaHTTPHandler(store *sagaHTTPStore) http.Handler {
	return withTestActor(NewWithIntegrations(nil, nil, nil, nil, nil, nil, nil, saga.NewHandoffService(store, nil), readyStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics()), requestactor.Actor{ID: "test-accountant", Display: "Test", Persona: "CONTABIL", AllClients: true})
}

func TestSagaArtifactDownloadIsExactPrivateAndRepeatable(t *testing.T) {
	store := &sagaHTTPStore{}
	handler := sagaHTTPHandler(store)
	want := []byte("<?xml version=\"1.0\"?><Facturi/>\n")
	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/clients/client-a/invoices/invoice-a/saga-export/artifact", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), want) {
			t.Fatalf("unexpected artifact response: %d %q", response.Code, response.Body.Bytes())
		}
		if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("missing security headers: %#v", response.Header())
		}
		if !strings.Contains(response.Header().Get("Content-Disposition"), "attachment") {
			t.Fatalf("unsafe disposition: %q", response.Header().Get("Content-Disposition"))
		}
	}
	if store.downloads != 2 {
		t.Fatalf("expected two audited downloads, got %d", store.downloads)
	}
}

func TestSagaArtifactDownloadConcealsCrossClientAndMissingArtifact(t *testing.T) {
	handler := sagaHTTPHandler(&sagaHTTPStore{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/clients/client-b/invoices/invoice-a/saga-export/artifact", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected concealed artifact response, got %d", response.Code)
	}
}

func TestSagaHumanConfirmationRequiresIdempotencyKey(t *testing.T) {
	handler := sagaHTTPHandler(&sagaHTTPStore{})
	body := `{"attemptId":"attempt-a","expectedInvoiceRevision":3}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/clients/client-a/invoices/invoice-a/saga-export/confirm-import", strings.NewReader(body)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error, got %d", response.Code)
	}
}

func TestSagaHumanConfirmationUsesDomainCommand(t *testing.T) {
	store := &sagaHTTPStore{}
	handler := sagaHTTPHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clients/client-a/invoices/invoice-a/saga-export/confirm-import", strings.NewReader(`{"attemptId":"attempt-a","expectedInvoiceRevision":3}`))
	request.Header.Set("Idempotency-Key", "confirm-a")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || store.confirms != 1 || !strings.Contains(response.Body.String(), `"confirmationType":"HUMAN"`) {
		t.Fatalf("unexpected confirmation: %d %s", response.Code, response.Body.String())
	}
}
