package commercialvalidation

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
)

func cleaningTariff() Rule {
	description := "Servicii curățenie birou (2 intervenții/săptămână, consumabile incluse)"
	return Rule{ID: "service-cleaning", Kind: RuleUnitRate, Narrative: description + " · 850.00 RON / lună", Unit: "lună", Applicability: Applicability{ServiceID: "service-cleaning", Aliases: []string{description}, BillingFrequency: "MONTHLY"}, DateBasis: DateInvoiceIssue, Currency: "RON", Expression: &Expression{Op: "literal", Value: "850.00", Scale: 4}, Evidence: []Evidence{{DocumentID: "contract", Snippet: "850,00 lei"}}, Blocking: true}
}

func cleaningClause() Rule {
	return Rule{ID: "extracted-4", Kind: RuleUnitRate, Narrative: "Servicii curățenie birou (2 intervenții/săptămână, consumabile incluse): 850,00 lei / lună", Expression: &Expression{Op: "literal", Value: "850.00", Scale: 2}, Evidence: []Evidence{{DocumentID: "contract", Snippet: "Servicii curățenie birou (2 intervenții/săptămână,\nconsumabile incluse)\tlună\t850,00 lei\t1 lună"}}}
}

func TestPricingClauseRestatingAReviewedTariffIsRecognised(t *testing.T) {
	tariff := cleaningTariff()
	if service, restated := RestatedServiceTariff(cleaningClause(), []Rule{tariff}); !restated || service.ID != tariff.ID {
		t.Fatalf("the table row read as a clause restates the tariff: %+v %v", service, restated)
	}
	cases := []struct {
		name   string
		change func(*Rule)
	}{
		{"another price", func(rule *Rule) { rule.Expression.Value = "900.00" }},
		{"another currency", func(rule *Rule) { rule.Currency = "EUR" }},
		{"another service", func(rule *Rule) {
			rule.Narrative = "Curățenie geamuri: 850,00 lei / lună"
			rule.Evidence = []Evidence{{DocumentID: "contract", Snippet: "Curățenie geamuri 850,00 lei"}}
		}},
		{"a tiered price", func(rule *Rule) {
			rule.Expression = &Expression{Op: "tiered", Tiers: []Tier{{Value: "850.00"}}}
		}},
		{"its own applicability", func(rule *Rule) { rule.Applicability.Aliases = []string{"Curățenie"} }},
		{"a discount", func(rule *Rule) { rule.Kind = RuleDiscount }},
	}
	for _, item := range cases {
		clause := cleaningClause()
		item.change(&clause)
		if _, restated := RestatedServiceTariff(clause, []Rule{tariff}); restated {
			t.Errorf("%s: must not count as a restatement", item.name)
		}
	}
	if _, restated := RestatedServiceTariff(tariff, []Rule{tariff}); restated {
		t.Error("a tariff never restates itself")
	}
	sameAmount := cleaningClause()
	sameAmount.Expression.Value = "850"
	if _, restated := RestatedServiceTariff(sameAmount, []Rule{tariff}); !restated {
		t.Error("850 and 850.00 are the same price")
	}
}

func TestServiceIsOfferedOnceWhenAClauseRestatesItsTariff(t *testing.T) {
	line := invoicing.Line{ID: "line-1", Position: 1, Description: "Servicii curățenie birou", Unit: "MON"}
	candidates := SuggestServices(line, []Rule{cleaningTariff(), cleaningClause()})
	if len(candidates) != 1 || candidates[0].RuleID != "service-cleaning" || !candidates[0].Suggested {
		t.Fatalf("only the reviewed tariff should be offered, preselected: %+v", candidates)
	}
	run := (Engine{}).Validate(Input{Invoice: invoicing.Invoice{ID: "invoice", Revision: 1, Lines: []invoicing.Line{line}}, Snapshot: Snapshot{Coverage: CoverageComplete, Rules: []Rule{cleaningTariff(), cleaningClause()}}}, time.Now())
	if len(run.Findings) != 1 || len(run.Findings[0].ServiceCandidates) != 1 {
		t.Fatalf("the uncovered line lists the service once: %+v", run.Findings)
	}
}
