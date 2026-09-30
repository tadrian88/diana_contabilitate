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

func TestProfileSuccessionKeepsHistoricalVersionButSelectsSuccessor(t *testing.T) {
	_, _, original, _ := accountingtest.Fixture("A")
	successor := *original
	successor.ID = "profile-successor"
	successor.Version = original.Version + 1
	successor.SupersedesProfileID = original.ID
	selected := accounting.ApplicableProfiles([]*accounting.Profile{original, &successor}, "A", "2026-09-15")
	if len(selected) != 1 || selected[0].ID != successor.ID {
		t.Fatalf("selected=%#v", selected)
	}
	if original.ID == successor.ID || original.SupersedesProfileID != "" {
		t.Fatal("historical profile was mutated")
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
	if !p.AccountAllowed("6022") || p.RuleAccountAllowed("6022") {
		t.Fatal("empty list must allow reviewed decisions over the global catalog but never rule automation")
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

func TestAccountingClassificationResolutionSeparatesAIReviewFromFinal(t *testing.T) {
	account := &accounting.Value{Kind: "ACCOUNT", Account: "626"}
	if got := accounting.ResolveClassification("ACCOUNT", nil, nil, "NO_MATCH", "PENDING"); got != accounting.ResolutionNeedsAI {
		t.Fatalf("got %s", got)
	}
	if got := accounting.ResolveClassification("ACCOUNT", nil, account, "AI_PROPOSAL", "PENDING"); got != accounting.ResolutionNeedsReview {
		t.Fatalf("got %s", got)
	}
	if got := accounting.ResolveClassification("ACCOUNT", account, account, "AI_PROPOSAL", "ACCEPTED"); got != accounting.ResolutionFinal {
		t.Fatalf("got %s", got)
	}
	items := []accounting.ClassificationResolutionItem{{Dimension: "ACCOUNT", Effective: account, Proposed: account, Source: "AI_PROPOSAL", ReviewStatus: "ACCEPTED"}}
	if !accounting.IsAccountingClassificationComplete(items, 1) {
		t.Fatal("accepted typed decision must be complete")
	}
	items[0].Effective = nil
	if accounting.IsAccountingClassificationComplete(items, 1) {
		t.Fatal("pending AI proposal must not be complete")
	}
}
