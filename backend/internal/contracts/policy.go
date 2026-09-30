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

func dateWithin(value, from time.Time, to *time.Time) bool {
	day := dateOnly(value)
	return !day.Before(dateOnly(from)) && (to == nil || !day.After(dateOnly(*to)))
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.UTC().Year(), value.UTC().Month(), value.UTC().Day(), 0, 0, 0, 0, time.UTC)
}

const OutgoingContextPolicyVersion = "OUTGOING_CONTEXT_V1"

// OutgoingContextPolicy links an issued invoice to a contract where the client
// is the supplier (D-126). The contract is context only: it is optional, it is
// never price-checked, and currency is not compared (sale contracts are priced
// in EUR and invoiced in RON). Contracts outside their effective period are not
// candidates. Several candidates leave the invoice unlinked instead of asking
// the accountant, because the link has no accounting effect in V1.
type OutgoingContextPolicy struct{}

func (OutgoingContextPolicy) Version() string { return OutgoingContextPolicyVersion }

func (policy OutgoingContextPolicy) Evaluate(invoice InvoiceContext, discovered []Contract) (MatchDecision, error) {
	candidates := make([]Contract, 0, len(discovered))
	for _, contract := range discovered {
		if dateWithin(invoice.IssueDay, contract.EffectiveFrom, contract.EffectiveTo) {
			candidates = append(candidates, contract)
		}
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].Reference < candidates[right].Reference })
	result := MatchDecision{Outcome: OutcomeNoMatch, PolicyVersion: policy.Version(), ContractOptional: true}
	for index, contract := range candidates {
		reasons := []string{"CUI-ul clientului facturii coincide cu cumpărătorul contractului în care clientul contabil este furnizor.", "Data facturii este în perioada efectivă a contractului."}
		if invoice.Currency != contract.Value.Currency {
			reasons = append(reasons, "Contractul este în "+contract.Value.Currency+", factura în "+invoice.Currency+"; prețul nu este verificat pentru facturile emise.")
		}
		result.Candidates = append(result.Candidates, Candidate{
			ContractID: contract.ID, ContractRevision: contract.Revision, Rank: index + 1, Recommended: index == 0,
			Compatibility: Compatible, Confidence: "Context pentru factura emisă — fără validare de preț", Reasons: reasons,
		})
	}
	switch len(result.Candidates) {
	case 0:
	case 1:
		result.Outcome = OutcomeUniqueCompatible
	default:
		result.Outcome = OutcomeMultiplePlausible
	}
	return result, nil
}
