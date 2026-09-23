package classification

import (
	"testing"

	"diana-contabilitate/backend/internal/accounting"
)

func TestNormalizedDescriptionV1IsConservativeAndDeterministic(t *testing.T) {
	if NormalizedDescriptionV1 != "NORMALIZED_DESCRIPTION_V1" {
		t.Fatal(NormalizedDescriptionV1)
	}
	equivalent := []string{"  Servicii IT / abonament 2026-09  ", "SERVICII IT—ABONAMENT 2026 / 09", "Servicii\tIT - abonament 2026_09"}
	want := "servicii it abonament 2026 09"
	for _, value := range equivalent {
		if got := NormalizeDescriptionV1(value); got != want {
			t.Fatalf("%q => %q", value, got)
		}
	}
	if NormalizeDescriptionV1("abonament 2026-09 sku-42") == NormalizeDescriptionV1("abonament 2025-09 sku-42") {
		t.Fatal("year was discarded")
	}
	if NormalizeDescriptionV1("abonament martie 10 buc") == NormalizeDescriptionV1("abonament aprilie 10 buc") {
		t.Fatal("month was discarded")
	}
	if NormalizeDescriptionV1("serviciu 10") == NormalizeDescriptionV1("serviciu 11") {
		t.Fatal("number was discarded")
	}
	if NormalizeDescriptionV1("consultanță fiscală") == NormalizeDescriptionV1("consultanță juridică") {
		t.Fatal("meaningful descriptions matched")
	}
	if NormalizeDescriptionV1("Business Standard - September") == NormalizeDescriptionV1("Business Standard - October") {
		t.Fatal("month name was discarded")
	}
	if NormalizeDescriptionV1("Abonament Smart 15") == NormalizeDescriptionV1("Telefon în rate + Abonament Smart 15") {
		t.Fatal("significant prefix was discarded")
	}
}

func TestLearnedMappingMatchesEachExactIdentityKind(t *testing.T) {
	tests := []struct {
		name       string
		line       LineContext
		kind       ServiceIdentityKind
		value      string
		normalizer string
	}{
		{name: "seller item", line: LineContext{ID: "line", Description: "Description", SourceFacts: &accounting.LineFacts{SellerItemID: "SELLER-42"}}, kind: IdentitySellerItemID, value: "SELLER-42", normalizer: ExactIdentifierV1},
		{name: "standard item", line: LineContext{ID: "line", Description: "Description", SourceFacts: &accounting.LineFacts{StandardItemID: "STD-7"}}, kind: IdentityStandardItemID, value: "STD-7", normalizer: ExactIdentifierV1},
		{name: "normalized description", line: LineContext{ID: "line", Description: "  Serviciu lunar - 2026/09 "}, kind: IdentityNormalizedDescription, value: "serviciu lunar 2026 09", normalizer: NormalizedDescriptionV1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := Result{Proposals: []Proposal{{InvoiceLineID: "line", Dimension: DimensionAccount, ProposedValue: "Necesită decizie", Source: SourceNoMatch, RequiresReview: true}}}
			mapping := MappingCandidate{MappingReference: MappingReference{MappingID: "mapping", Version: 1, Revision: 1, AccountCode: "6281", ServiceIdentityKind: string(tt.kind), ServiceIdentityValue: tt.value, NormalizerVersion: tt.normalizer}, Status: "ACTIVE"}
			proposal := applyLearnedAccountMappings(InvoiceContext{Lines: []LineContext{tt.line}, Mappings: []MappingCandidate{mapping}}, base).Proposals[0]
			if proposal.Source != SourceLearnedMapping || proposal.TypedValue == nil || proposal.TypedValue.Account != "6281" {
				t.Fatal(proposal)
			}
		})
	}
}

func TestIdentityPrecedenceAndExactMatching(t *testing.T) {
	line := LineContext{Description: "Serviciu lunar 2026-09", SourceFacts: &accounting.LineFacts{SellerItemID: "SELLER-42", StandardItemID: "STD-7"}}
	identity, ok := PreferredServiceIdentity(line)
	if !ok || identity.Kind != IdentitySellerItemID || identity.Value != "SELLER-42" || identity.NormalizerVersion != ExactIdentifierV1 {
		t.Fatal(identity, ok)
	}
	ids := serviceIdentities(line)
	if len(ids) != 3 || ids[1].Kind != IdentityStandardItemID || ids[2].Kind != IdentityNormalizedDescription {
		t.Fatal(ids)
	}
	standardOnly := LineContext{Description: "Fallback description", SourceFacts: &accounting.LineFacts{StandardItemID: "STD-7"}}
	identity, ok = PreferredServiceIdentity(standardOnly)
	if !ok || identity.Kind != IdentityStandardItemID || identity.Value != "STD-7" || identity.NormalizerVersion != ExactIdentifierV1 {
		t.Fatal(identity, ok)
	}
}

func TestLearnedMappingProposalConflictAndProvenance(t *testing.T) {
	line := LineContext{ID: "line", Description: "Serviciu lunar 2026-09", SourceFacts: &accounting.LineFacts{SellerItemID: "SELLER-42"}}
	base := Result{Proposals: []Proposal{{InvoiceLineID: "line", Dimension: DimensionAccount, ProposedValue: "Necesită decizie", Source: SourceNoMatch, RequiresReview: true}}}
	withoutMapping := applyLearnedAccountMappings(InvoiceContext{Lines: []LineContext{line}}, base).Proposals[0]
	if withoutMapping.Source != SourceNoMatch || withoutMapping.Mapping != nil {
		t.Fatal(withoutMapping)
	}
	mapping := MappingCandidate{MappingReference: MappingReference{MappingID: "mapping-1", Version: 2, Revision: 3, AccountCode: "6281", ServiceIdentityKind: string(IdentitySellerItemID), ServiceIdentityValue: "SELLER-42", NormalizerVersion: ExactIdentifierV1}, Status: "ACTIVE"}
	result := applyLearnedAccountMappings(InvoiceContext{Lines: []LineContext{line}, Mappings: []MappingCandidate{mapping}}, base)
	proposal := result.Proposals[0]
	if proposal.Source != SourceLearnedMapping || !proposal.RequiresReview || proposal.TypedValue.Account != "6281" || proposal.Mapping.MappingID != "mapping-1" || proposal.Mapping.Version != 2 {
		t.Fatal(proposal)
	}

	ruleValue := accounting.Value{Kind: "ACCOUNT", Account: "6262"}
	base.Proposals[0] = Proposal{InvoiceLineID: "line", Dimension: DimensionAccount, ProposedValue: "6262", TypedValue: &ruleValue, Source: SourceRule, Rule: &RuleReference{RuleVersionID: "rule-v1"}}
	conflict := applyLearnedAccountMappings(InvoiceContext{Lines: []LineContext{line}, Mappings: []MappingCandidate{mapping}}, base).Proposals[0]
	if conflict.Source != SourceAmbiguous || !conflict.RequiresReview || conflict.TypedValue != nil || conflict.Mapping == nil {
		t.Fatal(conflict)
	}
	sameRuleValue := accounting.Value{Kind: "ACCOUNT", Account: "6281"}
	base.Proposals[0] = Proposal{InvoiceLineID: "line", Dimension: DimensionAccount, ProposedValue: "6281", TypedValue: &sameRuleValue, Source: SourceRule, Rule: &RuleReference{RuleVersionID: "rule-v2"}}
	compatible := applyLearnedAccountMappings(InvoiceContext{Lines: []LineContext{line}, Mappings: []MappingCandidate{mapping}}, base).Proposals[0]
	if compatible.Source != SourceLearnedMapping || compatible.Rule == nil || compatible.Mapping == nil || !compatible.RequiresReview {
		t.Fatal(compatible)
	}

	mapping2 := mapping
	mapping2.MappingID = "mapping-2"
	mapping2.AccountCode = "6262"
	mapping2.ServiceIdentityKind = string(IdentityNormalizedDescription)
	mapping2.ServiceIdentityValue = NormalizeDescriptionV1(line.Description)
	mapping2.NormalizerVersion = NormalizedDescriptionV1
	base.Proposals[0] = Proposal{InvoiceLineID: "line", Dimension: DimensionAccount, ProposedValue: "Necesită decizie", Source: SourceNoMatch, RequiresReview: true}
	ambiguous := applyLearnedAccountMappings(InvoiceContext{Lines: []LineContext{line}, Mappings: []MappingCandidate{mapping, mapping2}}, base).Proposals[0]
	if ambiguous.Source != SourceAmbiguous || ambiguous.TypedValue != nil {
		t.Fatal(ambiguous)
	}
}

func TestLearnedMappingDoesNotProposeAnAccountMissingFromCurrentSelectableCatalog(t *testing.T) {
	line := LineContext{ID: "line", Description: "Service X", SourceFacts: &accounting.LineFacts{SellerItemID: "SERVICE-X"}}
	base := Result{Proposals: []Proposal{{InvoiceLineID: "line", Dimension: DimensionAccount, ProposedValue: "Necesită decizie", Source: SourceNoMatch, RequiresReview: true}}}
	mapping := MappingCandidate{MappingReference: MappingReference{MappingID: "historical-mapping", Version: 1, Revision: 1, AccountCode: "6281", ServiceIdentityKind: string(IdentitySellerItemID), ServiceIdentityValue: "SERVICE-X", NormalizerVersion: ExactIdentifierV1}, Status: "ACTIVE"}

	proposal := applyLearnedAccountMappings(InvoiceContext{
		Lines:              []LineContext{line},
		Mappings:           []MappingCandidate{mapping},
		SelectableAccounts: map[string]bool{"6262": true},
	}, base).Proposals[0]

	if proposal.Source != SourceNoMatch || proposal.Mapping != nil || proposal.TypedValue != nil || !proposal.RequiresReview {
		t.Fatalf("unusable historical mapping produced a proposal: %+v", proposal)
	}
}
