package postgres

import (
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/invoicing"
)

func TestInvoiceContractReferenceFromStornoLines(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"Anul 2 - Plata transa 1 cf ctr NR. 20 / 02.09.2025", "20 / 02.09.2025"},
		{"Anul 2 - Plata transa 1 cf ctr NR. 19 / 02.09.2025", "19 / 02.09.2025"},
		{"Servicii conform contract nr. 102/25.06.2025", "102/25.06.2025"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got, _ := invoiceContractReference(invoicing.Invoice{Lines: []invoicing.Line{{Position: 1, Description: tc.text}}})
			if got != tc.want {
				t.Fatalf("reference = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInvoiceContractReferencePreservesUBLSource(t *testing.T) {
	invoice := invoicing.Invoice{Lines: []invoicing.Line{{Position: 1, Description: "PRESTARI SERVICII CF. CTR.", SourceFacts: &accounting.LineFacts{ItemDescription: "NR.102/25.06.2025- luna august 2026"}}}}
	got, source := invoiceContractReference(invoice)
	if got != "102/25.06.2025" || source != "Descrierea liniei 1" {
		t.Fatalf("reference=%q source=%q", got, source)
	}
}

func TestInvoiceContractReferencePrefersStructuredUBLReference(t *testing.T) {
	invoice := invoicing.Invoice{SourceFacts: &accounting.SourceFacts{ContractReferences: []string{"102/25.06.2025"}}, Lines: []invoicing.Line{{Position: 1, Description: "conform contract 999/01.01.2020"}}}
	got, source := invoiceContractReference(invoice)
	if got != "102/25.06.2025" || source != "Referință contractuală structurată din e-Factura" {
		t.Fatalf("reference=%q source=%q", got, source)
	}
}
