package spv

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/invoicing"
)

func TestUBLParserMapsSyntheticANAFInvoiceExactly(t *testing.T) {
	xml := `<?xml version="1.0"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"><ID>F 001/2026</ID><IssueDate>2026-09-14</IssueDate><DueDate>2026-10-14</DueDate><DocumentCurrencyCode>RON</DocumentCurrencyCode><AccountingSupplierParty><Party><PartyLegalEntity><RegistrationName>Furnizor Sintetic SRL</RegistrationName></PartyLegalEntity><PartyTaxScheme><CompanyID>RO11111111</CompanyID></PartyTaxScheme></Party></AccountingSupplierParty><AccountingCustomerParty><Party><PartyLegalEntity><RegistrationName>Client Sintetic SRL</RegistrationName></PartyLegalEntity><PartyTaxScheme><CompanyID>RO22222222</CompanyID></PartyTaxScheme></Party></AccountingCustomerParty><LegalMonetaryTotal><LineExtensionAmount>20.0000</LineExtensionAmount><TaxExclusiveAmount>20.0000</TaxExclusiveAmount><TaxInclusiveAmount>23.8000</TaxInclusiveAmount><PayableAmount>3.8000</PayableAmount></LegalMonetaryTotal><TaxTotal><TaxAmount>3.8000</TaxAmount></TaxTotal><InvoiceLine><ID>1</ID><InvoicedQuantity unitCode="H87">2.0000</InvoicedQuantity><LineExtensionAmount>20.0000</LineExtensionAmount><Item><Name>Serviciu test</Name><SellersItemIdentification><ID>SKU-1</ID></SellersItemIdentification><ClassifiedTaxCategory><ID>S</ID><Percent>19.0000</Percent></ClassifiedTaxCategory></Item><Price><PriceAmount>10.0000</PriceAmount></Price></InvoiceLine></Invoice>`
	parsed, err := (UBLParser{}).Parse(testZIP(t, map[string]string{"invoice.xml": xml, "semnatura.xml": "<Signature/>"}))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Invoice.DocumentNumber != "F 001/2026" || parsed.Invoice.Total.Amount.String() != "23.8" || parsed.BuyerCUI != "RO22222222" || len(parsed.Invoice.Lines) != 1 {
		t.Fatalf("unexpected mapping: %+v", parsed)
	}
	line := parsed.Invoice.Lines[0]
	if line.Quantity.String() != "2" || line.UnitPrice.String() != "10" || line.NetValue.String() != "20" || line.VATValue.String() != "3.8" || line.TotalValue.String() != "23.8" || line.Unit != "H87" {
		t.Fatalf("unexpected line: %+v", line)
	}
	if line.AdditionalInfo == nil || !strings.Contains(*line.AdditionalInfo, "SKU-1") {
		t.Fatalf("missing sanitized additional info: %+v", line.AdditionalInfo)
	}
}

func TestUBLParserSupportsCreditNoteButNotCII(t *testing.T) {
	credit := `<CreditNote><ID>CN-1</ID><IssueDate>2026-09-14</IssueDate><DocumentCurrencyCode>RON</DocumentCurrencyCode><AccountingSupplierParty><Party><PartyLegalEntity><RegistrationName>S</RegistrationName><CompanyID>1</CompanyID></PartyLegalEntity></Party></AccountingSupplierParty><AccountingCustomerParty><Party><PartyLegalEntity><CompanyID>2</CompanyID></PartyLegalEntity></Party></AccountingCustomerParty><LegalMonetaryTotal><TaxInclusiveAmount>1</TaxInclusiveAmount></LegalMonetaryTotal><CreditNoteLine><ID>1</ID><CreditedQuantity unitCode="H87">1</CreditedQuantity><LineExtensionAmount>1</LineExtensionAmount><Item><Name>X</Name><ClassifiedTaxCategory><Percent>0</Percent></ClassifiedTaxCategory></Item><Price><PriceAmount>1</PriceAmount></Price></CreditNoteLine></CreditNote>`
	parsed, err := (UBLParser{}).Parse(testZIP(t, map[string]string{"credit.xml": credit}))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Invoice.DocumentType != invoicing.DocumentTypeCreditNote {
		t.Fatalf("document type=%q", parsed.Invoice.DocumentType)
	}
	cii := `<CrossIndustryInvoice/>`
	if _, err := (UBLParser{}).Parse(testZIP(t, map[string]string{"cii.xml": cii})); err == nil {
		t.Fatal("CII must remain explicitly unsupported")
	}
}

func TestUBLParserRejectsUnsafeOrAmbiguousZIP(t *testing.T) {
	tests := []map[string]string{{"../invoice.xml": "<Invoice/>"}, {"one.xml": "<Invoice/>", "two.xml": "<Invoice/>"}, {"readme.txt": "x"}}
	for _, files := range tests {
		if _, err := (UBLParser{}).Parse(testZIP(t, files)); err == nil {
			t.Fatalf("expected rejection for %v", files)
		}
	}
}

func testZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, value := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
