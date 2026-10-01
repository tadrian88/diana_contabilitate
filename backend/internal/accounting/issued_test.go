package accounting_test

import (
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/money"
)

// D-129: an issued invoice may name its customer only by CNP (natural person)
// or by a CUI without RO, and carries the client's own cash-accounting note.
func TestIssuedReconciliationAcceptsCNPCustomerAndIgnoresOwnCashNote(t *testing.T) {
	f, l, _, _ := accountingtest.Fixture("A")
	lines := []accounting.SourceLine{{Facts: l, Net: money.MustParse("100"), VAT: money.MustParse("21"), Total: money.MustParse("121"), Rate: money.MustParse("21")}}
	f.BuyerVATID, f.BuyerLegalID, f.CashAccounting = "", "1800101420010", "YES"
	if err := accounting.ReconcileIssued(f, lines, money.MustParse("121"), "RON"); err != nil {
		t.Fatalf("issued invoice rejected: %v", err)
	}
	if accounting.Reconcile(f, lines, money.MustParse("121"), "RON") == nil {
		t.Fatal("a received invoice must still require the buyer VAT ID and no cash accounting")
	}
	f.BuyerLegalID = ""
	if accounting.ReconcileIssued(f, lines, money.MustParse("121"), "RON") == nil {
		t.Fatal("an issued invoice without any customer identifier was accepted")
	}
}

func TestDirectionDerivedValuesAreNotApplicable(t *testing.T) {
	for _, dimension := range []string{"VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT"} {
		value, ok := accounting.DirectionDerivedValue(dimension)
		if !ok || value.Kind != "NOT_APPLICABLE" || value.Validate(dimension) != nil {
			t.Fatalf("%s = %#v ok=%v", dimension, value, ok)
		}
	}
	for _, dimension := range []string{"ACCOUNT", "VAT_TREATMENT"} {
		if _, ok := accounting.DirectionDerivedValue(dimension); ok {
			t.Fatalf("%s must be decided, not derived", dimension)
		}
	}
}

// D-132: a line may differ from net × rate by one cent of category rounding;
// more than that is still refused, and totals stay exact.
func TestReconciliationAllowsOneCentOfLineVATRounding(t *testing.T) {
	reconcile := func(net, vat, total string) error {
		f, l, _, _ := accountingtest.Fixture("A")
		amount := func(value string) *accounting.AmountFact {
			return &accounting.AmountFact{Amount: money.MustParse(value), Currency: "RON", Origin: accounting.Declared}
		}
		l.NetAmount, l.VATAmount, l.PriceAmount = nil, nil, nil
		f.LineExtension, f.TaxExclusive, f.TaxInclusive = amount(net), amount(net), amount(total)
		f.VATTotals = []accounting.AmountFact{*amount(vat)}
		f.Subtotals[0].Base, f.Subtotals[0].VAT = *amount(net), *amount(vat)
		lines := []accounting.SourceLine{{Facts: l, Net: money.MustParse(net), VAT: money.MustParse(vat), Total: money.MustParse(total), Rate: money.MustParse("21")}}
		return accounting.Reconcile(f, lines, money.MustParse(total), "RON")
	}
	if err := reconcile("19866.17", "4171.89", "24038.06"); err != nil {
		t.Fatalf("one cent of rounding rejected: %v", err)
	}
	if err := reconcile("19866.17", "4171.87", "24038.04"); err == nil {
		t.Fatal("three cents accepted as rounding")
	}
}
