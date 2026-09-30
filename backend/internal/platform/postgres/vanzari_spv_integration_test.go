//go:build integration

package postgres

import (
	"archive/zip"
	"bytes"
	"context"
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

// D-125: an invoice the client issued to a natural person arrives through the
// sent list (filter T) and enters Diana as OUTGOING with its customer.
func TestSentInvoiceFromSPVIsIngestedAsOutgoing(t *testing.T) {
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
	clientID, connectionID := "fake-anaf-sales-client", "fake-anaf-sales-connection"
	cleanup := func() {
		ids, _ := store.Client.Invoice.Query().Where(invoice.ClientIDEQ(clientID)).IDs(ctx)
		if len(ids) > 0 {
			_, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDIn(ids...)).Exec(ctx)
		}
		_, _ = store.Client.SPVSourceDocument.Delete().Where(spvsourcedocument.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.Invoice.Delete().Where(invoice.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.SPVConnection.Delete().Where(spvconnection.ClientIDEQ(clientID)).Exec(ctx)
		_, _ = store.Client.AccountingClient.Delete().Where(accountingclient.IDEQ(clientID)).Exec(ctx)
	}
	cleanup()
	defer cleanup()
	if _, err = store.Client.AccountingClient.Create().SetID(clientID).SetName("Client vânzări fake ANAF").SetCui("RO990005").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Client.SPVConnection.Create().SetID(connectionID).SetClientID(clientID).SetCif("RO990005").SetEnvironment(spvconnection.EnvironmentTEST).SetAccessTokenCiphertext("token").SetRefreshTokenCiphertext("refresh").SetAccessTokenExpiresAt(now.Add(time.Hour)).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	xml := `<Invoice><ID>FAKE-ANAF-VE-1</ID><IssueDate>2026-09-14</IssueDate><DocumentCurrencyCode>RON</DocumentCurrencyCode><AccountingSupplierParty><Party><PartyLegalEntity><RegistrationName>Client vânzări fake ANAF</RegistrationName></PartyLegalEntity><PartyTaxScheme><CompanyID>RO990005</CompanyID></PartyTaxScheme></Party></AccountingSupplierParty><AccountingCustomerParty><Party><PartyLegalEntity><RegistrationName>PERSOANĂ FIZICĂ TEST</RegistrationName><CompanyID>1800101420010</CompanyID></PartyLegalEntity></Party></AccountingCustomerParty><LegalMonetaryTotal><TaxInclusiveAmount>121</TaxInclusiveAmount></LegalMonetaryTotal><InvoiceLine><ID>1</ID><InvoicedQuantity unitCode="H87">1</InvoicedQuantity><LineExtensionAmount>100</LineExtensionAmount><Item><Name>shooting</Name><ClassifiedTaxCategory><Percent>21</Percent></ClassifiedTaxCategory></Item><Price><PriceAmount>100</PriceAmount></Price></InvoiceLine></Invoice>`
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
	filters := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/listaMesajePaginatieFactura":
			filters = append(filters, r.URL.Query().Get("filtru"))
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("filtru") == "T" {
				_, _ = w.Write([]byte(`{"mesaje":[{"id":"80001","id_solicitare":"81001","tip":"FACTURA TRIMISA","data_creare":"202609141200"}],"numar_total_pagini":1}`))
				return
			}
			_, _ = w.Write([]byte(`{"mesaje":[],"numar_total_pagini":1,"eroare":"Nu exista mesaje in intervalul selectat"}`))
		case "/descarcare":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buffer.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	pipeline := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	service := spv.NewService(store, spv.NewHTTPClient(server.Client(), server.URL, server.URL), spv.UBLParser{}, pipeline, clearTestCipher{}, spv.ServiceConfig{InitialWindow: 60 * 24 * time.Hour, Overlap: 72 * time.Hour})
	result, err := service.Sync(ctx, connectionID)
	if err != nil || len(result.Documents) != 1 || len(filters) != 2 || filters[0] != "P" || filters[1] != "T" {
		t.Fatalf("sync=%+v filters=%v err=%v", result, filters, err)
	}
	invoiceID, created, err := service.ProcessDocument(ctx, result.Documents[0].ID, "integration-worker")
	if err != nil || !created {
		t.Fatalf("process id=%s created=%t err=%v", invoiceID, created, err)
	}
	item, err := store.GetInvoice(ctx, invoiceID)
	if err != nil || item.Direction != invoicing.DirectionOutgoing || item.CustomerName == nil || *item.CustomerName != "PERSOANĂ FIZICĂ TEST" || item.CustomerIdentifierKind == nil || *item.CustomerIdentifierKind != "CNP" {
		t.Fatalf("issued invoice = %+v err=%v", item, err)
	}
	source, err := store.Client.SPVSourceDocument.Get(ctx, result.Documents[0].ID)
	if err != nil || source.MessageType == nil || *source.MessageType != "FACTURA TRIMISA" {
		t.Fatalf("source message type = %v err=%v", source, err)
	}
	connection, err := store.GetConnection(ctx, connectionID)
	if err != nil || connection.LastSuccessfulSentSyncAt == nil {
		t.Fatalf("sent cursor not recorded: %+v err=%v", connection, err)
	}
}
