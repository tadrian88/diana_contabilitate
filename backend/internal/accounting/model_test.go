package accounting_test

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/money"
	"testing"
)

func TestDomainTypedValues(t *testing.T) {
	cases := []struct {
		d     string
		v     accounting.Value
		valid bool
	}{
		{"ACCOUNT", accounting.Value{Kind: "ACCOUNT", Account: "628.01"}, true},
		{"VAT_DEDUCTIBILITY", accounting.Value{Kind: "whatever"}, false},
		{"VAT_DEDUCTIBILITY", accounting.Value{Kind: "LIMITED", Percentage: accountingtest.Rate("50")}, false},
		{"VAT_DEDUCTIBILITY", accounting.Value{Kind: "LIMITED", Percentage: accountingtest.Rate("50"), Basis: "TEST_ONLY usage evidence"}, true},
		{"VAT_DEDUCTIBILITY", accounting.Value{Kind: "LIMITED", Percentage: accountingtest.Rate("100"), Basis: "test"}, false},
		{"VAT_DEDUCTIBILITY", accounting.Value{Kind: "NOT_APPLICABLE"}, false},
		{"VAT_DEDUCTIBILITY", accounting.Value{Kind: "NOT_APPLICABLE", Reason: "No relevant input VAT"}, true},
		{"EXPENSE_TAX_TREATMENT", accounting.Value{Kind: "PERIOD_LIMIT_CATEGORY", Category: "TEST_ONLY_PROTOCOL", Basis: "TEST_ONLY article"}, true},
		{"EXPENSE_TAX_TREATMENT", accounting.Value{Kind: "PERIOD_LIMIT_CATEGORY", Category: "protocol", Percentage: accountingtest.Rate("50"), Basis: "test"}, false},
		{"VAT_TREATMENT", accounting.Value{Kind: "ORDINARY", Timing: "IMMEDIATE", SourceCategory: "S", SourceRate: accountingtest.Rate("21")}, true},
		{"EXPENSE_TAX_TREATMENT", accounting.Value{Kind: "FULLY_DEDUCTIBLE", SourceRate: accountingtest.Rate("21")}, false},
	}
	for _, c := range cases {
		t.Run(c.d+"/"+c.v.Kind, func(t *testing.T) {
			if (c.v.Validate(c.d) == nil) != c.valid {
				t.Fatalf("unexpected validity %#v", c.v)
			}
		})
	}
	if _, err := accounting.DecodeValue([]byte(`{"kind":"FULL","saga":"N50"}`), "VAT_DEDUCTIBILITY"); err == nil {
		t.Fatal("adapter fields accepted")
	}
}
func TestDomainClientProfileDatesAndUnknown(t *testing.T) {
	_, _, p, pack := accountingtest.Fixture("A")
	if !p.Valid("A", "2026-09-15") || p.Valid("B", "2026-09-15") || p.Valid("A", "2025-12-31") {
		t.Fatal("profile scope/date")
	}
	p.VATRegistration = "UNKNOWN"
	if p.Ordinary() {
		t.Fatal("unknown became ordinary")
	}
	if pack.Valid(p, "A", "2026-09-15", false) {
		t.Fatal("synthetic production activation")
	}
}
func TestDomainSourceReconciliation(t *testing.T) {
	f, l, _, _ := accountingtest.Fixture("A")
	lines := []accounting.SourceLine{{Facts: l, Net: money.MustParse("100"), VAT: money.MustParse("21"), Total: money.MustParse("121"), Rate: money.MustParse("21")}}
	if err := accounting.Reconcile(f, lines, money.MustParse("121"), "RON"); err != nil {
		t.Fatal(err)
	}
	f.Subtotals[0].VAT.Amount = money.MustParse("20.99")
	if accounting.Reconcile(f, lines, money.MustParse("121"), "RON") == nil {
		t.Fatal("category discrepancy accepted")
	}
	f.Subtotals[0].VAT.Amount = money.MustParse("21")
	l.Rate = nil
	if accounting.Reconcile(f, lines, money.MustParse("121"), "RON") == nil {
		t.Fatal("missing became zero")
	}
}

func TestDomainAccountVocabularyCannotBeInferred(t *testing.T) {
	_, _, p, pack := accountingtest.Fixture("A")
	p.AccountCodes = nil
	if p.Ordinary() || pack.Valid(p, "A", "2026-09-15", true) {
		t.Fatal("missing chart vocabulary accepted")
	}
	p.AccountCodes = []string{"628.TEST"}
	if p.AccountAllowed("628.UNAPPROVED") {
		t.Fatal("arbitrary analytic accepted")
	}
	pack.Rules[0].Result.Account = "628.UNAPPROVED"
	if pack.Valid(p, "A", "2026-09-15", true) {
		t.Fatal("rule outside approved vocabulary")
	}
}
