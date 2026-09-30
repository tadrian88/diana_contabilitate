package postgres

import (
	"testing"

	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
)

func TestReviewedLiteralRuleCannotInjectExecutableFormula(t *testing.T) {
	rule := commercialvalidation.Rule{Kind: commercialvalidation.RuleFixedPrice, Currency: "RON", DateBasis: commercialvalidation.DateInvoiceIssue, Expression: &commercialvalidation.Expression{Op: "literal", Value: "500"}, Blocking: true}
	if !reviewedRuleAllowed(rule) {
		t.Fatal("a manually reviewed literal price must be confirmable")
	}
	rule.Expression.Op = "multiply"
	if reviewedRuleAllowed(rule) {
		t.Fatal("a reviewer must not inject an executable formula")
	}
	rule.Expression.Op = "literal"
	rule.Expression.Value = "not a number"
	if reviewedRuleAllowed(rule) {
		t.Fatal("a reviewer must supply a numeric literal")
	}
}

func TestReviewedLiteralMustBeSupportedBySourceClause(t *testing.T) {
	payment := commercialvalidation.Rule{Kind: commercialvalidation.RulePaymentDue, DateBasis: commercialvalidation.DateReceipt, Expression: &commercialvalidation.Expression{Op: "literal", Value: "5"}, Blocking: true}
	if !reviewedRuleAllowed(payment) || !reviewedLiteralSupportedBySource(payment, "Plata în 5 zile de la data remiterii facturii") {
		t.Fatal("the stated five-day remittance term must be confirmable")
	}
	payment.DateBasis = commercialvalidation.DateInvoiceIssue
	if reviewedLiteralSupportedBySource(payment, "Plata în 5 zile de la data remiterii facturii") {
		t.Fatal("invoice issue date was not stated in the clause")
	}
	payment.DateBasis = commercialvalidation.DateReceipt
	payment.Expression.Value = "50"
	if reviewedLiteralSupportedBySource(payment, "Plata în 5 zile de la data remiterii facturii") {
		t.Fatal("a different number of days was not stated in the clause")
	}
	vat := commercialvalidation.Rule{Kind: commercialvalidation.RuleVAT, DateBasis: commercialvalidation.DateInvoiceIssue, Expression: &commercialvalidation.Expression{Op: "literal", Value: "21"}, Blocking: true}
	if reviewedLiteralSupportedBySource(vat, "Prețurile sunt fără TVA; se adaugă TVA aferent.") {
		t.Fatal("a VAT rate cannot be inferred from a net-of-VAT clause")
	}
	if !reviewedLiteralSupportedBySource(vat, "Se adaugă TVA de 21%") {
		t.Fatal("an explicit VAT rate should be confirmable")
	}
	vat.Expression = &commercialvalidation.Expression{Op: "variable", Variable: "applicable_vat_rate"}
	vat.RequiredVariables = []string{"applicable_vat_rate"}
	if !reviewedRuleAllowed(vat) || !reviewedLiteralSupportedBySource(vat, "Se adaugă TVA-ul aferent") {
		t.Fatal("applicable VAT treatment should be confirmable without inventing a percentage")
	}
	price := commercialvalidation.Rule{Kind: commercialvalidation.RuleFixedPrice, Currency: "RON", DateBasis: commercialvalidation.DateInvoiceIssue, Expression: &commercialvalidation.Expression{Op: "literal", Value: "500"}, Blocking: true}
	if !reviewedLiteralSupportedBySource(price, "Servicii de contabilitate: 500 RON pe lună") {
		t.Fatal("a stated price should be confirmable")
	}
	price.Expression.Value = "600"
	if reviewedLiteralSupportedBySource(price, "Servicii de contabilitate: 500 RON pe lună") {
		t.Fatal("invoice price cannot replace the stated contract price")
	}
}

func TestApplicableVATCompletionMayAddItsRequiredFiscalRate(t *testing.T) {
	proposed := commercialvalidation.Rule{
		ID:            "vat-applicable",
		Kind:          commercialvalidation.RuleVAT,
		Narrative:     "Prețurile sunt fără TVA; se adaugă TVA aferent.",
		Applicability: commercialvalidation.Applicability{},
		Evidence:      []commercialvalidation.Evidence{{Snippet: "Se adaugă TVA aferent."}},
	}
	reviewed := proposed
	reviewed.DateBasis = commercialvalidation.DateInvoiceIssue
	reviewed.Expression = &commercialvalidation.Expression{Op: "variable", Variable: "applicable_vat_rate"}
	reviewed.RequiredVariables = []string{"applicable_vat_rate"}
	reviewed.Blocking = true
	if !reviewedRuleAllowed(reviewed) || !reviewedRulePreservesProposal(proposed, reviewed) {
		t.Fatal("the controlled applicable VAT completion must preserve the narrative proposal")
	}

	withExtractedInput := proposed
	withExtractedInput.RequiredVariables = []string{"contract_supplied_rate"}
	if reviewedRulePreservesProposal(withExtractedInput, reviewed) {
		t.Fatal("an already extracted required input must not be replaced")
	}
}

func TestContractAmountsWithThousandsSeparatorsAreRecognised(t *testing.T) {
	price := func(value string) commercialvalidation.Rule {
		return commercialvalidation.Rule{Kind: commercialvalidation.RuleFixedPrice, Currency: "RON", DateBasis: commercialvalidation.DateInvoiceIssue, Expression: &commercialvalidation.Expression{Op: "literal", Value: value}, Blocking: true}
	}
	supported := []struct{ value, source string }{
		{"1800.00", "1.800,00 lei"},
		{"1800", "Mentenanță IT — abonament lunar · lună · 1 800,00 lei"},
		{"1800.00", "1,800.00 RON"},
		{"650.00", "650,00 lei"},
		{"2450", "Valoarea contractului: 2.450 lei/lună fără TVA"},
		{"125000.00", "Servicii 125000.00 RON"},
		{"1250000", "1.250.000 lei"},
	}
	for _, tc := range supported {
		if !reviewedLiteralSupportedBySource(price(tc.value), tc.source) {
			t.Errorf("%s should be supported by %q", tc.value, tc.source)
		}
	}
	unsupported := []struct{ value, source string }{
		{"1.8", "1.800,00 lei"},
		{"180000", "1.800,00 lei"},
		{"800", "1.800,00 lei"},
	}
	for _, tc := range unsupported {
		if reviewedLiteralSupportedBySource(price(tc.value), tc.source) {
			t.Errorf("%s must not be supported by %q", tc.value, tc.source)
		}
	}
}

func TestReviewedServicePricesReportEveryServiceThatCannotBeActivated(t *testing.T) {
	page := 1
	field := func(value, snippet string) contractingestion.Field {
		return contractingestion.Field{Value: &value, Status: "PRESENT", Evidence: contractingestion.Evidence{Page: &page, Snippet: snippet}}
	}
	proposed := func(description, price, snippet string) contractingestion.ProposedServiceTerm {
		return contractingestion.ProposedServiceTerm{ServiceDescription: field(description, description), UnitPrice: field(price, snippet), Currency: field("RON", "lei")}
	}
	reviewed := func(description, price, model, unit string) contractingestion.ReviewedServiceTerm {
		return contractingestion.ReviewedServiceTerm{ServiceDescription: description, PricingModel: model, UnitPrice: price, Currency: "RON", Unit: unit, BillingFrequency: "MONTHLY"}
	}
	terms := []contractingestion.ReviewedServiceTerm{
		reviewed("Mentenanță IT — abonament lunar", "1800.00", "FIXED_FEE", "lună"),
		reviewed("Hosting cloud — pachet Business 2 VM", "700.00", "FIXED_FEE", "lună"),
		reviewed("Intervenții suplimentare", "150.00", "UNIT_RATE", "oră"),
	}
	proposals := []contractingestion.ProposedServiceTerm{
		proposed("Mentenanță IT — abonament lunar", "1800.00", "1.800,00 lei"),
		proposed("Hosting cloud — pachet Business 2 VM", "650.00", "650,00 lei"),
		proposed("Intervenții suplimentare", "150.00", "150 de ore"),
	}
	rules, skipped := reviewedServicePriceRules("document", "contract", terms, proposals)
	if len(rules) != 1 || rules[0].Applicability.Aliases[0] != "Mentenanță IT — abonament lunar" || rules[0].Unit != "lună" {
		t.Fatalf("only the maintenance price is source-backed: %+v", rules)
	}
	if len(rules[0].Evidence) != 2 || rules[0].Evidence[0].Snippet != "1.800,00 lei" || rules[0].Evidence[1].Snippet != "Mentenanță IT — abonament lunar" {
		t.Fatalf("the rule must cite its price and its service row: %+v", rules[0].Evidence)
	}
	if len(skipped) != 2 || skipped[0].Position != 2 || skipped[0].Reason != commercialvalidation.SkipPriceChangedFromSource || skipped[1].Position != 3 || skipped[1].Reason != commercialvalidation.SkipPriceNotInSource {
		t.Fatalf("skipped services must be reported with their reason: %+v", skipped)
	}
}

func TestReactivationAcceptsRicherEvidenceButNotAChangedPrice(t *testing.T) {
	base := commercialvalidation.Rule{ID: "service-1", Kind: commercialvalidation.RuleFixedPrice, Currency: "RON", Expression: &commercialvalidation.Expression{Op: "literal", Value: "650.00", Scale: 4}, Applicability: commercialvalidation.Applicability{ServiceID: "service-1", Aliases: []string{"Hosting"}, BillingFrequency: "MONTHLY"}, Evidence: []commercialvalidation.Evidence{{Snippet: "650,00 lei"}}}
	richer := base
	richer.Unit = "lună"
	richer.Evidence = append([]commercialvalidation.Evidence{}, base.Evidence[0], commercialvalidation.Evidence{Snippet: "Hosting"})
	if !sameServicePrice(base, richer) {
		t.Fatal("added evidence and unit do not change the agreed price")
	}
	changed := richer
	changed.Expression = &commercialvalidation.Expression{Op: "literal", Value: "700.00", Scale: 4}
	if sameServicePrice(base, changed) {
		t.Fatal("a different amount is a changed price")
	}
}
