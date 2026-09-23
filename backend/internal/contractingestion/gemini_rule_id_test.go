package contractingestion

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/commercialvalidation"
)

func TestCompleteProviderRuleMetadataIDsAreStableAndDocumentScoped(t *testing.T) {
	rule := json.RawMessage(`{"kind":"FIXED_PRICE","narrative":"Tarif lunar","evidence":[{"snippet":"500 RON"}],"expression":{"op":"literal","value":"500"},"blocking":true}`)
	proposal := Proposal{CommercialClauses: []ProposedCommercialClause{{Rule: append(json.RawMessage(nil), rule...)}, {Rule: append(json.RawMessage(nil), rule...)}}}
	completeProviderRuleMetadata(&proposal, []byte("pdf-one"))
	first := ruleID(t, proposal.CommercialClauses[0].Rule)
	second := ruleID(t, proposal.CommercialClauses[1].Rule)
	if !strings.HasPrefix(first, "extracted-") || first == second {
		t.Fatalf("IDs must be generated and distinct: %q, %q", first, second)
	}
	var decoded commercialvalidation.Rule
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &decoded); err != nil || commercialvalidation.ValidateRule(decoded) != nil {
		t.Fatalf("generated ID did not complete an otherwise valid rule: %v", err)
	}
	completeProviderRuleMetadata(&proposal, []byte("pdf-one"))
	if ruleID(t, proposal.CommercialClauses[0].Rule) != first {
		t.Fatal("same document changed a generated ID")
	}
	other := Proposal{CommercialClauses: []ProposedCommercialClause{{Rule: append(json.RawMessage(nil), rule...)}}}
	completeProviderRuleMetadata(&other, []byte("pdf-two"))
	if ruleID(t, other.CommercialClauses[0].Rule) == first {
		t.Fatal("different documents shared a generated ID")
	}
}

func TestCompleteProviderRuleMetadataCopiesOnlyPresentClauseNarrativeAndEvidence(t *testing.T) {
	page := 2
	narrative := "Tarif lunar de 500 RON"
	kind := "FIXED_PRICE"
	proposal := Proposal{CommercialClauses: []ProposedCommercialClause{{
		Kind:      Field{Status: "PRESENT", Value: &kind},
		Narrative: Field{Status: "PRESENT", Value: &narrative},
		Evidence:  Evidence{Page: &page, Snippet: "500 RON lunar"},
		Rule:      json.RawMessage(`{"expression":{"op":"literal","value":"500"},"blocking":true}`),
	}}}
	completeProviderRuleMetadata(&proposal, []byte("pdf"))
	var rule commercialvalidation.Rule
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Kind != commercialvalidation.RuleFixedPrice || rule.Narrative != narrative || len(rule.Evidence) != 1 || rule.Evidence[0].Snippet != "500 RON lunar" || rule.Evidence[0].Page == nil || *rule.Evidence[0].Page != page {
		t.Fatalf("missing metadata was not copied from the clause envelope: %+v", rule)
	}
	if err := commercialvalidation.ValidateRule(rule); err != nil {
		t.Fatalf("otherwise valid rule did not pass validation: %v", err)
	}
}

func TestCompleteProviderRuleMetadataPreservesProvidedValuesAndUnknownFields(t *testing.T) {
	proposal := Proposal{CommercialClauses: []ProposedCommercialClause{
		{Rule: json.RawMessage(`{"id":"human-chosen","kind":"FIXED_PRICE","narrative":"Already present","evidence":[{"snippet":"Existing evidence"}]}`)},
		{Rule: json.RawMessage(`{"kind":"FIXED_PRICE","unexpected":"must remain invalid"}`)},
	}}
	original := append([]byte(nil), proposal.CommercialClauses[0].Rule...)
	completeProviderRuleMetadata(&proposal, []byte("pdf"))
	if !bytes.Equal(proposal.CommercialClauses[0].Rule, original) {
		t.Fatal("provider ID or its rule changed")
	}
	if !bytes.Contains(proposal.CommercialClauses[1].Rule, []byte(`"unexpected"`)) {
		t.Fatal("unknown provider property was silently removed")
	}
}

func TestCompleteProviderRuleMetadataDoesNotInventMissingNarrativeOrKind(t *testing.T) {
	unsupported := "SPECIAL_CASE"
	proposal := Proposal{CommercialClauses: []ProposedCommercialClause{{Kind: Field{Status: "PRESENT", Value: &unsupported}, Rule: json.RawMessage(`{"expression":{"op":"literal","value":"500"}}`)}}}
	completeProviderRuleMetadata(&proposal, []byte("pdf"))
	var rule commercialvalidation.Rule
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Narrative != "" || rule.Kind != "" || len(rule.Evidence) != 0 || commercialvalidation.ValidateRule(rule) == nil {
		t.Fatalf("missing commercial semantics were silently accepted: %+v", rule)
	}
}

func TestCompleteProviderRuleMetadataDoesNotOverwriteConflictingKind(t *testing.T) {
	kind := "FIXED_PRICE"
	proposal := Proposal{CommercialClauses: []ProposedCommercialClause{{Kind: Field{Status: "PRESENT", Value: &kind}, Rule: json.RawMessage(`{"kind":"UNKNOWN_KIND"}`)}}}
	completeProviderRuleMetadata(&proposal, []byte("pdf"))
	var rule commercialvalidation.Rule
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Kind != "UNKNOWN_KIND" || commercialvalidation.ValidRuleKind(rule.Kind) {
		t.Fatalf("unsupported explicit rule kind was overwritten: %+v", rule)
	}
}

func ruleID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value.ID
}
