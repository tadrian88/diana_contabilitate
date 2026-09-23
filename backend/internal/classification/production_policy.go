package classification

import (
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/rules"
	"fmt"
	"strings"
)

const ProductionPolicyVersion = "REAL_ACCOUNTING_RULES_V1"

type ProductionPolicy struct{ Observer ProductionObserver }
type ProductionObserver interface {
	ClassificationEvaluated(Dimension, int, int, bool)
}

func (ProductionPolicy) Version() string { return ProductionPolicyVersion }

// ProductionPolicy evaluates reviewed local evidence. Supplier rate confirmation
// does not determine the legally correct VAT treatment or right of deduction.
func (policy ProductionPolicy) Evaluate(input InvoiceContext) (Result, error) {
	result := Result{PolicyVersion: policy.Version()}
	eligible := make([]RuleCandidate, 0, len(input.Rules))
	for _, c := range input.Rules {
		if productionCandidate(c, input.SelectableAccounts[c.Result]) && input.IssueDate.Within(c.EffectiveFrom, c.EffectiveTo) && input.IssueDate.Within(c.Provenance.EffectiveFrom, c.Provenance.EffectiveTo) {
			eligible = append(eligible, c)
		}
	}
	applicable := replaceGlobalsWithClientOverrides(eligible)
	for _, line := range input.Lines {
		for _, dimension := range Dimensions {
			p := Proposal{InvoiceLineID: line.ID, Dimension: dimension, InvoiceDateUsed: input.IssueDate, ProposedValue: "Nedeterminat — necesită decizie", RequiresReview: true, Source: SourceNoMatch, Confidence: "Fără decizie automată — fără prag numeric", LegalBasis: "ACCOUNTING RULE SOURCE REQUIRED", Explanation: "Nicio regulă verificată pentru producție nu este aplicabilă datei facturii; este necesară decizia contabilului."}
			matches := productionMatches(line, dimension, applicable)
			switch {
			case !input.IssueDate.Valid():
				p.Explanation = "Data contabilă a facturii lipsește sau este invalidă; este necesară revizuirea."
			case input.DocumentType != "INVOICE":
				p.Explanation = "Tratamentul contabil al documentului nu este acceptat de acest modul; este necesară revizuirea."
			case dimension == DimensionDeductibility:
				p.LegalBasis = "PRODUCT DECISION REQUIRED — DEDUCTIBILITY SEMANTICS"
				p.Explanation = "TipDeducere SAGA nu stabilește dreptul de deducere TVA sau deductibilitatea cheltuielii; lipsesc semantica aprobată și contextul clientului."
			case overlappingLogicalRule(dimension, applicable) || len(matches) > 1:
				p.Source = SourceAmbiguous
				p.Explanation = "Mai multe reguli sau versiuni efective de același nivel sunt aplicabile; sistemul nu a ales arbitrar."
			case len(matches) == 1:
				c := matches[0]
				p.ProposedValue = c.Result
				p.Source = SourceRule
				p.LegalBasis = c.LegalBasis
				p.Confidence = "Potrivire deterministă unică — fără prag numeric"
				p.Rule = &RuleReference{RuleID: c.RuleID, RuleVersionID: c.RuleVersionID, Reference: c.Reference, Version: c.Version, Origin: c.Scope, ProductionEligible: true, RulePackVersion: c.RulePackVersion, Provenance: c.Provenance, EffectiveFrom: c.EffectiveFrom, EffectiveTo: c.EffectiveTo}
				condition := string(c.MatchKind)
				if c.MatchValue != nil {
					condition = fmt.Sprintf("%s (%q)", c.MatchKind, *c.MatchValue)
				}
				p.Explanation = fmt.Sprintf("Linia a fost propusă deoarece condiția %s este îndeplinită. Regula %s, versiunea %d, este aplicabilă datei %s. %s", condition, c.Reference, c.Version, input.IssueDate, c.Explanation)
				p.RequiresReview = false
				if dimension == DimensionVAT {
					rate, err := money.Parse(c.Result)
					if err != nil || !line.VATRate.Valid() || !rate.Equal(line.VATRate) {
						p.RequiresReview = true
						p.Explanation = "Procentul TVA propus nu confirmă numeric procentul declarat în factura sursă; este necesară revizuirea."
					}
				}
			}
			if policy.Observer != nil {
				evaluated := 0
				for _, c := range applicable {
					if string(c.Category) == string(dimension) {
						evaluated++
					}
				}
				policy.Observer.ClassificationEvaluated(dimension, evaluated, len(matches), p.RequiresReview)
			}
			result.Proposals = append(result.Proposals, p)
		}
	}
	return result, nil
}

func productionCandidate(c RuleCandidate, selectableAccount bool) bool {
	if !c.ProductionEligible || !c.Provenance.Valid() || strings.TrimSpace(c.RulePackVersion) == "" || c.RuleID == "" || c.RuleVersionID == "" || c.Version <= 0 || strings.TrimSpace(c.LegalBasis) == "" || strings.Contains(c.LegalBasis, rules.LegalBasisPlaceholder) || strings.TrimSpace(c.Explanation) == "" {
		return false
	}
	if c.Scope != rules.ScopeGlobal && c.Scope != rules.ScopeClientOverride {
		return false
	}
	switch c.Category {
	case rules.CategoryAccount:
		return selectableAccount && c.Scope == rules.ScopeClientOverride && c.Provenance.SourceType == "CLIENT_ACCOUNTING_POLICY" && c.Provenance.AccountingRegime == "OMFP_1802_2014" && strings.TrimSpace(c.Provenance.ClientPolicyReference) != "" && rules.ValidProductionAccount(c.Result) && c.MatchKind == rules.MatchDescriptionContains && c.MatchValue != nil && normalize(*c.MatchValue) != ""
	case rules.CategoryVAT:
		rate, err := money.Parse(c.Result)
		return err == nil && !strings.HasPrefix(rate.String(), "-") && !rate.Equal(money.MustParse("0")) && c.MatchKind == rules.MatchVATSourceRateEquals && c.MatchValue != nil && money.Amount(*c.MatchValue).Valid() && rate.Equal(money.Amount(*c.MatchValue))
	default:
		return false
	}
}
func productionMatches(line LineContext, dimension Dimension, candidates []RuleCandidate) []RuleCandidate {
	matches := matchingRules(line, dimension, candidates)
	for _, c := range candidates {
		if dimension == DimensionVAT && c.Category == rules.CategoryVAT && c.MatchKind == rules.MatchVATSourceRateEquals && c.MatchValue != nil && line.VATRate.Valid() && line.VATValue.Valid() && line.VATRate.Equal(money.Amount(*c.MatchValue)) {
			matches = append(matches, c)
		}
	}
	return matches
}
func overlappingLogicalRule(dimension Dimension, candidates []RuleCandidate) bool {
	counts := map[string]int{}
	for _, c := range candidates {
		if string(c.Category) != string(dimension) {
			continue
		}
		counts[c.RuleID]++
		if counts[c.RuleID] > 1 {
			return true
		}
	}
	return false
}
