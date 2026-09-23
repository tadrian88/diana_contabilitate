package commercialvalidation

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

func TestTierDiscountAndReferenceMismatchRemainExplainable(t *testing.T) {
	page := 20
	rules := []Rule{
		{ID: "reference", Kind: RuleContractReference, Narrative: "Contract nr. 19", Blocking: true, Expression: &Expression{Op: "literal", Value: "19 / 02.09.2025"}, Evidence: []Evidence{{DocumentID: "contract-19", Page: &page, Snippet: "Nr. 19 / 02.09.2025"}}},
		{ID: "product", Kind: RuleFixedPrice, Narrative: "6.000 EUR, discount 75%, tranșă 50%", Currency: "EUR", Blocking: true, Applicability: Applicability{ServiceID: "product", Aliases: []string{"Pachet Amprenta"}}, Expression: &Expression{Op: "percent", Scale: 4, Args: []Expression{{Op: "literal", Value: "1500"}, {Op: "literal", Value: "50"}}}, Evidence: []Evidence{{DocumentID: "contract-19", Page: &page, Snippet: "discount de 75%"}}},
	}
	input := Input{Reference: "20 / 02.09.2025", Snapshot: Snapshot{ID: "snapshot", Version: 1, Coverage: CoverageComplete, Rules: rules}, Invoice: invoicing.Invoice{ID: "COL30", Revision: 1, DocumentType: invoicing.DocumentTypeCreditNote, Total: money.Money{Amount: money.MustParse("-8738.68"), Currency: "RON"}, Lines: []invoicing.Line{{ID: "line-1", Description: "Pachet Amprenta de Sustenabilitate", UnitPrice: money.MustParse("750"), SourceFacts: &accounting.LineFacts{PriceAmount: &accounting.AmountFact{Currency: "EUR"}}}}}}
	run := (Engine{}).Validate(input, time.Now())
	if run.Outcome != Nonconform {
		t.Fatalf("outcome=%s findings=%+v", run.Outcome, run.Findings)
	}
	want := map[string]bool{"CONTRACT_REFERENCE_MISMATCH": false, "PRICE_MATCH": false, "ORIGINAL_INVOICE_UNAVAILABLE": false}
	for _, finding := range run.Findings {
		if _, ok := want[finding.Code]; ok {
			want[finding.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("missing %s: %+v", code, run.Findings)
		}
	}
}

func TestExpressionMissingVariableIsNeverGuessed(t *testing.T) {
	expr := Expression{Op: "multiply", Args: []Expression{{Op: "literal", Value: "50"}, {Op: "variable", Variable: "employee_count"}}}
	if _, missing, err := EvaluateExpression(expr, nil); err != nil || len(missing) != 1 || missing[0] != "employee_count" {
		t.Fatalf("missing=%v err=%v", missing, err)
	}
}

func TestSavedVariableOutsideInvoiceDateIsReportedRatherThanCalledMissing(t *testing.T) {
	rule := Rule{ID: "vat", Kind: RuleVAT, Narrative: "TVA aplicabilă", DateBasis: DateInvoiceIssue, Expression: &Expression{Op: "variable", Variable: "applicable_vat_rate"}, RequiredVariables: []string{"applicable_vat_rate"}, Evidence: []Evidence{{Snippet: "Se adaugă TVA aferent."}}, Blocking: true}
	start := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	input := Input{
		Invoice:              invoicing.Invoice{ID: "invoice", Revision: 1, IssueDay: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)},
		Snapshot:             Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}},
		Variables:            map[string]VariableValue{},
		UnavailableVariables: map[string]VariableValue{"applicable_vat_rate": {Name: "applicable_vat_rate", Value: "21", SourceReference: "Sursă fiscală", PeriodStart: &start}},
	}
	run := (Engine{}).Validate(input, time.Now())
	if len(run.Findings) != 1 || run.Findings[0].Code != "RULE_INPUT_OUTSIDE_VALIDITY" || run.Findings[0].Actual != "21" || run.Findings[0].Expected != "valabilă la 04.09.2026" || run.Findings[0].Calculation != "22.09.2026 – fără sfârșit precizat" {
		t.Fatalf("saved out-of-period value was not explained: %+v", run.Findings)
	}
}

func TestClosedExpressionOperations(t *testing.T) {
	limit := "10"
	cases := []struct {
		name       string
		expression Expression
		variables  map[string]string
		want       string
	}{
		{"add", Expression{Op: "add", Scale: 2, Args: []Expression{{Op: "literal", Value: "10"}, {Op: "literal", Value: "2.5"}}}, nil, "12.50"},
		{"discount", Expression{Op: "percent", Scale: 2, Args: []Expression{{Op: "literal", Value: "6000"}, {Op: "literal", Value: "25"}}}, nil, "1500.00"},
		{"cost-plus", Expression{Op: "multiply", Scale: 2, Args: []Expression{{Op: "variable", Variable: "cost"}, {Op: "literal", Value: "1.1"}}}, map[string]string{"cost": "100"}, "110.00"},
		{"tier", Expression{Op: "tier", Scale: 2, Args: []Expression{{Op: "variable", Variable: "documents"}}, Tiers: []Tier{{UpTo: &limit, Value: "500"}, {Value: "700"}}}, map[string]string{"documents": "11"}, "700.00"},
		{"prorata", Expression{Op: "prorate", Scale: 2, Args: []Expression{{Op: "literal", Value: "1000"}, {Op: "variable", Variable: "fraction"}}}, map[string]string{"fraction": "0.5"}, "500.00"},
		{"minimum", Expression{Op: "min", Scale: 2, Args: []Expression{{Op: "literal", Value: "30"}, {Op: "literal", Value: "20"}}}, nil, "20.00"},
		{"maximum", Expression{Op: "max", Scale: 2, Args: []Expression{{Op: "literal", Value: "30"}, {Op: "literal", Value: "20"}}}, nil, "30.00"},
		{"fx", Expression{Op: "fx", Scale: 2, Args: []Expression{{Op: "literal", Value: "100"}, {Op: "variable", Variable: "official_fx"}}}, map[string]string{"official_fx": "4.97"}, "497.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, missing, err := EvaluateExpression(tc.expression, tc.variables)
			if err != nil || len(missing) > 0 || got != tc.want {
				t.Fatalf("got=%s missing=%v err=%v want=%s", got, missing, err, tc.want)
			}
		})
	}
}

func TestFrameworkWithoutPriceRemainsUnverifiable(t *testing.T) {
	input := Input{Invoice: invoicing.Invoice{ID: "framework", Revision: 1, DocumentType: invoicing.DocumentTypeInvoice, Total: money.Money{Amount: money.MustParse("100"), Currency: "RON"}}, Snapshot: Snapshot{Version: 1, Coverage: CoveragePartial}}
	run := (Engine{}).Validate(input, time.Now())
	if run.Outcome != Unverifiable || len(run.Findings) != 1 || run.Findings[0].Code != "CONTRACT_COVERAGE_INCOMPLETE" {
		t.Fatalf("unexpected run: %+v", run)
	}
}

func TestCreditNoteChecksOriginalSignProportionAndCurrency(t *testing.T) {
	original := invoicing.Invoice{ID: "original", DocumentNumber: "INV-1", Total: money.Money{Amount: money.MustParse("1000"), Currency: "RON"}}
	input := Input{Original: &original, Invoice: invoicing.Invoice{ID: "credit", Revision: 1, DocumentType: invoicing.DocumentTypeCreditNote, SourceFacts: &accounting.SourceFacts{PrecedingInvoice: "INV-1"}, Total: money.Money{Amount: money.MustParse("-250"), Currency: "RON"}}, Snapshot: Snapshot{Version: 1, Coverage: CoverageComplete}}
	run := (Engine{}).Validate(input, time.Now())
	if run.Outcome != Conform {
		t.Fatalf("unexpected outcome: %+v", run)
	}
	want := map[string]bool{"CREDIT_NOTE_SIGN_MATCH": false, "CREDIT_NOTE_PROPORTION_VALID": false}
	for _, finding := range run.Findings {
		if _, ok := want[finding.Code]; ok {
			want[finding.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("missing %s", code)
		}
	}
}

func TestCOL31MatchesContract20PriceButRejectsContract19Reference(t *testing.T) {
	page := 20
	input := Input{Reference: "19 / 02.09.2025", Invoice: invoicing.Invoice{ID: "COL31", Revision: 1, DocumentType: invoicing.DocumentTypeCreditNote, Total: money.Money{Amount: money.MustParse("-26216.04"), Currency: "RON"}, Lines: []invoicing.Line{{ID: "line", Description: "Pachet Amprenta de Sustenabilitate", UnitPrice: money.MustParse("2250"), SourceFacts: &accounting.LineFacts{PriceAmount: &accounting.AmountFact{Currency: "EUR"}}}}}, Snapshot: Snapshot{ID: "snapshot-20", Version: 1, Coverage: CoverageComplete, Rules: []Rule{{ID: "reference", Kind: RuleContractReference, Narrative: "Contract nr. 20", DateBasis: DateInvoiceIssue, Expression: &Expression{Op: "literal", Value: "20 / 02.09.2025"}, Evidence: []Evidence{{DocumentID: "contract-20", Page: &page, Snippet: "Nr. 20"}}, Blocking: true}, {ID: "product", Kind: RuleFixedPrice, Narrative: "discount 25%", DateBasis: DateInvoiceIssue, Currency: "EUR", Applicability: Applicability{ServiceID: "product", Aliases: []string{"Pachet Amprenta"}}, Expression: &Expression{Op: "literal", Value: "2250", Scale: 2}, Evidence: []Evidence{{DocumentID: "contract-20", Page: &page, Snippet: "discount 25%"}}, Blocking: true}}}}
	run := (Engine{}).Validate(input, time.Now())
	if run.Outcome != Nonconform {
		t.Fatalf("unexpected outcome: %+v", run)
	}
	codes := map[string]bool{}
	for _, finding := range run.Findings {
		codes[finding.Code] = true
	}
	if !codes["CONTRACT_REFERENCE_MISMATCH"] || !codes["PRICE_MATCH"] || !codes["ORIGINAL_INVOICE_UNAVAILABLE"] {
		t.Fatalf("unexpected findings: %+v", run.Findings)
	}
}

func TestTextualReferenceRuleIsValid(t *testing.T) {
	rule := Rule{ID: "reference", Kind: RuleContractReference, Narrative: "Contract 19", Expression: &Expression{Op: "literal", Value: "19 / 02.09.2025"}, Evidence: []Evidence{{DocumentID: "document", Snippet: "Nr. 19 / 02.09.2025"}}}
	if err := ValidateRule(rule); err != nil {
		t.Fatalf("textual contract reference rejected: %v", err)
	}
}

func TestCrossCurrencyLineWithoutPriceCurrencyIsNeverGuessed(t *testing.T) {
	rule := Rule{ID: "price", Kind: RuleFixedPrice, Narrative: "750 EUR", Currency: "EUR", Applicability: Applicability{ServiceID: "service", Aliases: []string{"Pachet Amprenta"}}, Expression: &Expression{Op: "literal", Value: "750"}, Evidence: []Evidence{{DocumentID: "document", Snippet: "750 EUR"}}}
	input := Input{Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}}, Invoice: invoicing.Invoice{ID: "invoice", Revision: 1, Total: money.Money{Currency: "RON"}, Lines: []invoicing.Line{{ID: "line", Description: "Pachet Amprenta", UnitPrice: money.MustParse("750")}}}}
	run := (Engine{}).Validate(input, time.Now())
	if run.Outcome != Unverifiable || len(run.Findings) != 1 || run.Findings[0].Code != "LINE_PRICE_CURRENCY_MISSING" {
		t.Fatalf("unexpected result: %+v", run)
	}
}

func TestGenericInvoiceLabelRequiresThenUsesConfirmedAlias(t *testing.T) {
	rule := Rule{ID: "accounting-fee", Kind: RuleFixedPrice, Narrative: "Contabilitate lunară 500 RON", Currency: "RON", Expression: &Expression{Op: "literal", Value: "500"}, Evidence: []Evidence{{DocumentID: "contract-102", Snippet: "500 lei pentru partea de contabilitate"}}, Blocking: true}
	invoice := invoicing.Invoice{ID: "invoice", Revision: 1, Total: money.Money{Currency: "RON"}, Lines: []invoicing.Line{{ID: "line", Position: 1, Description: "PRESTARI SERVICII CF. CTR.", UnitPrice: money.MustParse("500"), SourceFacts: &accounting.LineFacts{PriceAmount: &accounting.AmountFact{Currency: "RON"}}}}}
	withoutAlias := (Engine{}).Validate(Input{Invoice: invoice, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}}}, time.Now())
	if withoutAlias.Outcome != Unverifiable || withoutAlias.Findings[0].Code != "SERVICE_LINE_UNCOVERED" || withoutAlias.Findings[0].Actual != "PRESTARI SERVICII CF. CTR." || len(withoutAlias.Findings[0].ServiceCandidates) != 1 {
		t.Fatalf("unexpected proposal result: %+v", withoutAlias)
	}
	withAlias := (Engine{}).Validate(Input{Invoice: invoice, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}}, Aliases: []Alias{{ServiceID: rule.ID, NormalizedLabel: "PRESTARI SERVICII CF. CTR."}}}, time.Now())
	if withAlias.Outcome != Conform || len(withAlias.Findings) != 1 || withAlias.Findings[0].Code != "PRICE_MATCH" {
		t.Fatalf("unexpected confirmed alias result: %+v", withAlias)
	}
}

func TestUnbilledServicesDoNotBecomeInvoiceFindings(t *testing.T) {
	price := func(id, label, amount string) Rule {
		return Rule{ID: id, Kind: RuleFixedPrice, Narrative: label + " " + amount + " RON", Applicability: Applicability{ServiceID: id, Aliases: []string{label}}, DateBasis: DateInvoiceIssue, Currency: "RON", Expression: &Expression{Op: "literal", Value: amount}, Evidence: []Evidence{{DocumentID: "contract", Snippet: amount + " RON"}}, Blocking: true}
	}
	rules := []Rule{price("accounting", "Contabilitate", "500"), price("payroll", "Salarizare", "50")}
	line := invoicing.Line{ID: "line", Position: 1, Description: "Contabilitate", UnitPrice: money.MustParse("500"), SourceFacts: &accounting.LineFacts{PriceAmount: &accounting.AmountFact{Currency: "RON"}}}
	invoice := invoicing.Invoice{ID: "invoice", Revision: 1, Lines: []invoicing.Line{line}}
	run := (Engine{}).Validate(Input{Invoice: invoice, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: rules}}, time.Now())
	if run.Outcome != Conform || len(run.Findings) != 1 || run.Findings[0].Code != "PRICE_MATCH" {
		t.Fatalf("unbilled service created false findings: %+v", run.Findings)
	}
}

func TestUnitRateNeedsIndependentlySourcedQuantity(t *testing.T) {
	rule := Rule{ID: "payroll", Kind: RuleUnitRate, Narrative: "Salarizare 50 RON / salariat", Applicability: Applicability{ServiceID: "payroll", Aliases: []string{"Salarizare"}}, DateBasis: DateInvoiceIssue, Currency: "RON", Expression: &Expression{Op: "literal", Value: "50"}, Evidence: []Evidence{{DocumentID: "contract", Snippet: "50 lei/salariat"}}, Blocking: true}
	invoice := invoicing.Invoice{ID: "invoice", Revision: 1, Lines: []invoicing.Line{{ID: "line", Description: "Salarizare", UnitPrice: money.MustParse("50"), Quantity: money.MustParse("3"), SourceFacts: &accounting.LineFacts{PriceAmount: &accounting.AmountFact{Currency: "RON"}}}}}
	input := Input{Invoice: invoice, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}}}
	missing := (Engine{}).Validate(input, time.Now())
	if missing.Outcome != Unverifiable || len(missing.Findings) != 2 || missing.Findings[1].Code != "UNIT_QUANTITY_SOURCE_MISSING" {
		t.Fatalf("unit price must not imply confirmed quantity: %+v", missing.Findings)
	}
	input.Variables = map[string]VariableValue{"unit_quantity_payroll": {Name: "unit_quantity_payroll", Value: "3", Source: "MANUAL", SourceReference: "Stat de salarii august 2026"}}
	checked := (Engine{}).Validate(input, time.Now())
	if checked.Outcome != Conform || len(checked.Findings) != 2 || checked.Findings[1].Code != "UNIT_QUANTITY_MATCH" {
		t.Fatalf("sourced quantity should be verifiable: %+v", checked.Findings)
	}
}

func TestReferenceFindingIncludesInvoiceProvenance(t *testing.T) {
	rule := Rule{ID: "reference", Kind: RuleContractReference, Narrative: "Contract 102", Expression: &Expression{Op: "literal", Value: "102/25.06.2025"}, Evidence: []Evidence{{DocumentID: "contract-102", Snippet: "102/25.06.2025"}}}
	run := (Engine{}).Validate(Input{Reference: "102/25.06.2025", ReferenceSource: "Descrierea liniei 1", Invoice: invoicing.Invoice{ID: "invoice", Revision: 1}, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}}}, time.Now())
	if len(run.Findings) != 1 || run.Findings[0].Code != "CONTRACT_REFERENCE_MATCH" || run.Findings[0].ActualSource != "Descrierea liniei 1" {
		t.Fatalf("unexpected finding: %+v", run.Findings)
	}
}

func TestInvoiceOutsideContractValidityIsNonconform(t *testing.T) {
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	run := (Engine{}).Validate(Input{Invoice: invoicing.Invoice{ID: "late", Revision: 1, IssueDay: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Snapshot: Snapshot{Coverage: CoverageComplete, EffectiveFrom: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: &to}}, time.Now())
	if run.Outcome != Nonconform || len(run.Findings) != 1 || run.Findings[0].Code != "CONTRACT_NOT_EFFECTIVE" {
		t.Fatalf("unexpected validity result: %+v", run)
	}
}

func TestPaymentDueUsesReceiptCalendarDate(t *testing.T) {
	received := time.Date(2026, 9, 4, 18, 30, 0, 0, time.FixedZone("EEST", 3*60*60))
	due := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	rule := Rule{ID: "payment", Kind: RulePaymentDue, Narrative: "5 zile de la primirea facturii", DateBasis: DateReceipt, Expression: &Expression{Op: "literal", Value: "5"}, Evidence: []Evidence{{DocumentID: "contract", Snippet: "5 zile de la data primirii facturii"}}}
	run := (Engine{}).Validate(Input{ReceiptDate: &received, Invoice: invoicing.Invoice{ID: "invoice", Revision: 1, DueDate: &due}, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}}}, time.Now())
	if run.Outcome != Conform || len(run.Findings) != 1 || run.Findings[0].Code != "PAYMENT_DUE_MATCH" {
		t.Fatalf("unexpected due-date result: %+v", run)
	}
}

func TestPaymentDueFromRemittanceRequiresItsOwnProvenancedDate(t *testing.T) {
	rule := Rule{ID: "payment", Kind: RulePaymentDue, Narrative: "5 zile de la remiterea facturii", DateBasis: DateReceipt, Expression: &Expression{Op: "literal", Value: "5"}, Evidence: []Evidence{{DocumentID: "contract", Snippet: "5 zile de la data remiterii facturii"}}}
	input := Input{Invoice: invoicing.Invoice{ID: "invoice", Revision: 1, IssueDay: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), DueDate: datePtr(2026, 9, 14)}, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{rule}}, ReceiptDate: datePtr(2026, 9, 9)}
	missing := (Engine{}).Validate(input, time.Now())
	if len(missing.Findings) != 1 || missing.Findings[0].Code != "RULE_DATE_BASIS_MISSING" || missing.Findings[0].MissingInputs[0] != "remittance_date" {
		t.Fatalf("receipt must not substitute remittance: %+v", missing.Findings)
	}
	input.RemittanceDate = datePtr(2026, 9, 9)
	checked := (Engine{}).Validate(input, time.Now())
	if len(checked.Findings) != 1 || checked.Findings[0].Code != "PAYMENT_DUE_MATCH" {
		t.Fatalf("remittance date should verify the due date: %+v", checked.Findings)
	}
}

func datePtr(year int, month time.Month, day int) *time.Time {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &value
}

func TestExpressionRejectsOutOfOrderTiersAndExcessiveDepth(t *testing.T) {
	first, second := "20", "10"
	badTiers := Expression{Op: "tier", Args: []Expression{{Op: "literal", Value: "15"}}, Tiers: []Tier{{UpTo: &first, Value: "500"}, {UpTo: &second, Value: "600"}, {Value: "700"}}}
	if err := ValidateExpression(badTiers); err == nil {
		t.Fatal("out-of-order tiers accepted")
	}
	deep := Expression{Op: "literal", Value: "1"}
	for range 18 {
		deep = Expression{Op: "round", Args: []Expression{deep}}
	}
	if err := ValidateExpression(deep); err == nil {
		t.Fatal("excessively deep expression accepted")
	}
}
