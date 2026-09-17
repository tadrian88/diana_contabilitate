//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/internal/invoicing"
)

func TestStoreAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	clientID := "integration-client"
	invoiceID := "integration-invoice"
	eventID := "integration-event"
	defer func() {
		_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.IDEQ(eventID)).Exec(ctx)
		_, _ = store.Client.Invoice.Delete().Where(invoice.IDEQ(invoiceID)).Exec(ctx)
		_ = store.Client.AccountingClient.DeleteOneID(clientID).Exec(ctx)
	}()

	now := time.Date(2026, time.September, 11, 10, 0, 0, 0, time.UTC)
	if _, err := store.Client.AccountingClient.Create().SetID(clientID).SetName("Client integrare").SetCui("RO-INTEGRATION-001").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	created, err := store.Client.Invoice.Create().SetID(invoiceID).SetClientID(clientID).
		SetSupplierName("Furnizor integrare").SetDocumentNumber("INT-001").SetNormalizedDocumentNumber("INT-001").
		SetIssueDate(now).SetIssueDay(time.Date(2026, time.September, 11, 0, 0, 0, 0, time.UTC)).
		SetTotalAmount("123456789012345.6789").SetCurrency("RON").SetSpvReference("SPV-INT-001").
		SetIngestionSource("INTEGRATION_TEST").SetExternalDeliveryID("delivery-integration-1").
		SetPipelineStatus(invoice.PipelineStatusDOWNLOADED).SetSagaStatus(invoice.SagaStatusNOT_READY).
		SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := "Sistem integrare"
	if _, err := store.Client.ActivityEvent.Create().SetID(eventID).SetClientID(clientID).SetInvoiceID(invoiceID).
		SetAggregateType("INVOICE").SetAggregateID(invoiceID).SetEventType("Factură persistată").SetOccurredAt(now).
		SetActorKind(activityevent.ActorKindSYSTEM).SetActorDisplay(actor).SetAutomatic(true).SetDetail("Eveniment de integrare.").Save(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetInvoice(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Total.Amount.String() != "123456789012345.6789" {
		t.Fatalf("money lost precision: %s", loaded.Total.Amount)
	}
	if loaded.ClientID != clientID || len(loaded.Activity) != 1 {
		t.Fatalf("relationship/audit not loaded: %+v", loaded)
	}
	listed, err := store.ListInvoices(ctx, invoicing.Filter{ClientID: clientID})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != invoiceID || len(listed[0].Activity) != 1 {
		t.Fatalf("complete scoped invoice list mismatch: %+v", listed)
	}
}

func TestInvoiceForeignKeyIsEnforced(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	_, err = store.Client.Invoice.Create().SetID("invalid-client-invoice").SetClientID("does-not-exist").
		SetSupplierName("Furnizor").SetDocumentNumber("INVALID").SetNormalizedDocumentNumber("INVALID").
		SetIssueDate(now).SetIssueDay(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)).
		SetTotalAmount("1.0000").SetCurrency("RON").SetSpvReference("INVALID").
		SetIngestionSource("INTEGRATION_TEST").SetExternalDeliveryID("invalid-client").
		SetPipelineStatus(invoice.PipelineStatusDOWNLOADED).SetSagaStatus(invoice.SagaStatusNOT_READY).
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err == nil {
		t.Fatal("expected client foreign key violation")
	}
}
