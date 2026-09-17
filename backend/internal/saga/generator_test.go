package saga

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

func TestGenerateDocumentedSAGAInvoiceXML(t *testing.T) {
	item := validInvoice()
	artifact, err := Generate(item, ClientIdentity{ID: item.ClientID, Name: "Cumpărător Știință SRL", CUI: "RO9876543"})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Filename != "F_RO1234567_FAC_42_14.09.2026.xml" || artifact.ExporterVersion != ExporterVersion || len(artifact.SHA256) != 64 {
		t.Fatalf("artifact=%+v", artifact)
	}
	xmlText := string(artifact.Payload)
	for _, expected := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`, `<Facturi>`, `<FurnizorNume>Furnizor Încercare SRL</FurnizorNume>`,
		`<ClientCIF>RO9876543</ClientCIF>`, `<FacturaData>14.09.2026</FacturaData>`, `<FacturaScadenta>30.09.2026</FacturaScadenta>`,
		`<Descriere>Servicii analiză &amp; consultanță</Descriere>`, `<Cantitate>1.2500</Cantitate>`, `<Pret>80.0000</Pret>`,
		`<Valoare>100.0000</Valoare>`, `<ProcTVA>19.0000</ProcTVA>`, `<TVA>19.0000</TVA>`, `<Cont>628.01</Cont>`,
	} {
		if !strings.Contains(xmlText, expected) {
			t.Errorf("missing %q in:\n%s", expected, xmlText)
		}
	}
	if strings.Contains(xmlText, "<FacturaMoneda>") || strings.Contains(xmlText, "<TipDeducere>") {
		t.Fatalf("RON/default optional tags must be omitted:\n%s", xmlText)
	}
	golden, err := os.ReadFile("testdata/invoice_utf8.golden.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(artifact.Payload, golden) {
		t.Fatalf("generated artifact differs from documented-contract golden fixture:\n%s", artifact.Payload)
	}
}

func TestGenerateRejectsUnsafeOrUnverifiedSemantics(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*invoicing.Invoice)
		category FailureCategory
	}{
		{"credit note", func(i *invoicing.Invoice) { i.DocumentType = invoicing.DocumentTypeCreditNote }, FailureUnsupportedDocumentType},
		{"pending classification", func(i *invoicing.Invoice) { i.Lines[0].Classifications[0].Status = classification.ReviewPending }, FailureDataInvalid},
		{"invented account", func(i *invoicing.Invoice) {
			value := "Cont demonstrativ"
			i.Lines[0].Classifications[0].EffectiveValue = &value
		}, FailureDataInvalid},
		{"vat disagreement", func(i *invoicing.Invoice) { value := "9"; i.Lines[0].Classifications[1].EffectiveValue = &value }, FailureDataInvalid},
		{"unknown deductibility", func(i *invoicing.Invoice) {
			value := "Deductibil"
			i.Lines[0].Classifications[2].EffectiveValue = &value
		}, FailureDataInvalid},
		{"line arithmetic", func(i *invoicing.Invoice) { i.Lines[0].TotalValue = money.MustParse("120") }, FailureDataInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := validInvoice()
			tc.mutate(item)
			_, err := Generate(item, ClientIdentity{ID: item.ClientID, Name: "Client", CUI: "RO9876543"})
			if err == nil || Category(err) != tc.category {
				t.Fatalf("category=%s err=%v", Category(err), err)
			}
		})
	}
}

func TestGenerateSupportsDocumentedNonRONAndDeductibilityCodes(t *testing.T) {
	item := validInvoice()
	item.Total.Currency = "EUR"
	value := "N50"
	item.Lines[0].Classifications[2].EffectiveValue = &value
	artifact, err := Generate(item, ClientIdentity{ID: item.ClientID, Name: "Client", CUI: "RO9876543"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(artifact.Payload), "<FacturaMoneda>EUR</FacturaMoneda>") || !strings.Contains(string(artifact.Payload), "<TipDeducere>N50</TipDeducere>") {
		t.Fatalf("payload=%s", artifact.Payload)
	}
}

func validInvoice() *invoicing.Invoice {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	supplierCUI := "RO1234567"
	lineID := "line-1"
	decision := func(id string, dimension classification.Dimension, value string) classification.Decision {
		return classification.Decision{HumanReviewed: true, ID: id, InvoiceID: "invoice-1", InvoiceLineID: lineID, Dimension: dimension, EffectiveValue: &value, Status: classification.ReviewAccepted, PolicyVersion: "POLICY_V1", Revision: 1}
	}
	return &invoicing.Invoice{
		ID: "invoice-1", ClientID: "client-1", SupplierName: "Furnizor Încercare SRL", SupplierCUI: &supplierCUI,
		DocumentNumber: "FAC/42", DocumentType: invoicing.DocumentTypeInvoice, IssueDate: now, DueDate: &due,
		Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, PipelineStatus: invoicing.StatusExporting, Revision: 7,
		Lines: []invoicing.Line{{
			ID: lineID, Position: 1, Description: "Servicii analiză & consultanță", Unit: "H87",
			Quantity: money.MustParse("1.2500"), UnitPrice: money.MustParse("80.0000"), NetValue: money.MustParse("100.0000"),
			VATRate: money.MustParse("19.0000"), VATValue: money.MustParse("19.0000"), TotalValue: money.MustParse("119.0000"),
			Classifications: []classification.Decision{
				decision("account", classification.DimensionAccount, "628.01"),
				decision("vat", classification.DimensionVAT, "19"),
				decision("deductibility", classification.DimensionDeductibility, "SAGA_DEFAULT"),
			},
		}},
	}
}
