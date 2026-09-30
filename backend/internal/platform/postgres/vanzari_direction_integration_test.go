//go:build integration

package postgres

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
)

// D-124: an issued invoice keeps the client as UBL supplier and stores its
// customer; the business duplicate key (client, supplier, number, day) means the
// client's own number and date.
func TestIssuedInvoicePersistsCustomerFiltersByDirectionAndDetectsDuplicate(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	service := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })

	received, _, err := service.Ingest(ctx, ingestionFixture("received", clientID, now))
	if err != nil {
		t.Fatal(err)
	}
	if received.Direction != invoicing.DirectionIncoming || received.CustomerName != nil {
		t.Fatalf("received invoice = direction %s customer %v", received.Direction, received.CustomerName)
	}

	issuedInput := ingestionFixture("issued", clientID, now)
	own := "RO51741718"
	issuedInput.SupplierName, issuedInput.SupplierCUI, issuedInput.DocumentNumber = "Client pipeline test", &own, "VE18810064"
	issuedInput.Direction, issuedInput.CustomerName, issuedInput.CustomerIdentifier, issuedInput.CustomerCountry = invoicing.DirectionOutgoing, "PERSOANĂ FIZICĂ TEST", "1800101420010", "RO"
	issued, created, err := service.Ingest(ctx, issuedInput)
	if err != nil || !created {
		t.Fatalf("issued created=%v err=%v", created, err)
	}
	stored, err := store.GetInvoice(ctx, issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Direction != invoicing.DirectionOutgoing || stored.CustomerName == nil || *stored.CustomerName != "PERSOANĂ FIZICĂ TEST" ||
		stored.CustomerIdentifier == nil || *stored.CustomerIdentifier != "1800101420010" ||
		stored.NormalizedCustomerID == nil || *stored.NormalizedCustomerID != "1800101420010" ||
		stored.CustomerIdentifierKind == nil || *stored.CustomerIdentifierKind != "CNP" {
		t.Fatalf("issued invoice customer = %+v", stored)
	}
	if party := stored.Counterparty(); party.Name != "PERSOANĂ FIZICĂ TEST" {
		t.Fatalf("counterparty = %+v", party)
	}

	outgoing, err := store.ListInvoices(ctx, invoicing.Filter{ClientID: clientID, Direction: invoicing.DirectionOutgoing})
	if err != nil || len(outgoing) != 1 || outgoing[0].ID != issued.ID {
		t.Fatalf("outgoing list = %d err=%v", len(outgoing), err)
	}
	incoming, err := store.ListInvoices(ctx, invoicing.Filter{ClientID: clientID, Direction: invoicing.DirectionIncoming})
	if err != nil || len(incoming) != 1 || incoming[0].ID != received.ID {
		t.Fatalf("incoming list = %d err=%v", len(incoming), err)
	}
	all, err := store.ListInvoices(ctx, invoicing.Filter{ClientID: clientID})
	if err != nil || len(all) != 2 {
		t.Fatalf("all list = %d err=%v", len(all), err)
	}

	again := issuedInput
	again.ID, again.SPVReference, again.ExternalDeliveryID = "issued-again", "SOURCE-issued-again", "delivery-issued-again"
	duplicate, _, err := service.Ingest(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.PipelineStatus != invoicing.StatusDuplicate || duplicate.DuplicateOfInvoiceID == nil || *duplicate.DuplicateOfInvoiceID != issued.ID {
		t.Fatalf("second copy of the issued invoice = %s duplicateOf=%v", duplicate.PipelineStatus, duplicate.DuplicateOfInvoiceID)
	}
}
