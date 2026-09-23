package accountinganalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

type catalog map[string]bool

func (c catalog) Postable(code string) bool { return c[code] }

func amount(value string) money.Amount { return money.MustParse(value) }
func fragment() legislation.Fragment {
	text := "TEST_ONLY fragment supplied as corpus evidence; not a legal assertion."
	sum := sha256.Sum256([]byte(text))
	return legislation.Fragment{ID: "fragment-1", VersionID: "law-v1", CitationKey: "TEST_ONLY art. 1", Text: text, ContentHash: hex.EncodeToString(sum[:]), Ordinal: 1}
}
func profile(accounts ...string) *accounting.Profile {
	return &accounting.Profile{ID: "profile-1", ClientID: "client-a", AccountCodes: accounts}
}
func citation(f legislation.Fragment) []Citation {
	return []Citation{{FragmentID: f.ID, VersionID: f.VersionID, CitationKey: f.CitationKey, ContentHash: f.ContentHash}}
}
func treatment(id, net, vat, kind, account string) LineTreatment {
	return LineTreatment{InvoiceLineID: id, VATKind: kind, VATBase: amount(net), VATAmount: amount(vat), VATTiming: "DEFERRED", VATAccount: account, VATDeductibility: "FULL", ExpenseDeductibility: "FULL", DeductibilityCondition: "TEST_ONLY accountant confirmation required"}
}
func base(input Input, entries []Entry, treatments []LineTreatment, f legislation.Fragment) Proposal {
	return Proposal{SchemaVersion: SchemaVersion, ClientID: input.ClientID, InvoiceID: input.InvoiceID, InvoiceRevision: input.InvoiceRevision, Entries: entries, LineTreatments: treatments, Citations: citation(f), ReasoningSummary: "TEST_ONLY structured proposal", Confidence: ConfidenceHigh, Source: SourceLLMLegislation, RequiresReview: true}
}
func entry(phase Phase, debit, credit, value string, lines ...string) Entry {
	return Entry{Phase: phase, DebitAccount: debit, CreditAccount: credit, Amount: amount(value), Currency: "RON", InvoiceLineIDs: lines, Explanation: "TEST_ONLY"}
}

func TestGoldenOrangeCashAccountingProposal(t *testing.T) {
	f := fragment()
	input := Input{ClientID: "client-a", InvoiceID: "orange", InvoiceRevision: 7, IssueDate: accountingdate.Date("2026-01-10"), Direction: Incoming, Currency: "RON", Total: amount("97.20"), Profile: profile("626", "401", "4428", "5121", "4426"), Lines: []Line{{ID: "telecom", Net: amount("80.33"), VAT: amount("16.87"), Gross: amount("97.20")}}}
	proposal := base(input, []Entry{entry(InvoicePhase, "626", "401", "80.33", "telecom"), entry(InvoicePhase, "4428", "401", "16.87", "telecom"), entry(PaymentPhase, "401", "5121", "97.20", "telecom"), entry(PaymentPhase, "4426", "4428", "16.87", "telecom")}, []LineTreatment{treatment("telecom", "80.33", "16.87", "INPUT_VAT", "4428")}, f)
	if issues := Validate(input, proposal, []legislation.Fragment{f}, catalog{"626": true, "401": true, "4428": true, "5121": true, "4426": true}); len(issues) != 0 {
		t.Fatalf("golden Orange rejected: %#v", issues)
	}
}

func TestGoldenBTLeasingMultipleTreatmentsNoVAT(t *testing.T) {
	f := fragment()
	input := Input{ClientID: "client-a", InvoiceID: "insurance", InvoiceRevision: 2, Direction: Incoming, Currency: "RON", Total: amount("1394.78"), Profile: profile("613", "665", "401"), Lines: []Line{{ID: "rca", Net: amount("445"), VAT: amount("0"), Gross: amount("445")}, {ID: "casco", Net: amount("949.76"), VAT: amount("0"), Gross: amount("949.76")}, {ID: "fx", Net: amount("0.02"), VAT: amount("0"), Gross: amount("0.02")}}}
	proposal := base(input, []Entry{entry(InvoicePhase, "613", "401", "445", "rca"), entry(InvoicePhase, "613", "401", "949.76", "casco"), entry(InvoicePhase, "665", "401", "0.02", "fx")}, []LineTreatment{treatment("rca", "445", "0", "NO_VAT", ""), treatment("casco", "949.76", "0", "NO_VAT", ""), treatment("fx", "0.02", "0", "NO_VAT", "")}, f)
	if issues := Validate(input, proposal, []legislation.Fragment{f}, catalog{"613": true, "665": true, "401": true}); len(issues) != 0 {
		t.Fatalf("golden BT Leasing rejected: %#v", issues)
	}
}

func TestGoldenOutgoingConsultingCashAccounting(t *testing.T) {
	f := fragment()
	input := Input{ClientID: "client-a", InvoiceID: "consulting", InvoiceRevision: 3, Direction: Outgoing, Currency: "RON", Total: amount("11885.83"), Profile: profile("4111", "704", "4428", "5121", "4427"), Lines: []Line{{ID: "consulting-line", Net: amount("9823"), VAT: amount("2062.83"), Gross: amount("11885.83")}}}
	proposal := base(input, []Entry{entry(InvoicePhase, "4111", "704", "9823", "consulting-line"), entry(InvoicePhase, "4111", "4428", "2062.83", "consulting-line"), entry(CollectionPhase, "5121", "4111", "11885.83", "consulting-line"), entry(CollectionPhase, "4428", "4427", "2062.83", "consulting-line")}, []LineTreatment{treatment("consulting-line", "9823", "2062.83", "OUTPUT_VAT", "4428")}, f)
	if issues := Validate(input, proposal, []legislation.Fragment{f}, catalog{"4111": true, "704": true, "4428": true, "5121": true, "4427": true}); len(issues) != 0 {
		t.Fatalf("golden consulting rejected: %#v", issues)
	}
}

func TestProposalGuardrailsFailClosed(t *testing.T) {
	f := fragment()
	input := Input{ClientID: "client-a", InvoiceID: "inv", InvoiceRevision: 1, Direction: Incoming, Currency: "RON", Total: amount("121"), Profile: profile("628", "401", "4426"), Lines: []Line{{ID: "line-a", Net: amount("100"), VAT: amount("21"), Gross: amount("121")}}}
	valid := base(input, []Entry{entry(InvoicePhase, "628", "401", "100", "line-a"), entry(InvoicePhase, "4426", "401", "21", "line-a")}, []LineTreatment{treatment("line-a", "100", "21", "INPUT_VAT", "4426")}, f)
	tests := []struct {
		name   string
		mutate func(*Proposal)
		code   string
	}{
		{"foreign tenant", func(p *Proposal) { p.ClientID = "client-b" }, "TENANT_MISMATCH"},
		{"invented account", func(p *Proposal) { p.Entries[0].DebitAccount = "9999" }, "ACCOUNT"},
		{"wrong amount", func(p *Proposal) { p.Entries[0].Amount = amount("99") }, "INVOICE_RECONCILIATION"},
		{"wrong vat", func(p *Proposal) { p.Entries[1].Amount = amount("20") }, "VAT_RECONCILIATION"},
		{"invented citation", func(p *Proposal) { p.Citations[0].CitationKey = "art. invented" }, "LEGAL_CITATION"},
		{"foreign line", func(p *Proposal) { p.Entries[0].InvoiceLineIDs = []string{"other"} }, "FOREIGN_LINE"},
		{"llm bypass review", func(p *Proposal) { p.RequiresReview = false }, "REVIEW_REQUIRED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proposal := valid
			proposal.Entries = append([]Entry(nil), valid.Entries...)
			proposal.Citations = append([]Citation(nil), valid.Citations...)
			tc.mutate(&proposal)
			issues := Validate(input, proposal, []legislation.Fragment{f}, catalog{"628": true, "401": true, "4426": true})
			for _, issue := range issues {
				if issue.Code == tc.code {
					return
				}
			}
			t.Fatalf("missing issue %s in %#v", tc.code, issues)
		})
	}
}
