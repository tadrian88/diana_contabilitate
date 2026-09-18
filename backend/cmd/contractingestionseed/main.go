// contractingestionseed creates synthetic fixtures in an explicitly selected test database.
// It never deletes or resets existing data.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"diana-contabilitate/backend/ent/accountingclient"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/platform/postgres"
)

func main() {
	if os.Getenv("APP_ENV") != "test" || os.Getenv("DATABASE_URL") == "" {
		fmt.Fprintln(os.Stderr, "APP_ENV=test and explicit DATABASE_URL required; use an isolated test database")
		os.Exit(1)
	}
	store, err := postgres.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer store.Close()
	ctx := context.Background()
	exists, err := store.Client.AccountingClient.Query().Where(accountingclient.IDEQ("client-contract-ingestion")).Exist(ctx)
	if err != nil {
		panic(err)
	}
	if exists {
		fmt.Fprintln(os.Stderr, "Fixture already exists. Use a fresh isolated database; no data was deleted.")
		os.Exit(1)
	}
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	_, err = store.Client.AccountingClient.Create().SetID("client-contract-ingestion").SetName("Contract Ingestion Test SRL").SetCui("RO10000000").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		panic(err)
	}
	_, err = store.Client.Invoice.Create().SetID("inv-contract-ingestion-waiting").SetClientID("client-contract-ingestion").SetSupplierName("Furnizor extras SRL").SetSupplierCui("RO12345678").SetNormalizedSupplierCui("12345678").SetDocumentNumber("CI-TEST-001").SetNormalizedDocumentNumber("CI-TEST-001").SetIssueDate(now).SetIssueDay(now).SetTotalAmount("1190.0000").SetCurrency("RON").SetSpvReference("CI-TEST-SPV-001").SetIngestionSource("CONTRACT_INGESTION_TEST").SetExternalDeliveryID("CI-TEST-001").SetPipelineStatus(invoice.PipelineStatusMATCHING).SetSagaStatus(invoice.SagaStatusNOT_READY).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		panic(err)
	}
	_, err = store.Client.InvoiceLine.Create().SetID("line-contract-ingestion-test").SetInvoiceID("inv-contract-ingestion-waiting").SetPosition(1).SetDescription("Servicii test sintetice").SetUnit("servicii").SetVatRate("19.0000").SetVatValue("190.0000").SetQuantity("1.0000").SetUnitPrice("1000.0000").SetNetValue("1000.0000").SetTotalValue("1190.0000").Save(ctx)
	if err != nil {
		panic(err)
	}
	_, _, err = contracts.NewService(store, contracts.BaselinePolicy{}, func() time.Time { return now }).MatchInvoice(ctx, contracts.MatchCommand{InvoiceID: "inv-contract-ingestion-waiting", ExpectedRevision: 1, CommandID: "ci-test-missing"})
	if err != nil {
		panic(err)
	}
	fmt.Println("Synthetic contract ingestion fixtures created; no existing rows modified.")
}
