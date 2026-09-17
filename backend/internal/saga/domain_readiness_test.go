package saga

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"strings"
	"testing"
	"time"
)

func domainInvoice(t *testing.T) *invoicing.Invoice {
	t.Helper()
	f, l, p, pack := accountingtest.Fixture("A")
	cui := "RO-TEST-SUPPLIER"
	input := classification.InvoiceContext{ModelVersion: accounting.ModelVersion, ClientID: "A", IssueDate: "2026-09-15", SupplierID: cui, DocumentType: "INVOICE", Currency: "RON", SourceFacts: f, Snapshot: &accounting.Snapshot{Profile: p, Pack: pack}, Lines: []classification.LineContext{{ID: "line", Description: "TEST_ONLY service", VATRate: money.MustParse("21"), VATValue: money.MustParse("21"), SourceFacts: l}}}
	out, err := (classification.DomainPolicy{AllowTestOnly: true}).Evaluate(input)
	if err != nil {
		t.Fatal(err)
	}
	item := &invoicing.Invoice{ModelVersion: accounting.ModelVersion, ID: "TEST_ONLY-invoice", ClientID: "A", SupplierName: "TEST_ONLY supplier", SupplierCUI: &cui, DocumentNumber: "TEST_ONLY-001", IssueDate: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), DocumentType: invoicing.DocumentTypeInvoice, Total: money.Money{Amount: money.MustParse("121"), Currency: "RON"}, SourceFacts: f, AccountingSnapshot: out.Snapshot, PipelineStatus: invoicing.StatusReadyForSAGA, Lines: []invoicing.Line{{ID: "line", Position: 1, Description: "TEST_ONLY service", Unit: "H87", Quantity: money.MustParse("1"), UnitPrice: money.MustParse("100"), NetValue: money.MustParse("100"), VATRate: money.MustParse("21"), VATValue: money.MustParse("21"), TotalValue: money.MustParse("121"), SourceFacts: l}}}
	for _, p := range out.Proposals {
		if p.RequiresReview {
			t.Fatalf("fixture requires review %#v", p)
		}
		text := p.ProposedValue
		item.Lines[0].Classifications = append(item.Lines[0].Classifications, classification.Decision{ID: string(p.Dimension), ModelVersion: accounting.ModelVersion, InvoiceLineID: "line", Dimension: p.Dimension, TypedValue: p.TypedValue, ProposedTypedValue: p.TypedValue, EffectiveValue: &text, Status: classification.ReviewAccepted, Evidence: p.Evidence, Source: p.Source, Rule: p.Rule, PolicyVersion: out.PolicyVersion})
	}
	return item
}
func domainClient() ClientIdentity {
	return ClientIdentity{ID: "A", Name: "TEST_ONLY client", CUI: "RO-TEST-BUYER"}
}
func TestDomainOrdinaryReadinessAndDerivedXML(t *testing.T) {
	item := domainInvoice(t)
	r := EvaluateReadiness(item, domainClient(), true, false)
	if !r.Ready {
		t.Fatal(r.Reason)
	}
	a, err := GenerateTestOnly(item, domainClient())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(a.Payload), "<Cont>628.TEST</Cont>") || strings.Contains(string(a.Payload), "TipDeducere") || a.ClassificationSnapshot["mapping_version"] == "" {
		t.Fatal("mapping output")
	}
	if _, err := Generate(item, domainClient()); err == nil {
		t.Fatal("synthetic map used by production")
	}
}
func TestDomainReadinessFalsePositiveAndGeneratorRecheck(t *testing.T) {
	item := domainInvoice(t)
	d := &item.Lines[0].Classifications[3]
	d.HumanReviewed = true
	d.ReviewReason = "TEST_ONLY accountant correction"
	d.TypedValue = &accounting.Value{Kind: "NONDEDUCTIBLE", Reason: "TEST_ONLY basis"}
	text := d.TypedValue.Text()
	d.EffectiveValue = &text
	if r := EvaluateReadiness(item, domainClient(), true, false); r.Ready || !strings.Contains(r.Reason, "Combinația") {
		t.Fatal(r)
	}
	if _, err := GenerateTestOnly(item, domainClient()); err == nil {
		t.Fatal("unsupported serialized")
	}
	item = domainInvoice(t)
	item.Lines[0].Description = "changed"
	item.Lines[0].SourceFacts.SellerItemID = "wrong"
	if EvaluateReadiness(item, domainClient(), true, false).Ready {
		t.Fatal("stale predicates accepted")
	}
	item = domainInvoice(t)
	item.Lines[0].Classifications[0].Evidence.ProfileVersion = 999
	if EvaluateReadiness(item, domainClient(), true, false).Ready {
		t.Fatal("corrupt profile accepted")
	}
}
func TestDomainMappingDisabledEvenAfterHumanConfirmation(t *testing.T) {
	item := domainInvoice(t)
	item.AccountingSnapshot.Pack.Mapping.Approved = false
	for i := range item.Lines[0].Classifications {
		item.Lines[0].Classifications[i].HumanReviewed = true
		item.Lines[0].Classifications[i].ReviewReason = "TEST_ONLY confirmation"
	}
	if EvaluateReadiness(item, domainClient(), true, false).Ready {
		t.Fatal("review promoted unapproved mapping")
	}
}
func TestDomainNoAdapterCodesInFiscalValues(t *testing.T) {
	for _, s := range []string{"N50", "I", "SAGA_DEFAULT"} {
		if (accounting.Value{Kind: s}).Validate("VAT_DEDUCTIBILITY") == nil {
			t.Fatal(s)
		}
	}
}

func TestDomainHumanAccountStillRequiresApprovedVocabulary(t *testing.T) {
	item := domainInvoice(t)
	d := &item.Lines[0].Classifications[0]
	d.TypedValue = &accounting.Value{Kind: "ACCOUNT", Account: "628.UNAPPROVED"}
	v := d.TypedValue.Text()
	d.EffectiveValue = &v
	d.HumanReviewed = true
	d.ReviewReason = "TEST_ONLY correction"
	if EvaluateReadiness(item, domainClient(), true, false).Ready {
		t.Fatal("unapproved analytic unlocked by review")
	}
}
