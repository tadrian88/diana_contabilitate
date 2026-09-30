package contracts

import (
	"context"
	"testing"
	"time"
)

func TestOutgoingContextPolicyIsOptionalAndIgnoresCurrencyAndExpiredContracts(t *testing.T) {
	policy := OutgoingContextPolicy{}
	invoice := policyInvoice()
	eur := policyContract("contract-eur", "CTR-EUR", "EUR")
	expired := policyContract("contract-old", "CTR-OLD", "RON")
	ended := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	expired.EffectiveTo = &ended
	for _, test := range []struct {
		name      string
		contracts []Contract
		outcome   MatchOutcome
		count     int
	}{
		{"no contract", nil, OutcomeNoMatch, 0},
		{"only an expired contract", []Contract{expired}, OutcomeNoMatch, 0},
		{"one EUR contract", []Contract{eur, expired}, OutcomeUniqueCompatible, 1},
		{"two contracts", []Contract{eur, policyContract("contract-ron", "CTR-RON", "RON")}, OutcomeMultiplePlausible, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision, err := policy.Evaluate(invoice, test.contracts)
			if err != nil || decision.Outcome != test.outcome || len(decision.Candidates) != test.count || !decision.ContractOptional || decision.PolicyVersion != OutgoingContextPolicyVersion {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
			if err = validateMatchDecision(decision, OutgoingContextPolicyVersion); err != nil {
				t.Fatalf("decision fails validation: %v", err)
			}
		})
	}
}

func TestServiceUsesOutgoingPolicyOnlyForIssuedInvoices(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC) }
	issued := &captureStore{input: InvoiceContext{ID: "invoice-issued", Direction: "OUTGOING", PipelineStatus: "MATCHING", Revision: 3, IssueDay: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Currency: "RON"}}
	decision, changed, err := NewService(issued, BaselinePolicy{}, now).MatchInvoice(context.Background(), MatchCommand{InvoiceID: "invoice-issued", ExpectedRevision: 3, CommandID: "match-issued"})
	if err != nil || !changed || decision.Outcome != OutcomeNoMatch || !issued.decision.ContractOptional || issued.decision.PolicyVersion != OutgoingContextPolicyVersion {
		t.Fatalf("issued decision=%+v err=%v", issued.decision, err)
	}
	received := &captureStore{input: InvoiceContext{ID: "invoice-received", PipelineStatus: "MATCHING", Revision: 3, IssueDay: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Currency: "RON"}}
	if _, _, err = NewService(received, BaselinePolicy{}, now).MatchInvoice(context.Background(), MatchCommand{InvoiceID: "invoice-received", ExpectedRevision: 3, CommandID: "match-received"}); err != nil || received.decision.ContractOptional || received.decision.PolicyVersion != BaselinePolicyVersion {
		t.Fatalf("received decision=%+v err=%v", received.decision, err)
	}
}
