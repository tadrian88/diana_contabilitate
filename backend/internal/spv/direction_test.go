package spv

import (
	"context"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
)

func TestResolveDirectionFromXMLParties(t *testing.T) {
	for _, tc := range []struct {
		buyer, supplier string
		want            invoicing.Direction
	}{
		{"RO51741718", "RO37403193", invoicing.DirectionIncoming},
		{"1800101420010", "RO51741718", invoicing.DirectionOutgoing},
		{"31482767", "51741718", invoicing.DirectionOutgoing},
		{"RO51741718", "RO51741718", invoicing.DirectionIncoming},
	} {
		got, err := ResolveDirection(ParsedDocument{BuyerCUI: tc.buyer, SupplierCUI: tc.supplier}, "RO51741718")
		if err != nil || got != tc.want {
			t.Fatalf("buyer %s supplier %s = %s err=%v, want %s", tc.buyer, tc.supplier, got, err, tc.want)
		}
	}
	if _, err := ResolveDirection(ParsedDocument{BuyerCUI: "RO26999270", SupplierCUI: "RO37403193"}, "RO51741718"); !errors.Is(err, ErrPermanent) {
		t.Fatalf("foreign document err=%v", err)
	}
}

func TestSyncListsReceivedAndSentWithOwnWindowsAndKeepsDocumentsOnce(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	lastReceived := now.Add(-24 * time.Hour)
	store := &memorySPVStore{connection: Connection{ID: "connection", ClientID: "client", CIF: "RO222", Status: "ACTIVE", AccessTokenCiphertext: "token", AccessTokenExpiresAt: now.Add(time.Hour), LastSuccessfulSyncAt: &lastReceived}, document: SourceDocument{ID: "document"}}
	client := &fakeSPVClient{}
	service := NewService(store, client, fixedParser{}, &captureIngester{}, fixedCipher{}, ServiceConfig{InitialWindow: 60 * 24 * time.Hour, Overlap: 72 * time.Hour})
	service.clock = func() time.Time { return now }
	result, err := service.Sync(context.Background(), "connection")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.filters) != 2 || client.filters[0] != MessageFilterReceived || client.filters[1] != MessageFilterSent {
		t.Fatalf("filters = %v", client.filters)
	}
	if !client.starts[MessageFilterReceived].Equal(lastReceived.Add(-72 * time.Hour)) {
		t.Fatalf("received window start = %s", client.starts[MessageFilterReceived])
	}
	if !client.starts[MessageFilterSent].Equal(now.Add(-60 * 24 * time.Hour)) {
		t.Fatalf("first sent sync must use the initial window, got %s", client.starts[MessageFilterSent])
	}
	if len(result.Documents) != 1 {
		t.Fatalf("documents = %d, want the shared document once", len(result.Documents))
	}
}

type issuedParser struct{ customerName string }

func (p issuedParser) Parse(raw []byte) (ParsedDocument, error) {
	parsed, err := fixedParser{}.Parse(raw)
	own := "RO222"
	parsed.BuyerCUI, parsed.SupplierCUI = "1800101420010", own
	parsed.Invoice.SupplierCUI = &own
	parsed.Invoice.CustomerName, parsed.Invoice.CustomerIdentifier, parsed.Invoice.CustomerCountry = p.customerName, "1800101420010", "RO"
	return parsed, err
}

type directionIngester struct{ input *invoicing.IngestionInput }

func (i *directionIngester) Ingest(_ context.Context, input invoicing.IngestionInput) (*invoicing.Invoice, bool, error) {
	i.input = &input
	return &invoicing.Invoice{ID: "invoice-1"}, true, nil
}

func TestProcessDocumentIngestsIssuedInvoiceWithCustomer(t *testing.T) {
	store := &memorySPVStore{connection: Connection{ID: "connection", ClientID: "client", CIF: "RO222", Status: "ACTIVE", AccessTokenCiphertext: "token", AccessTokenExpiresAt: time.Now().Add(time.Hour)}, document: SourceDocument{ID: "document", ConnectionID: "connection", ClientID: "client", ExternalMessageID: "200", Status: "DISCOVERED"}}
	ingester := &directionIngester{}
	service := NewService(store, &fakeSPVClient{}, issuedParser{customerName: "PERSOANĂ FIZICĂ TEST"}, ingester, fixedCipher{}, ServiceConfig{})
	if _, _, err := service.ProcessDocument(context.Background(), "document", "worker"); err != nil {
		t.Fatal(err)
	}
	if ingester.input == nil || ingester.input.Direction != invoicing.DirectionOutgoing || ingester.input.CustomerIdentifier != "1800101420010" {
		t.Fatalf("ingested = %+v", ingester.input)
	}

	unnamed := &memorySPVStore{connection: store.connection, document: SourceDocument{ID: "document-2", ConnectionID: "connection", ClientID: "client", ExternalMessageID: "201", Status: "DISCOVERED"}}
	service = NewService(unnamed, &fakeSPVClient{}, issuedParser{}, &directionIngester{}, fixedCipher{}, ServiceConfig{})
	if _, _, err := service.ProcessDocument(context.Background(), "document-2", "worker"); !errors.Is(err, ErrPermanent) || unnamed.failedKind != "PERMANENT" {
		t.Fatalf("issued invoice without customer name err=%v kind=%s", err, unnamed.failedKind)
	}
}
