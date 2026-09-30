package classification

import (
	"testing"
)

func outgoingInput() InvoiceContext {
	in := domainInput()
	in.Direction = "OUTGOING"
	in.NormalizedSupplierID = "26999270"
	return in
}

// D-129: on an issued invoice VAT_DEDUCTIBILITY and EXPENSE_TAX_TREATMENT are
// final DIRECTION decisions; ACCOUNT and VAT_TREATMENT stay for the AI and the
// accountant, and purchase rules never fire.
func TestIssuedInvoiceDerivesOnlyDeductibilityAndExpenseTax(t *testing.T) {
	in := outgoingInput()
	out, err := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range out.Proposals {
		switch p.Dimension {
		case "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT":
			if p.Source != SourceDirection || p.RequiresReview || p.TypedValue == nil || p.TypedValue.Kind != "NOT_APPLICABLE" || p.Evidence == nil {
				t.Fatalf("%s = %+v", p.Dimension, p)
			}
		default:
			if !p.RequiresReview || p.Source == SourceRule || p.Source == SourceDirection {
				t.Fatalf("%s must wait for a decision: %+v", p.Dimension, p)
			}
		}
	}
	if err = validateResult(in, out, out.PolicyVersion); err != nil {
		t.Fatalf("direction-derived result rejected: %v", err)
	}
}

func TestDirectionDerivedDecisionIsRejectedOnReceivedInvoice(t *testing.T) {
	out, _ := (DomainPolicy{AllowTestOnly: true}).Evaluate(outgoingInput())
	received := domainInput()
	if err := validateResult(received, out, out.PolicyVersion); err == nil {
		t.Fatal("a DIRECTION decision was accepted on a received invoice")
	}
}

func TestApprovedKnowledgeNeverReplacesDirectionDecision(t *testing.T) {
	in := outgoingInput()
	out, _ := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	for _, line := range in.Lines {
		identity, ok := PreferredServiceIdentity(line)
		if !ok {
			t.Skip("fixture line has no service identity")
		}
		in.Knowledge = append(in.Knowledge, KnowledgeCandidate{ID: "k", Version: 1, Status: "ACTIVE", Dimension: "VAT_DEDUCTIBILITY", NormalizedSupplierID: in.NormalizedSupplierID,
			Currency: in.Currency, DocumentType: in.DocumentType, VATRate: line.VATRate.String(), ProfileID: in.Snapshot.Profile.ID, ProfileVersion: in.Snapshot.Profile.Version,
			ServiceIdentityKind: string(identity.Kind), ServiceIdentityValue: identity.Value, NormalizerVersion: identity.NormalizerVersion})
	}
	after := applyApprovedKnowledge(in, out)
	for _, p := range after.Proposals {
		if p.Dimension == "VAT_DEDUCTIBILITY" && p.Source != SourceDirection {
			t.Fatalf("knowledge replaced a direction decision: %+v", p)
		}
	}
}
