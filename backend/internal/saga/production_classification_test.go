package saga

import (
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/rules"
	"testing"
)

func TestSAGAProductionVerifiedSourceClassificationAccepted(t *testing.T) {
	item := validInvoice()
	decision := &item.Lines[0].Classifications[1]
	decision.HumanReviewed = false
	decision.Source = classification.SourceRule
	decision.PolicyVersion = classification.ProductionPolicyVersion
	decision.InvoiceDateUsed = accountingdate.FromTime(item.IssueDate)
	decision.LegalBasis = "Fixture verified source-rate confirmation only"
	decision.Rule = &classification.RuleReference{RuleID: "vat", RuleVersionID: "vat-v1", Reference: "TEST-VAT", Version: 1, ProductionEligible: true, RulePackVersion: "TEST_ONLY", EffectiveFrom: "2020-01-01", Provenance: &rules.Provenance{SourceType: "LEGISLATION", SourceTitle: "Codul fiscal", Issuer: "Parlamentul României", LegalInstrument: "Legea 227/2015", Reference: "art. 291, source-rate fixture only", SourceURL: "https://legislatie.just.ro/Public/DetaliiDocument/186620", EffectiveFrom: "2020-01-01", VerifiedAt: "2026-09-15", VerifiedBy: "Fixture reviewer", Notes: "Synthetic test; accountant resolves ACCOUNT and DEDUCTIBILITY explicitly."}}
	if _, err := Generate(item, ClientIdentity{ID: item.ClientID, Name: "Fixture", CUI: "RO123"}); err != nil {
		t.Fatal(err)
	}
	decision.Rule.ProductionEligible = false
	if _, err := Generate(item, ClientIdentity{ID: item.ClientID, Name: "Fixture", CUI: "RO123"}); err == nil {
		t.Fatal("unverified automatic result exported")
	}
	decision.HumanReviewed = true
	if _, err := Generate(item, ClientIdentity{ID: item.ClientID, Name: "Fixture", CUI: "RO123"}); err != nil {
		t.Fatal("explicit human confirmation rejected", err)
	}
}
func TestSAGADemoAutomaticAndUnattributedCorrectionsRejected(t *testing.T) {
	for _, status := range []classification.ReviewStatus{classification.ReviewAccepted, classification.ReviewCorrected} {
		item := validInvoice()
		d := &item.Lines[0].Classifications[0]
		d.HumanReviewed = false
		d.Status = status
		d.LegalBasis = rules.LegalBasisPlaceholder
		if _, err := Generate(item, ClientIdentity{ID: item.ClientID, Name: "Fixture", CUI: "RO123"}); err == nil {
			t.Fatal("unattributed demo/correction exported")
		}
	}
}
