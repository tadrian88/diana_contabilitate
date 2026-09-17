package accountingacceptance

import (
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
)

func TestEmptyInputCannotRepresentApprovedProduction(t *testing.T) {
	missing := strings.Join((Input{FormatVersion: FormatVersion}).MissingInput(), "\n")
	for _, required := range []string{"pilot profile", "target-SAGA", "real family"} {
		if !strings.Contains(missing, required) {
			t.Fatal(missing)
		}
	}
}

func TestAccountApprovalDoesNotApproveOtherDimensions(t *testing.T) {
	_, _, p, pack := accountingtest.Fixture("TEST_ONLY-client")
	in := Input{FormatVersion: FormatVersion, Profile: p, Mapping: &pack.Mapping, Families: []Family{{ID: "TEST_ONLY-family", ClientID: p.ClientID, Decisions: map[string]Decision{"ACCOUNT": {Value: &accounting.Value{Kind: "ACCOUNT", Account: "628.TEST"}, PolicyReference: "TEST_ONLY", Basis: "TEST_ONLY"}}}}}
	missing := strings.Join(in.MissingInput(), "\n")
	for _, required := range []string{"non-test", "VAT_TREATMENT", "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT", "explicitAccountantApproval", "approvedPositiveRealInvoice"} {
		if !strings.Contains(missing, required) {
			t.Fatal(missing)
		}
	}
	p.TaxRegime = "UNKNOWN"
	if !strings.Contains(strings.Join(in.MissingInput(), "\n"), "profile.taxRegime") {
		t.Fatal("unknown accepted")
	}
}

func TestGoldenAssertionsRequireLineageAndFourExplicitExpectations(t *testing.T) {
	in := Input{FormatVersion: FormatVersion, Examples: []Example{{ID: "TEST_ONLY-example", Lines: []ExpectedLine{{SourceLineID: "line", Decisions: map[string]*accounting.Value{"ACCOUNT": nil}}}}}}
	missing := strings.Join(in.MissingInput(), "\n")
	for _, required := range []string{"fixture/hash/realSourceLineage", "VAT_TREATMENT expectation", "VAT_DEDUCTIBILITY expectation", "EXPENSE_TAX_TREATMENT expectation"} {
		if !strings.Contains(missing, required) {
			t.Fatal(missing)
		}
	}
}
