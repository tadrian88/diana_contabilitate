package saga

import (
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
)

const issuedCustomerCNP = "1800101420010"

// issuedInvoice turns the packless fixture into an invoice issued by the
// client to a natural person: reviewed ACCOUNT 704 and immediate VAT, and the
// two DIRECTION decisions of D-129.
func issuedInvoice(t *testing.T) *invoicing.Invoice {
	t.Helper()
	item := packlessInvoice(t)
	client := domainClient()
	own, customerName, customerID, kind := client.CUI, "PERSOANĂ FIZICĂ TEST", issuedCustomerCNP, "CNP"
	item.SupplierName, item.SupplierCUI = client.Name, &own
	item.Direction, item.CustomerName, item.CustomerIdentifier, item.NormalizedCustomerID, item.CustomerIdentifierKind = invoicing.DirectionOutgoing, &customerName, &customerID, &customerID, &kind
	facts := *item.SourceFacts
	facts.SupplierVATID, facts.BuyerVATID, facts.BuyerLegalID, facts.CashAccounting = client.CUI, "", issuedCustomerCNP, "YES"
	item.SourceFacts = &facts
	item.AccountingSnapshot.Profile.AccountCodes = nil
	for i := range item.Lines[0].Classifications {
		d := &item.Lines[0].Classifications[i]
		switch d.Dimension {
		case "ACCOUNT":
			d.TypedValue = &accounting.Value{Kind: "ACCOUNT", Account: "704"}
		case "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT":
			value, _ := accounting.DirectionDerivedValue(string(d.Dimension))
			d.TypedValue, d.Source, d.HumanReviewed, d.ReviewReason, d.PolicyVersion = &value, classification.SourceDirection, false, "", classification.DomainPolicyVersion
		}
		text := d.TypedValue.Text()
		d.EffectiveValue = &text
	}
	return item
}

func approvedOutgoingMapping() accounting.MappingPolicy {
	mapping := accounting.DefaultSAGAOutgoingMapping
	mapping.Approved = true
	mapping.Approval = accounting.Approval{Actor: "TEST_ONLY", At: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Evidence: []string{"TEST_ONLY accepted Ieșiri import"}}
	return mapping
}

func TestIssuedInvoiceIsBlockedOnlyByUnverifiedIesiriFormat(t *testing.T) {
	item := issuedInvoice(t)
	if r := EvaluateReadiness(item, domainClient(), true, false); r.Ready || r.Reason != OutgoingFormatUnverified {
		t.Fatalf("readiness = %+v", r)
	}
	if r := evaluateReadiness(item, domainClient(), true, false, approvedOutgoingMapping()); !r.Ready || r.MappingVersion != accounting.DefaultSAGAOutgoingMapping.Version {
		t.Fatalf("with an approved Ieșiri mapping the issued invoice must be ready: %+v", r)
	}
}

func TestIssuedInvoiceDataProblemsSurfaceBeforeTheGate(t *testing.T) {
	item := issuedInvoice(t)
	d := &item.Lines[0].Classifications[0]
	d.TypedValue = &accounting.Value{Kind: "ACCOUNT", Account: "626"}
	text := d.TypedValue.Text()
	d.EffectiveValue = &text
	if r := EvaluateReadiness(item, domainClient(), true, false); r.Ready || r.Reason == OutgoingFormatUnverified || !strings.Contains(r.Reason, "clasa 7") {
		t.Fatalf("expense account on an issued invoice = %+v", r)
	}

	deferred := issuedInvoice(t)
	for i := range deferred.Lines[0].Classifications {
		if v := deferred.Lines[0].Classifications[i]; v.Dimension == "VAT_TREATMENT" {
			value := *v.TypedValue
			value.Timing = "DEFERRED"
			text := value.Text()
			deferred.Lines[0].Classifications[i].TypedValue, deferred.Lines[0].Classifications[i].EffectiveValue = &value, &text
		}
	}
	if r := EvaluateReadiness(deferred, domainClient(), true, false); r.Ready || r.Reason == OutgoingFormatUnverified {
		t.Fatalf("deferred VAT for a client without cash accounting = %+v", r)
	}
}

func TestDirectionDecisionIsNotAcceptedOnReceivedInvoice(t *testing.T) {
	item := packlessInvoice(t)
	value, _ := accounting.DirectionDerivedValue("EXPENSE_TAX_TREATMENT")
	d := &item.Lines[0].Classifications[3]
	d.TypedValue, d.Source, d.HumanReviewed, d.ReviewReason, d.PolicyVersion = &value, classification.SourceDirection, false, "", classification.DomainPolicyVersion
	text := value.Text()
	d.EffectiveValue = &text
	if r := EvaluateReadiness(item, domainClient(), true, false); r.Ready {
		t.Fatal("a DIRECTION decision made a received invoice exportable")
	}
}

func TestIesiriXMLPutsTheClientAsSupplierAndTheCustomerAsClient(t *testing.T) {
	item := issuedInvoice(t)
	if _, err := GenerateTestOnly(item, domainClient()); err == nil || !strings.Contains(err.Error(), OutgoingFormatUnverified) {
		t.Fatalf("production path must refuse the unverified Ieșiri format: %v", err)
	}
	artifact, err := generateWith(item, domainClient(), true, approvedOutgoingMapping())
	if err != nil {
		t.Fatal(err)
	}
	payload := string(artifact.Payload)
	for _, want := range []string{
		"<FurnizorNume>" + domainClient().Name + "</FurnizorNume>",
		"<FurnizorCIF>" + accountingtest.BuyerCUI + "</FurnizorCIF>",
		"<ClientNume>PERSOANĂ FIZICĂ TEST</ClientNume>",
		"<ClientCIF>" + issuedCustomerCNP + "</ClientCIF>",
		"<FacturaTVAIncasare>Nu</FacturaTVAIncasare>",
		"<Cont>704</Cont>",
	} {
		if !strings.Contains(payload, want) {
			t.Fatalf("missing %s in:\n%s", want, payload)
		}
	}
	if strings.Contains(payload, "TipDeducere") || strings.Index(payload, "<FacturaData>") > strings.Index(payload, "<FacturaTVAIncasare>") {
		t.Fatalf("unexpected Ieșiri structure:\n%s", payload)
	}
	if !strings.HasPrefix(artifact.Filename, "F_"+accountingtest.BuyerCUI+"_") || !strings.HasSuffix(artifact.ExporterVersion, "/"+accounting.DefaultSAGAOutgoingMapping.Version) {
		t.Fatalf("filename=%s exporter=%s", artifact.Filename, artifact.ExporterVersion)
	}
}
