package contracts

import (
	"sort"
	"time"
)

type MatchingPolicy interface {
	Version() string
	Evaluate(InvoiceContext, []Contract) (MatchDecision, error)
}

// BaselinePolicy is a deterministic Module 4 integration policy, not the final
// accounting or legal matching policy. It deliberately has no score, tolerance,
// semantic matching, value matching, or SKU logic.
type BaselinePolicy struct{}

func (BaselinePolicy) Version() string { return BaselinePolicyVersion }

func (policy BaselinePolicy) Evaluate(invoice InvoiceContext, discovered []Contract) (MatchDecision, error) {
	candidates := append([]Contract(nil), discovered...)
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].Reference < candidates[right].Reference })
	if len(candidates) == 0 {
		return MatchDecision{Outcome: OutcomeNoMatch, PolicyVersion: policy.Version()}, nil
	}
	for _, contract := range candidates {
		if !dateWithin(invoice.IssueDay, contract.EffectiveFrom, contract.EffectiveTo) {
			return MatchDecision{}, ErrExpiredContractSemantics
		}
	}

	result := MatchDecision{PolicyVersion: policy.Version(), Candidates: make([]Candidate, 0, len(candidates))}
	for index, contract := range candidates {
		compatibility := Compatible
		reasons := []string{"CUI-ul furnizorului coincide exact în cadrul aceluiași client.", "Data facturii este în perioada efectivă a contractului."}
		confidence := "Semnale deterministe complete — fără scor final"
		if invoice.Currency != contract.Value.Currency {
			compatibility = Incompatible
			reasons = append(reasons, "Moneda contractului diferă de moneda facturii; este necesară confirmarea umană.")
			confidence = "Semnale deterministe nealiniate — fără scor final"
		} else {
			reasons = append(reasons, "Moneda contractului coincide cu moneda facturii.")
		}
		result.Candidates = append(result.Candidates, Candidate{
			ContractID: contract.ID, ContractRevision: contract.Revision, Rank: index + 1,
			Recommended: index == 0, Compatibility: compatibility, Confidence: confidence, Reasons: reasons,
		})
	}

	switch {
	case len(result.Candidates) > 1:
		result.Outcome = OutcomeMultiplePlausible
	case result.Candidates[0].Compatibility == Compatible:
		result.Outcome = OutcomeUniqueCompatible
	default:
		result.Outcome = OutcomeUniqueIncompatible
	}
	return result, nil
}

func dateWithin(value, from, to time.Time) bool {
	day := dateOnly(value)
	return !day.Before(dateOnly(from)) && !day.After(dateOnly(to))
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.UTC().Year(), value.UTC().Month(), value.UTC().Day(), 0, 0, 0, 0, time.UTC)
}
