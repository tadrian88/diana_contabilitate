package accountinganalysis

import (
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/legislation"
)

// D-128: an issued invoice follows the client's profile, not its own printed
// "TVA la încasare" note; a received invoice keeps the supplier-note rule.
func TestIssuedInvoiceVATTimingFollowsClientProfile(t *testing.T) {
	f := testFragment()
	line := testLine("line-sale", "21")
	input := testInput(Outgoing, line)
	input.Profile.CashAccounting = "NO"
	input.SourceFacts = &accounting.SourceFacts{CashAccounting: "YES"}
	immediate := proposal(line.ID, citedDecision("VAT_TREATMENT", ordinary("21"), f))
	results, _ := ValidateUnified(input, immediate, []legislation.Fragment{f}, testCatalog())
	if len(results) != 1 || !results[0].Valid() {
		t.Fatalf("IMMEDIATE must be valid for a client without cash accounting: %#v", results)
	}
	deferred := ordinary("21")
	deferred.Timing = "DEFERRED"
	results, _ = ValidateUnified(input, proposal(line.ID, citedDecision("VAT_TREATMENT", deferred, f)), []legislation.Fragment{f}, testCatalog())
	if len(results) != 1 || results[0].Valid() {
		t.Fatalf("DEFERRED must be refused when the profile has no cash accounting: %#v", results)
	}

	received := testInput(Incoming, line)
	received.SourceFacts = &accounting.SourceFacts{CashAccounting: "YES"}
	results, _ = ValidateUnified(received, immediate, []legislation.Fragment{f}, testCatalog())
	if len(results) != 1 || results[0].Valid() {
		t.Fatalf("a received invoice with the supplier's cash-accounting note must stay DEFERRED: %#v", results)
	}
}

func TestIssuedInvoiceAccountMustBeCreditedSalesAccount(t *testing.T) {
	f := testFragment()
	line := testLine("line-sale", "21")
	input := testInput(Outgoing, line)
	results, _ := ValidateUnified(input, proposal(line.ID, citedDecision("ACCOUNT", accounting.Value{Kind: "ACCOUNT", Account: "704"}, f)), []legislation.Fragment{f}, testCatalog())
	if len(results) != 1 || !results[0].Valid() {
		t.Fatalf("704 must be valid on an issued invoice: %#v", results)
	}
	results, _ = ValidateUnified(input, proposal(line.ID, citedDecision("ACCOUNT", accounting.Value{Kind: "ACCOUNT", Account: "626"}, f)), []legislation.Fragment{f}, testCatalog())
	if len(results) != 1 || results[0].Valid() {
		t.Fatalf("an expense account must be refused on an issued invoice: %#v", results)
	}
}

func TestProviderNeverReceivesCNP(t *testing.T) {
	facts := &accounting.SourceFacts{BuyerLegalID: "1800101420010", SupplierVATID: "RO51741718"}
	masked := maskedPartyFacts(facts)
	if masked.BuyerLegalID != "180***" || masked.SupplierVATID != "RO51741718" || facts.BuyerLegalID != "1800101420010" {
		t.Fatalf("masked=%+v original=%+v", masked, facts)
	}
}
