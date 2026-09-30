package main

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

func TestDemoSeedingEnvironmentBoundary(t *testing.T) {
	for _, environment := range []string{"production", "PRODUCTION", "staging", "local-real", "", "unknown"} {
		if demoSeedingAllowed(environment) {
			t.Fatal(environment)
		}
	}
	for _, environment := range []string{"development", "test"} {
		if !demoSeedingAllowed(environment) {
			t.Fatal(environment)
		}
	}
}

// The Module 6 and 7 journeys reach SAGA without a human exception: every
// journey invoice line is priced by its contract's seeded snapshot.
func TestJourneyInvoicesPassCommercialValidation(t *testing.T) {
	for _, item := range demoCommercialServices {
		rules := demoCommercialRules(item.contractID, item.descriptions)
		for _, description := range item.descriptions {
			line := invoicing.Line{ID: "line", Position: 1, Description: description, Unit: "buc.", Quantity: money.MustParse("1.0000"), UnitPrice: money.MustParse("100.0000"), NetValue: money.MustParse("100.0000")}
			invoice := invoicing.Invoice{ID: "invoice", Revision: 1, Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, Lines: []invoicing.Line{line}}
			run := (commercialvalidation.Engine{}).Validate(commercialvalidation.Input{Invoice: invoice, Snapshot: commercialvalidation.Snapshot{Coverage: commercialvalidation.CoverageComplete, Rules: rules}}, time.Now())
			if run.Outcome != commercialvalidation.Conform {
				t.Errorf("%s / %q: outcome %s, findings %+v", item.contractID, description, run.Outcome, run.Findings)
			}
		}
	}
}
