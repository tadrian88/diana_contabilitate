// Package accountingtest holds unmistakably synthetic acceptance data. None of
// these mappings/policies constitute Romanian accounting approval.
package accountingtest

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/money"
	"fmt"
	"time"
)

const (
	BuyerNormalizedCUI    = "90000001"
	SupplierNormalizedCUI = "90000002"
	BuyerCUI              = "RO" + BuyerNormalizedCUI
	SupplierCUI           = "RO" + SupplierNormalizedCUI
)

func Rate(v string) *money.Amount { a := money.MustParse(v); return &a }
func Fixture(client string) (*accounting.SourceFacts, *accounting.LineFacts, *accounting.Profile, *accounting.Pack) {
	approval := accounting.Approval{Actor: "TEST_ONLY synthetic accountant", At: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), Evidence: []string{"TEST_ONLY engineering assumptions; not a real accounting policy"}}
	amount := func(v string) *accounting.AmountFact {
		return &accounting.AmountFact{Amount: money.MustParse(v), Currency: "RON", Origin: accounting.Declared}
	}
	category := accounting.TaxCategory{Code: "S", Rate: Rate("21"), Scheme: "VAT"}
	facts := &accounting.SourceFacts{ParserVersion: "TEST_ONLY_V2", SourceDocumentID: "TEST_ONLY-source", SourceHash: "TEST_ONLY-hash", TypeCode: "380", SupplierVATID: SupplierCUI, BuyerVATID: BuyerCUI, SupplierCountry: "RO", BuyerCountry: "RO", CashAccounting: "UNKNOWN", VATTotals: []accounting.AmountFact{*amount("21")}, Subtotals: []accounting.TaxSubtotal{{TaxCategory: category, Base: *amount("100"), VAT: *amount("21")}}, LineExtension: amount("100"), TaxExclusive: amount("100"), TaxInclusive: amount("121"), Payable: amount("121")}
	line := &accounting.LineFacts{SourceID: "SOURCE-1", Path: "/Invoice/InvoiceLine[1]", TaxCategory: category, VATOrigin: accounting.Calculated, SellerItemID: "TEST_ONLY_ITEM"}
	profile := &accounting.Profile{TestOnly: true, ID: "TEST_ONLY-profile-" + client, ClientID: client, Version: 1, EffectiveFrom: "2026-01-01", Framework: "OMFP_1802_2014", ChartPolicy: "TEST_ONLY-chart-" + client, AccountCodes: []string{"628.TEST", "628.TEST2"}, TaxRegime: "PROFIT_TAX", VATRegistration: "ORDINARY_REGISTERED", DeductionActivity: "WITH_DEDUCTION_RIGHT", CashAccounting: "NO", ProRata: "NO", Approval: approval}
	pack := &accounting.Pack{ID: "TEST_ONLY-pack-" + client, ClientID: client, ProfileID: profile.ID, Version: 1, TestOnly: true, EffectiveFrom: "2026-01-01", Approval: approval, Mapping: accounting.MappingPolicy{Version: "TEST_ONLY_ORDINARY_FULL_V1", TestOnly: true, Approved: true, OrdinaryFullOmission: true, Approval: approval}}
	values := []accounting.Value{{Kind: "ACCOUNT", Account: "628.TEST"}, {Kind: "ORDINARY", Timing: "IMMEDIATE", SourceCategory: "S", SourceRate: Rate("21")}, {Kind: "FULL"}, {Kind: "FULLY_DEDUCTIBLE"}}
	for i, d := range accounting.Dimensions {
		id := fmt.Sprintf("TEST_ONLY-%s-%s", client, d)
		pack.Rules = append(pack.Rules, accounting.Rule{ID: id, VersionID: id + "-v1", Version: 1, Dimension: d, Scope: "GLOBAL", ClientPolicy: profile.ChartPolicy, AcquisitionPolicy: "TEST_ONLY business-purpose and supplier registration evidence", DateBasis: accounting.IssueDateBasis, EffectiveFrom: "2026-01-01", Predicate: accounting.Predicate{Version: "ORDINARY_INCOMING_V1", SupplierCashAccounting: "NO", SupplierVATRegistration: "ORDINARY_REGISTERED", SupplierID: SupplierCUI, SellerItemID: "TEST_ONLY_ITEM", Category: "S", Rate: money.MustParse("21"), Currency: "RON", TypeCode: "380"}, Result: values[i], Explanation: "TEST_ONLY exact approved engineering case", LegalBasis: "TEST_ONLY legal applicability assumed; not a released accounting rule", Evidence: approval.Evidence})
	}
	return facts, line, profile, pack
}

// XML includes distinct supplier legal/VAT identity, all source totals and VAT
// breakdown. rateElement may be empty to demonstrate absence versus explicit 0.
func XML(rateElement, lineVAT string) string {
	return `<?xml version="1.0"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"><ID>TEST_ONLY-001</ID><InvoiceTypeCode>380</InvoiceTypeCode><IssueDate>2026-09-15</IssueDate><DocumentCurrencyCode>RON</DocumentCurrencyCode><AccountingSupplierParty><Party><PostalAddress><Country><IdentificationCode>RO</IdentificationCode></Country></PostalAddress><PartyLegalEntity><RegistrationName>TEST_ONLY supplier</RegistrationName><CompanyID>TEST-LEGAL-S</CompanyID></PartyLegalEntity><PartyTaxScheme><CompanyID>` + SupplierCUI + `</CompanyID><TaxScheme><ID>VAT</ID></TaxScheme></PartyTaxScheme></Party></AccountingSupplierParty><AccountingCustomerParty><Party><PostalAddress><Country><IdentificationCode>RO</IdentificationCode></Country></PostalAddress><PartyLegalEntity><CompanyID>TEST-LEGAL-B</CompanyID></PartyLegalEntity><PartyTaxScheme><CompanyID>` + BuyerCUI + `</CompanyID><TaxScheme><ID>VAT</ID></TaxScheme></PartyTaxScheme></Party></AccountingCustomerParty><TaxTotal><TaxAmount currencyID="RON">21</TaxAmount><TaxSubtotal><TaxableAmount currencyID="RON">100</TaxableAmount><TaxAmount currencyID="RON">21</TaxAmount><TaxCategory><ID>S</ID><Percent>21</Percent><TaxScheme><ID>VAT</ID></TaxScheme></TaxCategory></TaxSubtotal></TaxTotal><LegalMonetaryTotal><LineExtensionAmount currencyID="RON">100</LineExtensionAmount><TaxExclusiveAmount currencyID="RON">100</TaxExclusiveAmount><TaxInclusiveAmount currencyID="RON">121</TaxInclusiveAmount><PayableAmount currencyID="RON">121</PayableAmount></LegalMonetaryTotal><InvoiceLine><ID>SOURCE-1</ID><InvoicedQuantity unitCode="H87">1</InvoicedQuantity><LineExtensionAmount currencyID="RON">100</LineExtensionAmount>` + lineVAT + `<Item><Name>TEST_ONLY service</Name><SellersItemIdentification><ID>TEST_ONLY_ITEM</ID></SellersItemIdentification><ClassifiedTaxCategory><ID>S</ID>` + rateElement + `<TaxScheme><ID>VAT</ID></TaxScheme></ClassifiedTaxCategory></Item><Price><PriceAmount currencyID="RON">100</PriceAmount><BaseQuantity>1</BaseQuantity></Price></InvoiceLine></Invoice>`
}
