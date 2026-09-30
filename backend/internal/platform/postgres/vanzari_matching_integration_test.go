//go:build integration

package postgres

import (
	"fmt"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	entcontract "diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/fiscalidentity"
)

const vanzariClientCUI = "RO51741718"

func (tc *contractTestContext) createIssuedInvoice(t *testing.T, suffix, customerID string) string {
	t.Helper()
	id := fmt.Sprintf("issued-invoice-%s-%s", tc.clientID, suffix)
	kind, normalized := fiscalidentity.Classify(customerID, "RO")
	_, err := tc.store.Client.Invoice.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName("Client contabil").SetSupplierCui(vanzariClientCUI).
		SetNormalizedSupplierCui(fiscalidentity.ForComparison(vanzariClientCUI, "")).SetDirection(invoice.DirectionOUTGOING).
		SetCustomerName("Chiriaș " + suffix).SetCustomerIdentifier(customerID).SetNormalizedCustomerIdentifier(normalized).SetCustomerIdentifierKind(invoice.CustomerIdentifierKind(kind)).
		SetDocumentNumber("VE-" + suffix).SetNormalizedDocumentNumber("VE-" + suffix).
		SetIssueDate(tc.now).SetIssueDay(time.Date(tc.now.Year(), tc.now.Month(), tc.now.Day(), 0, 0, 0, 0, time.UTC)).
		SetTotalAmount("121.0000").SetCurrency("RON").SetSpvReference("SPV-" + id).SetIngestionSource("CONTRACT_TEST").
		SetExternalDeliveryID("DELIVERY-" + id).SetPipelineStatus(invoice.PipelineStatusMATCHING).SetSagaStatus(invoice.SagaStatusNOT_READY).
		SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	tc.ids = append(tc.ids, id)
	return id
}

// createSaleContract stores a contract where the client is the supplier
// (Locator) and the given party is the buyer (Locatar).
func (tc *contractTestContext) createSaleContract(t *testing.T, suffix, buyerID, currency string, to time.Time) string {
	t.Helper()
	id := fmt.Sprintf("sale-contract-%s-%s", tc.clientID, suffix)
	_, normalized := fiscalidentity.Classify(buyerID, "RO")
	_, err := tc.store.Client.Contract.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName("Client contabil").SetSupplierCui(vanzariClientCUI).
		SetNormalizedSupplierCui(fiscalidentity.ForComparison(vanzariClientCUI, "")).SetClientRole(entcontract.ClientRoleSUPPLIER).
		SetBuyerName("Chiriaș " + suffix).SetBuyerCui(buyerID).SetNormalizedBuyerCui(normalized).SetReference("LOC-" + suffix).
		SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetEffectiveTo(to).
		SetTotalValue("2000.0000").SetCurrency(currency).SetUnitType("EVENIMENT").SetPaymentTerms("avans 50%").
		SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	tc.contractIDs = append(tc.contractIDs, id)
	return id
}

func (tc *contractTestContext) assertContinuedWithoutTask(t *testing.T, invoiceID string) {
	t.Helper()
	row, err := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	if err != nil || row.PipelineStatus != invoice.PipelineStatusDEDUPE_CHECKED {
		t.Fatalf("issued invoice status=%v err=%v", row, err)
	}
	if tasks, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).Count(tc.ctx); tasks != 0 {
		t.Fatalf("issued invoice has %d tasks", tasks)
	}
	if pending, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID), outboxentry.StatusEQ(outboxentry.StatusPENDING)).Count(tc.ctx); pending != 1 {
		t.Fatalf("issued invoice continuations=%d", pending)
	}
}

func TestIssuedInvoiceLinksUniqueSaleContractByCustomerIgnoringCurrency(t *testing.T) {
	tc := newContractTestContext(t)
	contractID := tc.createSaleContract(t, "event", "RO40138380", "EUR", time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	tc.createSaleContract(t, "other-tenant", "RO16193331", "EUR", time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	invoiceID := tc.createIssuedInvoice(t, "event", "40138380")
	decision, changed, err := tc.match(t, invoiceID, "issued-unique-"+invoiceID)
	if err != nil || !changed || decision.Outcome != contracts.OutcomeUniqueCompatible || decision.PolicyVersion != contracts.OutgoingContextPolicyVersion {
		t.Fatalf("decision=%+v changed=%v err=%v", decision, changed, err)
	}
	association, err := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Only(tc.ctx)
	if err != nil || association.ContractID != contractID {
		t.Fatalf("association=%+v err=%v", association, err)
	}
	tc.assertContinuedWithoutTask(t, invoiceID)
}

func TestIssuedInvoiceWithoutOrWithSeveralContractsContinuesUnlinked(t *testing.T) {
	tc := newContractTestContext(t)
	expired := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	tc.createSaleContract(t, "expired", "RO26999270", "EUR", expired)
	withoutID := tc.createIssuedInvoice(t, "without", "RO26999270")
	decision, _, err := tc.match(t, withoutID, "issued-none-"+withoutID)
	if err != nil || decision.Outcome != contracts.OutcomeNoMatch {
		t.Fatalf("expired-only decision=%+v err=%v", decision, err)
	}
	tc.assertContinuedWithoutTask(t, withoutID)
	if audits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(withoutID), activityevent.EventTypeEQ("CONTRACT_NOT_REQUIRED_OUTGOING")).Count(tc.ctx); audits != 1 {
		t.Fatalf("not-required audits=%d", audits)
	}

	far := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	tc.createSaleContract(t, "first", "31482767", "EUR", far)
	tc.createSaleContract(t, "second", "RO31482767", "RON", far)
	severalID := tc.createIssuedInvoice(t, "several", "31482767")
	decision, _, err = tc.match(t, severalID, "issued-several-"+severalID)
	if err != nil || decision.Outcome != contracts.OutcomeMultiplePlausible {
		t.Fatalf("several decision=%+v err=%v", decision, err)
	}
	tc.assertContinuedWithoutTask(t, severalID)
	if links, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(severalID)).Count(tc.ctx); links != 0 {
		t.Fatalf("several candidates must not link, links=%d", links)
	}
}

// A sale contract names the client as supplier; it must never be a candidate
// for, nor resume, a purchase invoice from the same CUI.
func TestSaleContractNeverMatchesOrResumesPurchaseInvoices(t *testing.T) {
	tc := newContractTestContext(t)
	saleID := tc.createSaleContract(t, "lease", "RO40138380", "RON", time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	purchaseID, _ := tc.createMissingContractInvoice(t, "purchase", vanzariClientCUI, "RON", false)
	_, candidates, err := tc.store.LoadMatchingInput(tc.ctx, purchaseID)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("purchase candidates=%d err=%v", len(candidates), err)
	}
	blocked, err := tc.store.ListBlockedInvoicesForContract(tc.ctx, saleID, "", 10)
	if err != nil || len(blocked) != 0 {
		t.Fatalf("blocked invoices for sale contract=%d err=%v", len(blocked), err)
	}
}
