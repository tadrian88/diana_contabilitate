package contracts

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/money"
)

func TestBaselinePolicyProducesFourNonExpiredOutcomes(t *testing.T) {
	policy := BaselinePolicy{}
	invoice := policyInvoice()
	compatible := policyContract("contract-a", "CTR-A", "RON")
	incompatible := policyContract("contract-b", "CTR-B", "EUR")
	tests := []struct {
		name      string
		contracts []Contract
		outcome   MatchOutcome
	}{
		{"unique compatible", []Contract{compatible}, OutcomeUniqueCompatible},
		{"multiple plausible", []Contract{compatible, policyContract("contract-c", "CTR-C", "RON")}, OutcomeMultiplePlausible},
		{"unique incompatible", []Contract{incompatible}, OutcomeUniqueIncompatible},
		{"no match", nil, OutcomeNoMatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := policy.Evaluate(invoice, test.contracts)
			if err != nil || decision.Outcome != test.outcome || decision.PolicyVersion != BaselinePolicyVersion {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
		})
	}
}

func TestBaselinePolicyRanksDeterministicallyWithoutNumericScore(t *testing.T) {
	decision, err := (BaselinePolicy{}).Evaluate(policyInvoice(), []Contract{
		policyContract("contract-z", "CTR-Z", "RON"),
		policyContract("contract-a", "CTR-A", "RON"),
	})
	if err != nil || decision.Candidates[0].ContractID != "contract-a" || !decision.Candidates[0].Recommended || decision.Candidates[1].Recommended {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func policyInvoice() InvoiceContext {
	return InvoiceContext{IssueDay: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Currency: "RON"}
}

func policyContract(id, reference, currency string) Contract {
	return Contract{ID: id, Reference: reference, Revision: 1, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), Value: money.Money{Amount: money.MustParse("100.0000"), Currency: currency}}
}
