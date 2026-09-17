package fixtures

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestContractPDFScannedAndTextFixtures(t *testing.T) {
	for _, name := range Names {
		pdf := PDF(name)
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("%%EOF")) {
			t.Fatal("invalid synthetic PDF")
		}
		if name == "scanned" && (bytes.Contains(pdf, []byte(" Tj")) || !bytes.Contains(pdf, []byte("/Subtype /Image"))) {
			t.Fatal("scan fixture contains text layer")
		}
		if _, err := (Extractor{}).Extract(context.Background(), pdf, "application/pdf"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (Extractor{}).Extract(context.Background(), []byte("%PDF- arbitrary"), "application/pdf"); err == nil {
		t.Fatal("fake must not invent data for arbitrary PDFs")
	}
}
func TestContractGoldenStructuredValues(t *testing.T) {
	golden, err := os.ReadFile("testdata/romanian-values.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if json.Unmarshal(golden, &want) != nil {
		t.Fatal("golden JSON")
	}
	p := Proposal("romanian")
	for field, wantValue := range want {
		values := map[string]*string{"supplierName": p.SupplierName.Value, "supplierCui": p.SupplierCUI.Value, "reference": p.Reference.Value, "currency": p.Currency.Value, "effectiveFrom": p.EffectiveFrom.Value, "effectiveTo": p.EffectiveTo.Value, "totalValue": p.TotalValue.Value, "unitType": p.UnitType.Value, "paymentTerms": p.PaymentTerms.Value, "buyerCui": p.BuyerCUI.Value}
		if values[field] == nil || *values[field] != wantValue {
			t.Errorf("golden mismatch field=%s", field)
		}
	}
}
