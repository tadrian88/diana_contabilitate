package spv

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"strings"
	"testing"
)

func TestAccountingUBLRatePresenceAndOrigin(t *testing.T) {
	for _, c := range []struct {
		name, rate, vat string
		present         bool
		origin          accounting.Origin
	}{{"positive", "<Percent>21</Percent>", "", true, accounting.Calculated}, {"zero", "<Percent>0</Percent>", "", true, accounting.Calculated}, {"missing", "", "", false, accounting.Unknown}, {"declared", "<Percent>21</Percent>", "<TaxTotal><TaxAmount>21</TaxAmount></TaxTotal>", true, accounting.Declared}} {
		t.Run(c.name, func(t *testing.T) {
			out, err := (UBLParser{}).Parse(testZIP(t, map[string]string{"invoice.xml": accountingtest.XML(c.rate, c.vat)}))
			if err != nil {
				t.Fatal(err)
			}
			facts := out.Invoice.Lines[0].SourceFacts
			if (facts.Rate != nil) != c.present || facts.VATOrigin != c.origin {
				t.Fatal(facts)
			}
			if facts.Code != "S" || facts.SourceID != "SOURCE-1" || out.Invoice.ModelVersion != accounting.ModelVersion || len(out.Invoice.SourceFacts.Subtotals) != 1 {
				t.Fatal("source evidence lost")
			}
		})
	}
}
func TestAccountingUBLExemptionDatesAdjustments(t *testing.T) {
	xml := accountingtest.XML("<Percent>0</Percent>", "")
	xml = strings.Replace(xml, "<IssueDate>2026-09-15</IssueDate>", "<IssueDate>2026-09-15</IssueDate><TaxPointDate>2026-09-01</TaxPointDate><InvoicePeriod><StartDate>2026-08-01</StartDate><EndDate>2026-08-31</EndDate></InvoicePeriod><AllowanceCharge><ChargeIndicator>false</ChargeIndicator><Amount currencyID=\"RON\">5</Amount><TaxCategory><ID>S</ID><Percent>21</Percent></TaxCategory></AllowanceCharge>", 1)
	xml = strings.Replace(xml, "<ClassifiedTaxCategory><ID>S</ID>", "<ClassifiedTaxCategory><ID>AE</ID><TaxExemptionReasonCode>VATEX-EU-AE</TaxExemptionReasonCode><TaxExemptionReason>Taxare inversă</TaxExemptionReason>", 1)
	out, err := (UBLParser{}).Parse(testZIP(t, map[string]string{"invoice.xml": xml}))
	if err != nil {
		t.Fatal(err)
	}
	f := out.Invoice.SourceFacts
	l := out.Invoice.Lines[0].SourceFacts
	if f.TaxPointDate != "2026-09-01" || f.PeriodStart != "2026-08-01" || len(f.Adjustments) != 1 || l.ExemptionCode != "VATEX-EU-AE" || l.ExemptionReason == "" || f.SupplierLegalID == f.SupplierVATID || f.BuyerLegalID == "" {
		t.Fatal(f, l)
	}
}
