package commercialvalidation

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

func TestIssuedInvoiceIsNotCommerciallyValidated(t *testing.T) {
	issued := invoicing.Invoice{ID: "invoice", Revision: 2, Direction: invoicing.DirectionOutgoing, IssueDay: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC),
		Total: money.Money{Amount: money.MustParse("34936.75"), Currency: "RON"}, Lines: []invoicing.Line{{ID: "line-1", Description: "servicii conform contract"}}}
	for name, snapshot := range map[string]Snapshot{
		"without contract": {Version: 1, Coverage: CoveragePartial},
		"with an EUR tariff": {ID: "snapshot", Version: 1, Coverage: CoverageComplete, Rules: []Rule{{ID: "rule", Kind: RuleFixedPrice, Currency: "EUR"}}},
	} {
		run := (Engine{}).Validate(Input{Invoice: issued, Snapshot: snapshot}, time.Now())
		if run.Outcome != Conform || len(run.Findings) != 1 || run.Findings[0].Code != "NOT_APPLICABLE_OUTGOING" {
			t.Fatalf("%s: outcome=%s findings=%+v", name, run.Outcome, run.Findings)
		}
	}
}
