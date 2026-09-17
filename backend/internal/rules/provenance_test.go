package rules

import "testing"

func TestProductionPackExplicitlyReviewOnly(t *testing.T) {
	if ProductionRulePackVersion != "RO_INCOMING_ACCOUNTING_V1_REVIEW_ONLY" || len(ProductionPack()) != 0 {
		t.Fatal("Unreviewed mappings must not silently enter the production pack")
	}
}
func TestProductionAccountVocabularyAndSubaccounts(t *testing.T) {
	for _, code := range []string{"628", "628.01", "628.01.ABC"} {
		if !ValidProductionAccount(code) {
			t.Fatal(code)
		}
	}
	for _, code := range []string{"999", "628.", "628..01", "6280", "628.01-2", "628.demo_value"} {
		if ValidProductionAccount(code) {
			t.Fatal(code)
		}
	}
}
func TestProductionProvenanceStructureRequired(t *testing.T) {
	p := Provenance{SourceType: "ACCOUNTING_REGULATION", SourceTitle: "OMFP 1802/2014", Issuer: "Ministerul Finanțelor", LegalInstrument: "OMFP 1802/2014", Reference: "chapter 14 account 628", SourceURL: "https://static.anaf.ro/static/10/Anaf/legislatie/OMFP_1802_2014.pdf", EffectiveFrom: "2015-01-01", VerifiedAt: "2026-09-15", VerifiedBy: "Fixture", Notes: "Fixture assumptions only"}
	if !p.Valid() {
		t.Fatal(p)
	}
	for _, change := range []func(*Provenance){func(p *Provenance) { p.SourceType = "BLOG" }, func(p *Provenance) { p.SourceURL = "javascript:alert(1)" }, func(p *Provenance) { p.SourceURL = "https://static.anaf.ro.example.com/law" }, func(p *Provenance) { p.SourceURL = "https://user@static.anaf.ro/law" }, func(p *Provenance) { p.Reference = "" }, func(p *Provenance) { p.VerifiedAt = "invalid" }, func(p *Provenance) { p.Notes = LegalBasisPlaceholder }} {
		copy := p
		change(&copy)
		if copy.Valid() {
			t.Fatal(copy)
		}
	}
}
