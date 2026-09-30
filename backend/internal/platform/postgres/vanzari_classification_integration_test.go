//go:build integration

package postgres

import (
	"testing"
	"time"

	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
)

// D-129: the DIRECTION decisions of an issued invoice satisfy the database
// classification shape; ACCOUNT and VAT_TREATMENT wait for the accountant.
func TestIssuedInvoicePersistsDirectionDerivedDecisions(t *testing.T) {
	tc := newModule5TestContext(t)
	f, l, _, _ := domainReleaseFixture(t, tc, nil)
	client, err := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	if err != nil {
		t.Fatal(err)
	}
	f.SupplierVATID, f.BuyerVATID, f.BuyerLegalID = client.Cui, "", "1800101420010"
	id := "domain-" + tc.clientID + "-issued"
	if _, err = tc.store.Client.Invoice.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName(client.Name).SetSupplierCui(client.Cui).SetNormalizedSupplierCui(invoicing.NormalizeBusinessIdentifier(client.Cui)).
		SetDirection(invoice.DirectionOUTGOING).SetCustomerName("PERSOANĂ FIZICĂ TEST").SetCustomerIdentifier("1800101420010").SetNormalizedCustomerIdentifier("1800101420010").SetCustomerIdentifierKind(invoice.CustomerIdentifierKindCNP).
		SetDocumentNumber(id).SetNormalizedDocumentNumber(id).SetIssueDate(tc.now).SetIssueDay(invoicing.InvoiceIssueDay(tc.now)).SetTotalAmount("121").SetCurrency("RON").SetSpvReference(id).SetIngestionSource("TEST_ONLY").SetExternalDeliveryID(id).
		SetModelVersion(accounting.ModelVersion).SetSourceFacts(f).SetPipelineStatus(invoice.PipelineStatusCOMMERCIALLY_VALIDATED).SetSagaStatus(invoice.SagaStatusNOT_READY).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	tc.invoices = append(tc.invoices, id)
	if _, err = tc.store.Client.InvoiceLine.Create().SetID(id + "-line").SetInvoiceID(id).SetPosition(1).SetDescription("chirie lunara birouri").SetUnit("H87").SetQuantity("1").SetUnitPrice("100").SetNetValue("100").SetVatRate("21").SetVatValue("21").SetTotalValue("121").SetSourceFacts(l).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	service := classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })
	if _, _, err = service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id + ":classify"}); err != nil {
		t.Fatal(err)
	}
	rows, err := tc.store.Client.LineClassification.Query().Where(lineclassification.InvoiceIDEQ(id)).All(tc.ctx)
	if err != nil || len(rows) != 4 {
		t.Fatalf("classifications=%d err=%v", len(rows), err)
	}
	for _, row := range rows {
		switch row.Dimension {
		case lineclassification.DimensionVAT_DEDUCTIBILITY, lineclassification.DimensionEXPENSE_TAX_TREATMENT:
			if row.Source != lineclassification.SourceDIRECTION || row.EffectiveSource == nil || *row.EffectiveSource != "DIRECTION" || row.EffectiveTypedValue == nil || row.EffectiveTypedValue.Kind != "NOT_APPLICABLE" {
				t.Fatalf("%s = source %s effective %v", row.Dimension, row.Source, row.EffectiveTypedValue)
			}
		default:
			if row.Source == lineclassification.SourceDIRECTION || row.EffectiveTypedValue != nil {
				t.Fatalf("%s must wait for a decision: source %s", row.Dimension, row.Source)
			}
		}
	}
	item, err := tc.store.GetInvoice(tc.ctx, id)
	if err != nil || item.PipelineStatus != invoicing.StatusAwaitingReview {
		t.Fatalf("issued invoice status=%v err=%v", item, err)
	}
}
