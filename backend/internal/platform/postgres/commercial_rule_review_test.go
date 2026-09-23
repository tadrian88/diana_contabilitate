package postgres

import (
	"testing"

	"diana-contabilitate/backend/internal/commercialvalidation"
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
