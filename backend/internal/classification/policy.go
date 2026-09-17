package classification

import (
	"fmt"
	"strings"

	"diana-contabilitate/backend/internal/rules"
)

type Policy interface {
	Version() string
	Evaluate(InvoiceContext) (Result, error)
}

// BaselinePolicy is deterministic integration behavior. It is demonstrative,
// contains no accounting/tax truth and uses no confidence thresholds.
type BaselinePolicy struct{}

func (BaselinePolicy) Version() string { return BaselinePolicyVersion }

func (policy BaselinePolicy) Evaluate(input InvoiceContext) (Result, error) {
	result := Result{PolicyVersion: policy.Version(), Proposals: make([]Proposal, 0, len(input.Lines)*len(Dimensions))}
	// Baseline is explicit test/demo policy and preserves latest-version fixtures.
	latest := map[string]RuleCandidate{}
	for _, candidate := range input.Rules {
		previous, ok := latest[candidate.RuleID]
		if !ok || candidate.Version > previous.Version {
			latest[candidate.RuleID] = candidate
		}
	}
	selected := make([]RuleCandidate, 0, len(latest))
	for _, candidate := range latest {
		selected = append(selected, candidate)
	}
	applicable := replaceGlobalsWithClientOverrides(selected)
	for _, line := range input.Lines {
		for _, dimension := range Dimensions {
			matches := matchingRules(line, dimension, applicable)
			proposal := Proposal{InvoiceLineID: line.ID, Dimension: dimension, LegalBasis: rules.LegalBasisPlaceholder}
			switch len(matches) {
			case 1:
				matched := matches[0]
				proposal.ProposedValue = matched.Result
				proposal.Confidence = "Potrivire deterministă unică — fără prag numeric"
				proposal.Explanation = fmt.Sprintf("Regula demonstrativă %s, versiunea %d, a produs o singură potrivire.", matched.Reference, matched.Version)
				proposal.LegalBasis = matched.LegalBasis
				proposal.Source = SourceRule
				proposal.Rule = &RuleReference{RuleID: matched.RuleID, RuleVersionID: matched.RuleVersionID, Reference: matched.Reference, Version: matched.Version, Origin: matched.Scope}
			default:
				proposal.RequiresReview = true
				proposal.ProposedValue = "Nedeterminat — necesită decizie"
				proposal.Confidence = "Fără decizie automată — fără prag numeric"
				proposal.Source = SourceNoMatch
				proposal.Explanation = "Nicio regulă demonstrativă deterministă nu s-a potrivit; este necesară decizia contabilului."
				if len(matches) > 1 {
					proposal.Source = SourceAmbiguous
					proposal.Confidence = "Potriviri multiple — fără prag numeric"
					proposal.Explanation = "Mai multe reguli demonstrative de același nivel s-au potrivit; sistemul nu a ales arbitrar."
				}
			}
			result.Proposals = append(result.Proposals, proposal)
		}
	}
	return result, nil
}

func replaceGlobalsWithClientOverrides(candidates []RuleCandidate) []RuleCandidate {
	overridden := make(map[string]bool)
	for _, candidate := range candidates {
		if candidate.Scope == rules.ScopeClientOverride && candidate.ParentRuleID != nil {
			overridden[*candidate.ParentRuleID] = true
		}
	}
	result := make([]RuleCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Scope == rules.ScopeGlobal && overridden[candidate.RuleID] {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func matchingRules(line LineContext, dimension Dimension, candidates []RuleCandidate) []RuleCandidate {
	result := make([]RuleCandidate, 0)
	for _, candidate := range candidates {
		if string(candidate.Category) != string(dimension) || candidate.MatchKind == rules.MatchNoAutomation {
			continue
		}
		matches := candidate.MatchKind == rules.MatchAlways
		if candidate.MatchKind == rules.MatchDescriptionContains && candidate.MatchValue != nil {
			matches = strings.Contains(normalize(line.Description), normalize(*candidate.MatchValue))
		}
		if matches {
			result = append(result, candidate)
		}
	}
	return result
}

func normalize(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), " ")) }
