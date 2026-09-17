package classification

import (
	"testing"

	"diana-contabilitate/backend/internal/rules"
)

func TestBaselinePolicyProducesExactlyThreeIndependentDimensionsPerLine(t *testing.T) {
	input := InvoiceContext{Lines: []LineContext{{ID: "line-1", Description: "Serviciu demo"}}, Rules: baselineRules()}
	result, err := (BaselinePolicy{}).Evaluate(input)
	if err != nil || len(result.Proposals) != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for index, dimension := range Dimensions {
		if result.Proposals[index].Dimension != dimension || result.Proposals[index].RequiresReview {
			t.Fatalf("proposal=%+v", result.Proposals[index])
		}
	}
}

func TestBaselinePolicyRequiresReviewForNoMatchAndAmbiguity(t *testing.T) {
	ruleset := baselineRules()
	ruleset = append(ruleset, RuleCandidate{RuleID: "account-2", RuleVersionID: "account-2-v1", Reference: "A2", Version: 1, Category: rules.CategoryAccount, Scope: rules.ScopeGlobal, Result: "Other", MatchKind: rules.MatchDescriptionContains, MatchValue: stringPointer("serviciu"), LegalBasis: rules.LegalBasisPlaceholder})
	result, err := (BaselinePolicy{}).Evaluate(InvoiceContext{Lines: []LineContext{{ID: "line-1", Description: "Serviciu demo"}}, Rules: ruleset})
	if err != nil || !result.Proposals[0].RequiresReview || result.Proposals[0].Source != SourceAmbiguous {
		t.Fatalf("account=%+v err=%v", result.Proposals[0], err)
	}
	result, _ = (BaselinePolicy{}).Evaluate(InvoiceContext{Lines: []LineContext{{ID: "line-2", Description: "Necunoscut"}}, Rules: baselineRules()})
	if !result.Proposals[0].RequiresReview || result.Proposals[0].Source != SourceNoMatch {
		t.Fatalf("account=%+v", result.Proposals[0])
	}
}

func TestDirectClientOverrideReplacesOnlyItsGlobalOrigin(t *testing.T) {
	parent := "vat"
	ruleset := baselineRules()
	ruleset = append(ruleset, RuleCandidate{RuleID: "vat-override", RuleVersionID: "vat-override-v1", Reference: "VAT-O", Version: 1, Category: rules.CategoryVAT, Scope: rules.ScopeClientOverride, ParentRuleID: &parent, Result: "Override", MatchKind: rules.MatchAlways, LegalBasis: rules.LegalBasisPlaceholder})
	result, _ := (BaselinePolicy{}).Evaluate(InvoiceContext{Lines: []LineContext{{ID: "line", Description: "Serviciu"}}, Rules: ruleset})
	if result.Proposals[1].ProposedValue != "Override" || result.Proposals[1].Rule.Origin != rules.ScopeClientOverride {
		t.Fatalf("vat=%+v", result.Proposals[1])
	}
}

func baselineRules() []RuleCandidate {
	value := "serviciu"
	return []RuleCandidate{
		{RuleID: "account", RuleVersionID: "account-v1", Reference: "A", Version: 1, Category: rules.CategoryAccount, Scope: rules.ScopeGlobal, Result: "Account demo", MatchKind: rules.MatchDescriptionContains, MatchValue: &value, LegalBasis: rules.LegalBasisPlaceholder},
		{RuleID: "vat", RuleVersionID: "vat-v1", Reference: "V", Version: 1, Category: rules.CategoryVAT, Scope: rules.ScopeGlobal, Result: "VAT demo", MatchKind: rules.MatchAlways, LegalBasis: rules.LegalBasisPlaceholder},
		{RuleID: "deduct", RuleVersionID: "deduct-v1", Reference: "D", Version: 1, Category: rules.CategoryDeductibility, Scope: rules.ScopeGlobal, Result: "Deduct demo", MatchKind: rules.MatchDescriptionContains, MatchValue: &value, LegalBasis: rules.LegalBasisPlaceholder},
	}
}

func stringPointer(value string) *string { return &value }
