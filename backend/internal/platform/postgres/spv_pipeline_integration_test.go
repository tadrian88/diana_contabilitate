//go:build integration

package postgres

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/accountingclient"
	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoiceline"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/spvconnection"
	"diana-contabilitate/backend/ent/spvsourcedocument"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/spv"
)

type clearTestCipher struct{}

func (clearTestCipher) Encrypt(v string) (string, error) { return v, nil }
func (clearTestCipher) Decrypt(v string) (string, error) { return v, nil }

func TestFakeANAFToDianaPipelineArchivesSourceAndEntersNativeWorkflow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	clientID := "fake-anaf-client"
	connectionID := "fake-anaf-connection"
	var importedInvoiceID string
	defer func() {
		if importedInvoiceID != "" {
			_, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDEQ(importedInvoiceID)).Exec(ctx)
			_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDEQ(importedInvoiceID)).Exec(ctx)
			_, _ = store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDEQ(importedInvoiceID)).Exec(ctx)
		}
		_, _ = store.Client.SPVSourceDocument.Delete().Where(spvsourcedocument.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.Invoice.Delete().Where(invoice.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.SPVConnection.Delete().Where(spvconnection.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.AccountingClient.Delete().Where(accountingclient.IDEQ(clientID)).Exec(ctx)
	}()
	_, err = store.Client.AccountingClient.Create().SetID(clientID).SetName("Client fake ANAF").SetCui("RO990002").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Client.SPVConnection.Create().SetID(connectionID).SetClientID(clientID).SetCif("RO990002").SetEnvironment(spvconnection.EnvironmentTEST).SetAccessTokenCiphertext("token").SetRefreshTokenCiphertext("refresh").SetAccessTokenExpiresAt(now.Add(time.Hour)).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw := fakeANAFZIP(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/listaMesajePaginatieFactura":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"mesaje":[{"id":"70001","id_solicitare":"60001","tip":"FACTURA PRIMITA","data_creare":"202609141200"}],"numar_total_pagini":1}`))
		case "/descarcare":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	pipeline := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	service := spv.NewService(store, spv.NewHTTPClient(server.Client(), server.URL, server.URL), spv.UBLParser{}, pipeline, clearTestCipher{}, spv.ServiceConfig{InitialWindow: 60 * 24 * time.Hour, Overlap: 72 * time.Hour})
	result, err := service.Sync(ctx, connectionID)
	if err != nil || len(result.Documents) != 1 {
		t.Fatalf("sync=%+v err=%v", result, err)
	}
	invoiceID, created, err := service.ProcessDocument(ctx, result.Documents[0].ID, "integration-worker")
	importedInvoiceID = invoiceID
	if err != nil || !created {
		t.Fatalf("process id=%s created=%t err=%v", invoiceID, created, err)
	}
	row, err := store.Client.SPVSourceDocument.Get(ctx, result.Documents[0].ID)
	if err != nil || row.ProcessingStatus != spvsourcedocument.ProcessingStatusPROCESSED || len(row.RawDocument) == 0 || row.ContentSha256 == nil || row.InvoiceID == nil {
		t.Fatalf("source not archived: %+v err=%v", row, err)
	}
	item, err := store.GetInvoice(ctx, invoiceID)
	if err != nil || item.PipelineStatus != invoicing.StatusDownloaded || len(item.Lines) != 1 {
		t.Fatalf("invoice did not enter native workflow: %+v err=%v", item, err)
	}
	entries, err := store.PendingOutbox(ctx, 10, now)
	if err != nil || len(entries) != 1 {
		t.Fatalf("outbox=%d err=%v", len(entries), err)
	}
	if _, err = pipeline.ProcessContinuation(ctx, entries[0]); err != nil {
		t.Fatal(err)
	}
	item, err = store.GetInvoice(ctx, invoiceID)
	if err != nil || item.PipelineStatus != invoicing.StatusArchived {
		t.Fatalf("native continuation failed: %+v %v", item, err)
	}
}

func fakeANAFZIP(t *testing.T) []byte {
	t.Helper()
	xml := fmt.Sprintf(`<Invoice><ID>FAKE-ANAF-1</ID><IssueDate>2026-09-14</IssueDate><DocumentCurrencyCode>RON</DocumentCurrencyCode><AccountingSupplierParty><Party><PartyLegalEntity><RegistrationName>Furnizor Fake</RegistrationName></PartyLegalEntity><PartyTaxScheme><CompanyID>RO990003</CompanyID></PartyTaxScheme></Party></AccountingSupplierParty><AccountingCustomerParty><Party><PartyTaxScheme><CompanyID>RO990002</CompanyID></PartyTaxScheme></Party></AccountingCustomerParty><LegalMonetaryTotal><TaxInclusiveAmount>119</TaxInclusiveAmount></LegalMonetaryTotal><InvoiceLine><ID>1</ID><InvoicedQuantity unitCode="H87">1</InvoicedQuantity><LineExtensionAmount>100</LineExtensionAmount><Item><Name>Linie fake</Name><ClassifiedTaxCategory><Percent>19</Percent></ClassifiedTaxCategory></Item><Price><PriceAmount>100</PriceAmount></Price></InvoiceLine></Invoice>`)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("invoice.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte(xml))
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
