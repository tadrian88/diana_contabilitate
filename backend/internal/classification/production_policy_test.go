package classification

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/rules"
)

// Fixtures model reviewed source-rate confirmation only. Periods/configuration
// are synthetic engineering assumptions and are NOT shipped fiscal rules.
func productionVATFixture() RuleCandidate {
	value := "21"
	return RuleCandidate{RuleID: "rate", RuleVersionID: "rate-v1", Reference: "TEST-RATE", Version: 1, Category: rules.CategoryVAT, Scope: rules.ScopeGlobal, Result: value, MatchKind: rules.MatchVATSourceRateEquals, MatchValue: &value, Explanation: "Confirmare a cotei declarate; nu concluzie asupra tratamentului fiscal.", LegalBasis: "Fixture: Legea 141/2025, art. II, modificarea art. 291; doar confirmare sursă", ProductionEligible: true, RulePackVersion: "TEST_ONLY_NOT_A_PRODUCTION_PACK", EffectiveFrom: "2025-08-01", Provenance: &rules.Provenance{SourceType: "LEGISLATION", SourceTitle: "Legea 141/2025", Issuer: "Parlamentul României", LegalInstrument: "Legea 141/2025", Reference: "art. II / art. 291", SourceURL: "https://legislatie.just.ro/Public/DetaliiDocument/300022", EffectiveFrom: "2025-08-01", VerifiedAt: "2026-09-15", VerifiedBy: "Fixture reviewer", Notes: "Explicit synthetic engineering fixture; not approved production accounting policy."}}
}
func productionInput(candidates ...RuleCandidate) InvoiceContext {
	return InvoiceContext{ID: "invoice", ClientID: "client", PipelineStatus: "LINES_READ", Revision: 1, IssueDate: "2025-08-01", DocumentType: "INVOICE", Lines: []LineContext{{ID: "line", Description: "not a VAT description", VATRate: money.MustParse("21.00"), VATValue: money.MustParse("21")}}, Rules: candidates}
}
func evaluatedVAT(t *testing.T, input InvoiceContext) Proposal {
	t.Helper()
	result, err := (ProductionPolicy{}).Evaluate(input)
	if err != nil || len(result.Proposals) != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err = validateResult(input, result, ProductionPolicyVersion); err != nil {
		t.Fatal(err)
	}
	return result.Proposals[1]
}
func TestProductionVerifiedRuleExactProvenanceAndDate(t *testing.T) {
	c := productionVATFixture()
	p := evaluatedVAT(t, productionInput(c))
	if p.RequiresReview || p.Source != SourceRule || p.Rule == nil || p.Rule.RuleVersionID != c.RuleVersionID || p.InvoiceDateUsed != "2025-08-01" || !reflect.DeepEqual(p.Rule.Provenance, c.Provenance) || p.LegalBasis != c.LegalBasis || p.Rule.RulePackVersion != c.RulePackVersion {
		t.Fatalf("proposal=%+v", p)
	}
}
func TestProductionEffectiveDateBoundariesFutureAndExpired(t *testing.T) {
	c := productionVATFixture()
	end := accountingdate.Date("2025-08-02")
	c.EffectiveTo = &end
	for _, tc := range []struct {
		date   accountingdate.Date
		review bool
	}{{"2025-07-31", true}, {"2025-08-01", false}, {"2025-08-02", false}, {"2025-08-03", true}} {
		t.Run(string(tc.date), func(t *testing.T) {
			input := productionInput(c)
			input.IssueDate = tc.date
			p := evaluatedVAT(t, input)
			if p.RequiresReview != tc.review {
				t.Fatalf("%s=%+v", tc.date, p)
			}
		})
	}
}
func TestProductionHistoricalVersionsLawChangeBoundary(t *testing.T) {
	v2 := productionVATFixture()
	v2.Version = 2
	v2.RuleVersionID = "rate-v2"
	v1 := productionVATFixture()
	v1.Provenance = &rules.Provenance{}
	*v1.Provenance = *v2.Provenance
	v1.EffectiveFrom = "2025-01-01"
	end := accountingdate.Date("2025-07-31")
	v1.EffectiveTo = &end
	v1.Provenance.EffectiveFrom = v1.EffectiveFrom
	v1.Result = "19"
	v1.MatchValue = stringPointer("19")
	for _, tc := range []struct {
		date          accountingdate.Date
		rate, version string
	}{{"2025-07-31", "19", "rate-v1"}, {"2025-08-01", "21", "rate-v2"}, {"2025-08-02", "21", "rate-v2"}} {
		input := productionInput(v2, v1)
		input.IssueDate = tc.date
		input.Lines[0].VATRate = money.Amount(tc.rate)
		p := evaluatedVAT(t, input)
		if p.RequiresReview || p.Rule.RuleVersionID != tc.version {
			t.Fatalf("date=%s p=%+v", tc.date, p)
		}
	}
}
func TestProductionDemoIsolationAndInvalidProvenance(t *testing.T) {
	changes := map[string]func(*RuleCandidate){"unverified": func(c *RuleCandidate) { c.ProductionEligible = false }, "placeholder": func(c *RuleCandidate) { c.LegalBasis = rules.LegalBasisPlaceholder }, "no source": func(c *RuleCandidate) { c.Provenance = nil }, "fake URL": func(c *RuleCandidate) { c.Provenance.SourceURL = "https://example.com/law" }, "no verified actor": func(c *RuleCandidate) { c.Provenance.VerifiedBy = "" }, "no pack": func(c *RuleCandidate) { c.RulePackVersion = "" }, "no version": func(c *RuleCandidate) { c.RuleVersionID = "" }, "description VAT": func(c *RuleCandidate) {
		c.MatchKind = rules.MatchDescriptionContains
		c.MatchValue = stringPointer("VAT")
	}, "ALWAYS VAT": func(c *RuleCandidate) { c.MatchKind = rules.MatchAlways; c.MatchValue = nil }}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			c := productionVATFixture()
			change(&c)
			if p := evaluatedVAT(t, productionInput(c)); !p.RequiresReview || p.Rule != nil {
				t.Fatalf("unsafe p=%+v", p)
			}
		})
	}
	input := productionInput(baselineRules()...)
	input.Lines[0].Description = "Serviciu"
	result, _ := (ProductionPolicy{}).Evaluate(input)
	for _, p := range result.Proposals {
		if !p.RequiresReview {
			t.Fatalf("demo resolved %+v", p)
		}
	}
}
func TestProductionNoMatchAmbiguousAndOverlap(t *testing.T) {
	if p := evaluatedVAT(t, productionInput()); !p.RequiresReview || p.Source != SourceNoMatch {
		t.Fatal(p)
	}
	first := productionVATFixture()
	second := productionVATFixture()
	second.RuleID = "second"
	second.RuleVersionID = "second-v1"
	if p := evaluatedVAT(t, productionInput(first, second)); !p.RequiresReview || p.Source != SourceAmbiguous {
		t.Fatal(p)
	}
	second.RuleID = first.RuleID
	second.Version = 2
	second.Result = "11"
	second.MatchValue = stringPointer("11")
	// Different trigger does not make overlapping logical versions unambiguous.
	if p := evaluatedVAT(t, productionInput(first, second)); !p.RequiresReview || p.Source != SourceAmbiguous {
		t.Fatal(p)
	}
}
func TestProductionClientOverrideTemporalPrecedence(t *testing.T) {
	global := productionVATFixture()
	override := productionVATFixture()
	override.RuleID = "override"
	override.RuleVersionID = "override-v1"
	override.Scope = rules.ScopeClientOverride
	override.ParentRuleID = &global.RuleID
	override.EffectiveFrom = "2025-08-02"
	end := accountingdate.Date("2025-08-02")
	override.EffectiveTo = &end
	for _, date := range []accountingdate.Date{"2025-08-01", "2025-08-02", "2025-08-03"} {
		input := productionInput(global, override)
		input.IssueDate = date
		p := evaluatedVAT(t, input)
		expected := global.RuleVersionID
		if date == "2025-08-02" {
			expected = override.RuleVersionID
		}
		if p.RequiresReview || p.Rule.RuleVersionID != expected {
			t.Fatalf("date=%s p=%+v", date, p)
		}
	}
}
func TestProductionVATStructuredExactnessConflictAndZeroReview(t *testing.T) {
	for _, rate := range []string{"21", "21.0", "21.00", "21.0000"} {
		input := productionInput(productionVATFixture())
		input.Lines[0].VATRate = money.MustParse(rate)
		input.Lines[0].Description = "scutit / vehicul / orice descriere"
		if p := evaluatedVAT(t, input); p.RequiresReview {
			t.Fatal(p)
		}
	}
	for _, rate := range []string{"19", "0", "-21", ""} {
		input := productionInput(productionVATFixture())
		input.Lines[0].VATRate = money.Amount(rate)
		if p := evaluatedVAT(t, input); !p.RequiresReview {
			t.Fatal(p)
		}
	}
	c := productionVATFixture()
	c.Result = "19"
	if p := evaluatedVAT(t, productionInput(c)); !p.RequiresReview {
		t.Fatal(p)
	}
	c = productionVATFixture()
	c.Result = "0"
	c.MatchValue = stringPointer("0")
	input := productionInput(c)
	input.Lines[0].VATRate = "0"
	if p := evaluatedVAT(t, input); !p.RequiresReview {
		t.Fatal(p)
	}
}
func TestProductionAccountRequiresExplicitClientPolicyAndVocabulary(t *testing.T) {
	c := productionVATFixture()
	c.Category = rules.CategoryAccount
	c.Result = "628.01"
	c.MatchKind = rules.MatchDescriptionContains
	c.MatchValue = stringPointer("serviciu asumat explicit")
	c.Scope = rules.ScopeClientOverride
	c.Provenance.SourceType = "CLIENT_ACCOUNTING_POLICY"
	c.Provenance.AccountingRegime = "OMFP_1802_2014"
	c.Provenance.ClientPolicyReference = "Fixture client-adopted policy 1"
	c.Provenance.SourceURL = "https://static.anaf.ro/static/10/Anaf/legislatie/OMFP_1802_2014.pdf"
	input := productionInput(c)
	input.Lines[0].Description = *c.MatchValue
	result, _ := (ProductionPolicy{}).Evaluate(input)
	if result.Proposals[0].RequiresReview {
		t.Fatalf("explicit fixture=%+v", result.Proposals[0])
	}
	for _, change := range []func(*RuleCandidate){func(c *RuleCandidate) { c.Scope = rules.ScopeGlobal }, func(c *RuleCandidate) { c.MatchKind = rules.MatchAlways }, func(c *RuleCandidate) { c.Provenance.ClientPolicyReference = "" }, func(c *RuleCandidate) { c.Provenance.AccountingRegime = "" }, func(c *RuleCandidate) { c.Result = "999" }} {
		copy := c
		provenance := *c.Provenance
		copy.Provenance = &provenance
		change(&copy)
		input.Rules = []RuleCandidate{copy}
		result, _ = (ProductionPolicy{}).Evaluate(input)
		if !result.Proposals[0].RequiresReview {
			t.Fatalf("unsafe=%+v", result.Proposals[0])
		}
	}
}
func TestProductionDeductibilityInsufficientContextAndCreditNote(t *testing.T) {
	c := productionVATFixture()
	c.Category = rules.CategoryDeductibility
	c.Result = "N50"
	c.MatchKind = rules.MatchDescriptionContains
	c.MatchValue = stringPointer("car")
	input := productionInput(c)
	input.Lines[0].Description = "car"
	result, _ := (ProductionPolicy{}).Evaluate(input)
	if !result.Proposals[2].RequiresReview || !strings.Contains(result.Proposals[2].LegalBasis, "PRODUCT DECISION REQUIRED") {
		t.Fatal(result)
	}
	input = productionInput(productionVATFixture())
	input.DocumentType = "CREDIT_NOTE"
	if p := evaluatedVAT(t, input); !p.RequiresReview {
		t.Fatal(p)
	}
}
func TestProductionDefaultServiceIsSafeAndIdempotent(t *testing.T) {
	input := productionInput(baselineRules()...)
	store := &captureStore{input: input}
	service := NewService(store, nil, nil)
	command := ProcessCommand{InvoiceID: input.ID, ExpectedRevision: 1, CommandID: "production"}
	result, changed, err := service.ProcessInvoice(context.Background(), command)
	if err != nil || !changed || result.PolicyVersion != ProductionPolicyVersion {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, p := range result.Proposals {
		if !p.RequiresReview {
			t.Fatal(p)
		}
	}
	store.committed = true
	if _, changed, err = service.ProcessInvoice(context.Background(), command); err != nil || changed {
		t.Fatalf("replay changed=%v err=%v", changed, err)
	}
}
func TestProductionEvaluationSnapshotConverges(t *testing.T) {
	input := productionInput(productionVATFixture())
	before, _ := (ProductionPolicy{}).Evaluate(input)
	newer := productionVATFixture()
	newer.Version = 2
	newer.RuleVersionID = "rate-v2"
	newer.EffectiveFrom = "2026-01-01"
	next := productionInput(append(input.Rules, newer)...)
	after, _ := (ProductionPolicy{}).Evaluate(next)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("unrelated future update changed historical output")
	}
	if before.Proposals[1].Rule.RuleVersionID != "rate-v1" {
		t.Fatal(before)
	}
}
