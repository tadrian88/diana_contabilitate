package accountinganalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accounts"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

func testFragment() legislation.Fragment {
	text := "TEST_ONLY corpus evidence; not a normative legal assertion."
	sum := sha256.Sum256([]byte(text))
	return legislation.Fragment{ID: "fragment-1", VersionID: "law-v1", CitationKey: "TEST_ONLY art. 1", Text: text, ContentHash: hex.EncodeToString(sum[:]), Ordinal: 1}
}

func testInput(direction Direction, lines ...Line) Input {
	return Input{ClientID: "client-a", InvoiceID: "invoice-a", ClassificationRunID: "run-a", InvoiceRevision: 3, Direction: direction,
		Profile: &accounting.Profile{ID: "profile-a", ClientID: "client-a", AccountCodes: []string{"626", "613", "665", "704", "6281"}}, Lines: lines}
}

func testCatalog() Catalog {
	entries := map[string]accounts.Account{}
	for _, code := range []string{"626", "613", "665", "704", "6281"} {
		entries[code] = accounts.Account{Code: code, Name: "TEST_ONLY " + code, Active: true, Postable: true, ParentCode: map[string]string{"6281": "628"}[code]}
	}
	entries["628"] = accounts.Account{Code: "628", Name: "Synthetic", Active: true, Postable: false, Synthetic: true}
	return Catalog{Entries: entries, Children: map[string][]accounts.Account{"628": {entries["6281"]}}}
}

func citedDecision(dimension string, value accounting.Value, fragment legislation.Fragment) DimensionProposal {
	return DimensionProposal{Dimension: dimension, ProposedValue: value, Explanation: "TEST_ONLY concise explanation", Confidence: ConfidenceHigh,
		Citations: []Citation{{FragmentID: fragment.ID, VersionID: fragment.VersionID, CitationKey: fragment.CitationKey, ContentHash: fragment.ContentHash}}}
}

func proposal(lineID string, decisions ...DimensionProposal) Proposal {
	return Proposal{SchemaVersion: SchemaVersion, Source: SourceAIProposal, Summary: "TEST_ONLY", Lines: []LineProposal{{InvoiceLineID: lineID, Decisions: decisions}}}
}

func ordinary(rate string) accounting.Value {
	r := money.MustParse(rate)
	return accounting.Value{Kind: "ORDINARY", Timing: "IMMEDIATE", SourceCategory: "S", SourceRate: &r}
}

func fullProposal(lineID, account string, fragment legislation.Fragment) Proposal {
	return proposal(lineID,
		citedDecision("ACCOUNT", accounting.Value{Kind: "ACCOUNT", Account: account}, fragment),
		citedDecision("VAT_TREATMENT", ordinary("21"), fragment),
		citedDecision("VAT_DEDUCTIBILITY", accounting.Value{Kind: "FULL"}, fragment),
		citedDecision("EXPENSE_TAX_TREATMENT", accounting.Value{Kind: "FULLY_DEDUCTIBLE"}, fragment),
	)
}

func testLine(id, rate string) Line {
	r := money.MustParse(rate)
	vat := money.MustParse("21")
	if rate == "0" {
		vat = money.MustParse("0")
	}
	return Line{ID: id, VAT: vat, Facts: &accounting.LineFacts{TaxCategory: accounting.TaxCategory{Code: "S", Rate: &r}}}
}

func findDecision(t *testing.T, results []ValidatedDecision, dimension string) ValidatedDecision {
	t.Helper()
	for _, result := range results {
		if result.Dimension == dimension {
			return result
		}
	}
	t.Fatalf("missing %s", dimension)
	return ValidatedDecision{}
}

func TestAIProposalIsTypedReviewableAndNotEffective(t *testing.T) {
	f := testFragment()
	input := testInput(Incoming, testLine("line-a", "21"))
	results, issues := ValidateUnified(input, fullProposal("line-a", "626", f), []legislation.Fragment{f}, testCatalog())
	if len(issues) != 0 || len(results) != 4 {
		t.Fatalf("valid AI proposal rejected: %#v", issues)
	}
	account := findDecision(t, results, "ACCOUNT")
	if !account.Valid() || account.TypedValue == nil || account.TypedValue.Account != "626" {
		t.Fatalf("ACCOUNT was not retained as a valid proposal: %#v", account)
	}
}

func TestInvalidAccountDoesNotDiscardValidVAT(t *testing.T) {
	f := testFragment()
	input := testInput(Incoming, testLine("line-a", "21"))
	results, _ := ValidateUnified(input, fullProposal("line-a", "628", f), []legislation.Fragment{f}, testCatalog())
	account, vat := findDecision(t, results, "ACCOUNT"), findDecision(t, results, "VAT_TREATMENT")
	if account.Valid() || len(account.Issues) == 0 || account.Issues[0].Code != "ACCOUNT_NOT_POSTABLE" || len(account.Issues[0].SuggestedAccounts) != 1 {
		t.Fatalf("expected typed non-postable failure: %#v", account)
	}
	if !vat.Valid() {
		t.Fatalf("valid VAT was discarded: %#v", vat.Issues)
	}
}

func TestInvalidCitationAndEnumAreGranular(t *testing.T) {
	f := testFragment()
	input := testInput(Incoming, testLine("line-a", "21"))
	p := fullProposal("line-a", "626", f)
	p.Lines[0].Decisions[0].Citations[0].CitationKey = "invented"
	p.Lines[0].Decisions[2].ProposedValue.Kind = "MOSTLY_DEDUCTIBLE"
	results, _ := ValidateUnified(input, p, []legislation.Fragment{f}, testCatalog())
	if findDecision(t, results, "ACCOUNT").Issues[0].Code != "LEGAL_CITATION" {
		t.Fatal("invented citation was not rejected")
	}
	if findDecision(t, results, "VAT_DEDUCTIBILITY").Issues[0].Code != "INVALID_TYPED_VALUE" {
		t.Fatal("invalid enum was not isolated")
	}
	if !findDecision(t, results, "VAT_TREATMENT").Valid() {
		t.Fatal("sibling dimension did not survive")
	}
}

func TestForeignLineAndResolvedDimensionCannotBeWritten(t *testing.T) {
	f := testFragment()
	input := testInput(Incoming, testLine("line-a", "21"))
	input.ResolvedDimensions = []ResolvedDimension{{InvoiceLineID: "line-a", Dimension: "ACCOUNT", Value: accounting.Value{Kind: "ACCOUNT", Account: "626"}}}
	p := proposal("other-line", citedDecision("ACCOUNT", accounting.Value{Kind: "ACCOUNT", Account: "626"}, f))
	results, _ := ValidateUnified(input, p, []legislation.Fragment{f}, testCatalog())
	if len(results) != 1 || results[0].Issues[0].Code != "FOREIGN_LINE" {
		t.Fatalf("foreign line accepted: %#v", results)
	}
	p = proposal("line-a", citedDecision("ACCOUNT", accounting.Value{Kind: "ACCOUNT", Account: "6281"}, f))
	results, _ = ValidateUnified(input, p, []legislation.Fragment{f}, testCatalog())
	if findDecision(t, results, "ACCOUNT").Issues[0].Code != "DIMENSION_ALREADY_RESOLVED" {
		t.Fatal("AI overwrote a resolved dimension")
	}
}

func TestGoldenUnifiedContractsDoNotModelFutureEvents(t *testing.T) {
	f := testFragment()
	tests := []struct {
		name      string
		direction Direction
		lines     []Line
		accounts  []string
	}{
		{"Orange telecom", Incoming, []Line{testLine("telecom", "21")}, []string{"626"}},
		{"BT Leasing", Incoming, []Line{testLine("rca", "0"), testLine("casco", "0"), testLine("fx", "0")}, []string{"613", "613", "665"}},
		{"issued consulting", Outgoing, []Line{testLine("consulting", "21")}, []string{"704"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := testInput(tc.direction, tc.lines...)
			for _, line := range tc.lines {
				for _, dimension := range []string{"VAT_TREATMENT", "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT"} {
					input.ResolvedDimensions = append(input.ResolvedDimensions, ResolvedDimension{InvoiceLineID: line.ID, Dimension: dimension, Value: accounting.Value{Kind: "NOT_APPLICABLE", Reason: "TEST_ONLY"}})
				}
			}
			p := Proposal{SchemaVersion: SchemaVersion, Source: SourceAIProposal, Summary: "invoice event only"}
			for index, line := range tc.lines {
				p.Lines = append(p.Lines, LineProposal{InvoiceLineID: line.ID, Decisions: []DimensionProposal{citedDecision("ACCOUNT", accounting.Value{Kind: "ACCOUNT", Account: tc.accounts[index]}, f)}})
			}
			results, issues := ValidateUnified(input, p, []legislation.Fragment{f}, testCatalog())
			if len(issues) != 0 || len(results) != len(tc.lines) {
				t.Fatalf("golden contract rejected: %#v", issues)
			}
		})
	}
}
