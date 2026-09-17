package spv

import (
	"context"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

type memorySPVStore struct {
	connection            Connection
	document              SourceDocument
	failedKind, invoiceID string
	syncFinishedErr       error
}

func (s *memorySPVStore) ListActiveConnections(context.Context) ([]Connection, error) {
	return []Connection{s.connection}, nil
}
func (s *memorySPVStore) GetConnection(context.Context, string) (Connection, error) {
	return s.connection, nil
}
func (s *memorySPVStore) SaveTokens(_ context.Context, _ string, _ uint64, a, r string, e time.Time, re *time.Time, _ time.Time) (Connection, error) {
	s.connection.AccessTokenCiphertext = a
	s.connection.RefreshTokenCiphertext = r
	s.connection.AccessTokenExpiresAt = e
	s.connection.RefreshTokenExpiresAt = re
	s.connection.Revision++
	return s.connection, nil
}
func (*memorySPVStore) MarkSyncStarted(context.Context, string, time.Time) error { return nil }
func (s *memorySPVStore) MarkSyncFinished(_ context.Context, _ string, _ time.Time, err error) error {
	s.syncFinishedErr = err
	return nil
}
func (s *memorySPVStore) Discover(_ context.Context, _ Connection, m Message, _ time.Time) (SourceDocument, bool, error) {
	s.document.ExternalMessageID = m.ID
	return s.document, true, nil
}
func (s *memorySPVStore) ClaimSourceDocument(context.Context, string, string, time.Time, time.Duration) (SourceDocument, error) {
	if s.document.Status == "PROCESSED" {
		return SourceDocument{}, ErrDocumentClaimed
	}
	s.document.Status = "PROCESSING"
	return s.document, nil
}
func (s *memorySPVStore) StoreRaw(_ context.Context, _ string, _ string, raw []byte, contentType, hash string, _ time.Time) error {
	s.document.RawDocument = raw
	s.document.ContentType = contentType
	s.document.ContentSHA256 = hash
	return nil
}
func (s *memorySPVStore) MarkSourceFailed(_ context.Context, _ string, _ string, kind, _ string, _ time.Time, _ time.Time) error {
	s.failedKind = kind
	s.document.Status = "FAILED"
	return nil
}
func (s *memorySPVStore) MarkProcessed(_ context.Context, _ string, _ string, invoiceID, _ string, _ string, _ time.Time) error {
	s.invoiceID = invoiceID
	s.document.Status = "PROCESSED"
	return nil
}

type fixedCipher struct{}

func (fixedCipher) Encrypt(v string) (string, error) { return "enc:" + v, nil }
func (fixedCipher) Decrypt(v string) (string, error) { return v, nil }

type fakeSPVClient struct {
	downloads  int
	refreshErr error
}

func (f *fakeSPVClient) ListIncoming(context.Context, string, string, time.Time, time.Time, int) ([]Message, int, error) {
	return []Message{{ID: "100"}}, 1, nil
}
func (f *fakeSPVClient) Download(context.Context, string, string) ([]byte, string, error) {
	f.downloads++
	return []byte("zip"), "application/zip", nil
}
func (f *fakeSPVClient) RefreshToken(context.Context, string, string, string) (TokenResponse, error) {
	if f.refreshErr != nil {
		return TokenResponse{}, f.refreshErr
	}
	return TokenResponse{}, errors.New("unexpected refresh")
}

func TestPermanentRefreshFailureIsExplicitlyReauthenticationRequired(t *testing.T) {
	store := &memorySPVStore{connection: Connection{ID: "connection", ClientID: "client", CIF: "RO222", Status: "ACTIVE", AccessTokenCiphertext: "expired", RefreshTokenCiphertext: "refresh", AccessTokenExpiresAt: time.Now().Add(-time.Hour)}}
	client := &fakeSPVClient{refreshErr: errors.Join(ErrPermanent, errors.New("provider rejected refresh"))}
	service := NewService(store, client, fixedParser{}, &captureIngester{}, fixedCipher{}, ServiceConfig{OAuthClientID: "app", OAuthClientSecret: "secret"})
	_, err := service.Sync(context.Background(), "connection")
	if !errors.Is(err, ErrReauthenticationRequired) || !errors.Is(store.syncFinishedErr, ErrReauthenticationRequired) {
		t.Fatalf("err=%v finished=%v", err, store.syncFinishedErr)
	}
}

type fixedParser struct{ buyer string }

func (p fixedParser) Parse([]byte) (ParsedDocument, error) {
	cui := "RO111"
	return ParsedDocument{BuyerCUI: p.buyer, Format: ParserTypeUBL, Version: ParserVersion, Invoice: invoicing.IngestionInput{SupplierName: "S", SupplierCUI: &cui, DocumentNumber: "1", IssueDate: time.Now().UTC(), Total: money.Money{Amount: money.MustParse("1"), Currency: "RON"}, Lines: []invoicing.Line{{Position: 1, Description: "x", Unit: "H87", VATRate: money.MustParse("0"), VATValue: money.MustParse("0"), Quantity: money.MustParse("1"), UnitPrice: money.MustParse("1"), NetValue: money.MustParse("1"), TotalValue: money.MustParse("1")}}}}, nil
}

type captureIngester struct{ calls int }

func (i *captureIngester) Ingest(_ context.Context, input invoicing.IngestionInput) (*invoicing.Invoice, bool, error) {
	i.calls++
	return &invoicing.Invoice{ID: "invoice-1", SPVReference: input.SPVReference}, true, nil
}

func TestServiceProcessesOneSourceThroughNativeInvoiceBoundary(t *testing.T) {
	store := &memorySPVStore{connection: Connection{ID: "connection", ClientID: "client", CIF: "RO222", Status: "ACTIVE", AccessTokenCiphertext: "token", AccessTokenExpiresAt: time.Now().Add(time.Hour)}, document: SourceDocument{ID: "document", ConnectionID: "connection", ClientID: "client", ExternalMessageID: "100", Status: "DISCOVERED"}}
	client := &fakeSPVClient{}
	ingester := &captureIngester{}
	service := NewService(store, client, fixedParser{buyer: "222"}, ingester, fixedCipher{}, ServiceConfig{})
	id, created, err := service.ProcessDocument(context.Background(), "document", "worker")
	if err != nil || !created || id != "invoice-1" || client.downloads != 1 || ingester.calls != 1 || store.invoiceID != "invoice-1" {
		t.Fatalf("id=%s created=%t downloads=%d calls=%d err=%v", id, created, client.downloads, ingester.calls, err)
	}
}
func TestServiceQuarantinesWrongBuyerWithoutInvoice(t *testing.T) {
	store := &memorySPVStore{connection: Connection{ID: "c", ClientID: "client", CIF: "222", Status: "ACTIVE", AccessTokenCiphertext: "token", AccessTokenExpiresAt: time.Now().Add(time.Hour)}, document: SourceDocument{ID: "d", ConnectionID: "c", ExternalMessageID: "1"}}
	ingester := &captureIngester{}
	service := NewService(store, &fakeSPVClient{}, fixedParser{buyer: "333"}, ingester, fixedCipher{}, ServiceConfig{})
	_, _, err := service.ProcessDocument(context.Background(), "d", "worker")
	if !errors.Is(err, ErrPermanent) || store.failedKind != "PERMANENT" || ingester.calls != 0 {
		t.Fatalf("err=%v failure=%s calls=%d", err, store.failedKind, ingester.calls)
	}
}
