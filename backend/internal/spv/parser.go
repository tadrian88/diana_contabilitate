package spv

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"math/big"
	"path"
	"sort"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

const maxXMLBytes = 12 << 20

type UBLParser struct{}

type ublDocument struct {
	TypeCode           string          `xml:"InvoiceTypeCode"`
	CreditTypeCode     string          `xml:"CreditNoteTypeCode"`
	TaxCurrency        string          `xml:"TaxCurrencyCode"`
	TaxPointDate       string          `xml:"TaxPointDate"`
	Period             ublPeriod       `xml:"InvoicePeriod"`
	Adjustments        []ublAdjustment `xml:"AllowanceCharge"`
	Notes              []string        `xml:"Note"`
	PrecedingInvoice   string          `xml:"BillingReference>InvoiceDocumentReference>ID"`
	BuyerReference     string          `xml:"BuyerReference"`
	ContractReferences []string        `xml:"ContractDocumentReference>ID"`
	XMLName            xml.Name
	ID                 string        `xml:"ID"`
	IssueDate          string        `xml:"IssueDate"`
	DueDate            string        `xml:"DueDate"`
	Currency           string        `xml:"DocumentCurrencyCode"`
	Supplier           ublParty      `xml:"AccountingSupplierParty>Party"`
	Buyer              ublParty      `xml:"AccountingCustomerParty>Party"`
	Monetary           ublMonetary   `xml:"LegalMonetaryTotal"`
	TaxTotals          []ublTaxTotal `xml:"TaxTotal"`
	InvoiceLines       []ublLine     `xml:"InvoiceLine"`
	CreditLines        []ublLine     `xml:"CreditNoteLine"`
}
type ublParty struct {
	Country          string `xml:"PostalAddress>Country>IdentificationCode"`
	Name             string `xml:"PartyName>Name"`
	RegistrationName string `xml:"PartyLegalEntity>RegistrationName"`
	TaxID            string `xml:"PartyTaxScheme>CompanyID"`
	LegalID          string `xml:"PartyLegalEntity>CompanyID"`
}
type ublMonetary struct {
	Prepaid       ublAmount `xml:"PrepaidAmount"`
	Rounding      ublAmount `xml:"PayableRoundingAmount"`
	TaxInclusive  ublAmount `xml:"TaxInclusiveAmount"`
	LineExtension ublAmount `xml:"LineExtensionAmount"`
	TaxExclusive  ublAmount `xml:"TaxExclusiveAmount"`
	Payable       ublAmount `xml:"PayableAmount"`
}
type ublTaxTotal struct {
	Amount    ublAmount     `xml:"TaxAmount"`
	Subtotals []ublSubtotal `xml:"TaxSubtotal"`
}
type ublLine struct {
	Adjustments     []ublAdjustment `xml:"AllowanceCharge"`
	ID, Note        string
	InvoiceQuantity quantity  `xml:"InvoicedQuantity"`
	CreditQuantity  quantity  `xml:"CreditedQuantity"`
	LineExtension   ublAmount `xml:"LineExtensionAmount"`
	Item            ublItem
	Price           ublPrice
	TaxTotal        ublTaxTotal
}
type quantity struct {
	Value string `xml:",chardata"`
	Unit  string `xml:"unitCode,attr"`
}
type ublItem struct {
	Name, Description string
	Tax               ublTaxCategory `xml:"ClassifiedTaxCategory"`
	SellerID          string         `xml:"SellersItemIdentification>ID"`
	StandardID        string         `xml:"StandardItemIdentification>ID"`
}
type ublTaxCategory struct {
	ExemptionCode   string `xml:"TaxExemptionReasonCode"`
	ExemptionReason string `xml:"TaxExemptionReason"`
	Scheme          string `xml:"TaxScheme>ID"`
	Rate            string `xml:"Percent"`
	Code            string `xml:"ID"`
}
type ublPrice struct {
	Amount ublAmount `xml:"PriceAmount"`
	Base   string    `xml:"BaseQuantity"`
}

type ublAmount struct {
	Value    string `xml:",chardata"`
	Currency string `xml:"currencyID,attr"`
}
type ublSubtotal struct {
	Base     ublAmount      `xml:"TaxableAmount"`
	VAT      ublAmount      `xml:"TaxAmount"`
	Category ublTaxCategory `xml:"TaxCategory"`
}
type ublPeriod struct {
	Start string `xml:"StartDate"`
	End   string `xml:"EndDate"`
	Code  string `xml:"DescriptionCode"`
}
type ublAdjustment struct {
	Charge   bool           `xml:"ChargeIndicator"`
	Amount   ublAmount      `xml:"Amount"`
	Category ublTaxCategory `xml:"TaxCategory"`
	Reason   string         `xml:"AllowanceChargeReason"`
}

func amountFact(a ublAmount, currency string) (*accounting.AmountFact, error) {
	if strings.TrimSpace(a.Value) == "" {
		return nil, nil
	}
	value, err := parseAmount(a.Value)
	if err != nil {
		return nil, err
	}
	return &accounting.AmountFact{Amount: value, Currency: first(a.Currency, currency), Origin: accounting.Declared}, nil
}
func categoryFact(c ublTaxCategory) (accounting.TaxCategory, error) {
	result := accounting.TaxCategory{Code: c.Code, Scheme: c.Scheme, ExemptionCode: c.ExemptionCode, ExemptionReason: c.ExemptionReason}
	if strings.TrimSpace(c.Rate) != "" {
		rate, err := parseAmount(c.Rate)
		if err != nil {
			return result, err
		}
		result.Rate = &rate
	}
	return result, nil
}
func adjustmentFacts(values []ublAdjustment, currency string) ([]accounting.Adjustment, error) {
	var result []accounting.Adjustment
	for _, v := range values {
		a, err := amountFact(v.Amount, currency)
		if err != nil || a == nil {
			return nil, fmt.Errorf("%w: invalid allowance/charge", ErrPermanent)
		}
		c, err := categoryFact(v.Category)
		if err != nil {
			return nil, err
		}
		result = append(result, accounting.Adjustment{Charge: v.Charge, Amount: *a, Category: c, Reason: v.Reason})
	}
	return result, nil
}

func (p UBLParser) Parse(rawZIP []byte) (ParsedDocument, error) {
	xmlBytes, err := safeInvoiceXML(rawZIP)
	if err != nil {
		return ParsedDocument{}, err
	}
	var document ublDocument
	if err = xml.Unmarshal(xmlBytes, &document); err != nil {
		return ParsedDocument{}, fmt.Errorf("%w: malformed UBL XML", ErrPermanent)
	}
	root := document.XMLName.Local
	lines := document.InvoiceLines
	if root == "CreditNote" {
		lines = document.CreditLines
	}
	if root != "Invoice" && root != "CreditNote" {
		return ParsedDocument{}, fmt.Errorf("%w: unsupported XML root %q", ErrPermanent, root)
	}
	issueDate, err := parseDate(document.IssueDate)
	if err != nil {
		return ParsedDocument{}, err
	}
	var dueDate *time.Time
	if strings.TrimSpace(document.DueDate) != "" {
		parsed, dateErr := parseDate(document.DueDate)
		if dateErr != nil {
			return ParsedDocument{}, dateErr
		}
		dueDate = &parsed
	}
	supplierCUI := partyCUI(document.Supplier)
	buyerCUI := partyCUI(document.Buyer)
	supplierName := first(document.Supplier.RegistrationName, document.Supplier.Name)
	currency := strings.ToUpper(strings.TrimSpace(document.Currency))
	if strings.TrimSpace(document.ID) == "" || supplierName == "" || supplierCUI == "" || buyerCUI == "" || len(currency) != 3 || len(lines) == 0 {
		return ParsedDocument{}, fmt.Errorf("%w: UBL document misses required identity/header/line data", ErrPermanent)
	}
	vatHeader := ""
	for _, v := range document.TaxTotals {
		if first(v.Amount.Currency, currency) == currency {
			vatHeader = v.Amount.Value
			break
		}
	}
	totalRaw := first(document.Monetary.TaxInclusive.Value, addDecimal(document.Monetary.TaxExclusive.Value, vatHeader), addDecimal(document.Monetary.LineExtension.Value, vatHeader))
	total, err := parseAmount(totalRaw)
	if err != nil {
		return ParsedDocument{}, err
	}
	documentType := invoicing.DocumentTypeInvoice
	if root == "CreditNote" {
		documentType = invoicing.DocumentTypeCreditNote
	}
	facts := &accounting.SourceFacts{ParserVersion: ParserVersion + "_ACCOUNTING_V2", TypeCode: first(document.TypeCode, document.CreditTypeCode), SupplierVATID: document.Supplier.TaxID, SupplierLegalID: document.Supplier.LegalID, BuyerVATID: document.Buyer.TaxID, BuyerLegalID: document.Buyer.LegalID, SupplierCountry: document.Supplier.Country, BuyerCountry: document.Buyer.Country, TaxCurrency: document.TaxCurrency, TaxPointDate: document.TaxPointDate, PeriodStart: document.Period.Start, PeriodEnd: document.Period.End, TaxPointCode: document.Period.Code, CashAccounting: "UNKNOWN", PrecedingInvoice: document.PrecedingInvoice, BuyerReference: strings.TrimSpace(document.BuyerReference), ContractReferences: nonemptyTrimmed(document.ContractReferences), Notes: nonemptyTrimmed(document.Notes)}
	for _, date := range []string{facts.TaxPointDate, facts.PeriodStart, facts.PeriodEnd} {
		if date != "" {
			if _, err := parseDate(date); err != nil {
				return ParsedDocument{}, err
			}
		}
	}
	for _, note := range document.Notes {
		if strings.Contains(strings.ToLower(note), "tva la încasare") || strings.Contains(strings.ToLower(note), "tva la incasare") {
			facts.CashAccounting = "YES"
		}
	}
	for _, pair := range []struct {
		raw    ublAmount
		target **accounting.AmountFact
	}{{document.Monetary.LineExtension, &facts.LineExtension}, {document.Monetary.TaxExclusive, &facts.TaxExclusive}, {document.Monetary.TaxInclusive, &facts.TaxInclusive}, {document.Monetary.Payable, &facts.Payable}, {document.Monetary.Prepaid, &facts.Prepaid}, {document.Monetary.Rounding, &facts.Rounding}} {
		value, err := amountFact(pair.raw, currency)
		if err != nil {
			return ParsedDocument{}, err
		}
		*pair.target = value
	}
	for _, t := range document.TaxTotals {
		a, err := amountFact(t.Amount, currency)
		if err != nil {
			return ParsedDocument{}, err
		}
		if a != nil {
			facts.VATTotals = append(facts.VATTotals, *a)
		}
		for _, sub := range t.Subtotals {
			base, err := amountFact(sub.Base, currency)
			if err != nil || base == nil {
				return ParsedDocument{}, fmt.Errorf("%w: subtotal base", ErrPermanent)
			}
			vat, err := amountFact(sub.VAT, currency)
			if err != nil || vat == nil {
				return ParsedDocument{}, fmt.Errorf("%w: subtotal VAT", ErrPermanent)
			}
			category, err := categoryFact(sub.Category)
			if err != nil {
				return ParsedDocument{}, err
			}
			facts.Subtotals = append(facts.Subtotals, accounting.TaxSubtotal{TaxCategory: category, Base: *base, VAT: *vat})
		}
	}
	facts.Adjustments, err = adjustmentFacts(document.Adjustments, currency)
	if err != nil {
		return ParsedDocument{}, err
	}
	input := invoicing.IngestionInput{ModelVersion: accounting.ModelVersion, SourceFacts: facts, Source: SourceName, DocumentType: documentType, SupplierName: supplierName, SupplierCUI: &supplierCUI, DocumentNumber: strings.TrimSpace(document.ID), IssueDate: issueDate, DueDate: dueDate, Total: money.Money{Amount: total, Currency: currency}}
	// The buyer is kept as the customer; it is used only when the invoice turns
	// out to be issued by the client (D-124). A natural person is identified by
	// a CNP in PartyLegalEntity/CompanyID.
	input.CustomerName = strings.TrimSpace(first(document.Buyer.RegistrationName, document.Buyer.Name))
	input.CustomerIdentifier = buyerCUI
	input.CustomerCountry = document.Buyer.Country
	for index, external := range lines {
		q := external.InvoiceQuantity
		if root == "CreditNote" {
			q = external.CreditQuantity
		}
		quantityValue, e1 := parseAmount(q.Value)
		unitPrice, e2 := parseAmount(external.Price.Amount.Value)
		net, e3 := parseAmount(external.LineExtension.Value)
		category, e4 := categoryFact(external.Item.Tax)
		rate := money.MustParse("0")
		if category.Rate != nil {
			rate = *category.Rate
		}
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || q.Unit == "" {
			return ParsedDocument{}, fmt.Errorf("%w: invalid decimal or unit on line %d", ErrPermanent, index+1)
		}
		vatRaw := strings.TrimSpace(external.TaxTotal.Amount.Value)
		origin := accounting.Declared
		if vatRaw == "" {
			origin = accounting.Calculated
		}
		if vatRaw == "" {
			if category.Rate == nil {
				origin = accounting.Unknown
			}
			vatRaw, err = percentage(net.String(), rate.String())
			if err != nil {
				return ParsedDocument{}, err
			}
		}
		vat, err := parseAmount(vatRaw)
		if err != nil {
			return ParsedDocument{}, err
		}
		lineTotalRaw := addDecimal(net.String(), vat.String())
		lineTotal, err := parseAmount(lineTotalRaw)
		if err != nil {
			return ParsedDocument{}, err
		}
		description := first(external.Item.Name, external.Item.Description)
		if description == "" {
			return ParsedDocument{}, fmt.Errorf("%w: line %d has no description", ErrPermanent, index+1)
		}
		var additional *string
		parts := []string{}
		if external.Item.SellerID != "" {
			parts = append(parts, "seller_item_id="+external.Item.SellerID)
		}
		if external.Item.StandardID != "" {
			parts = append(parts, "standard_item_id="+external.Item.StandardID)
		}
		if external.Note != "" {
			parts = append(parts, "note="+external.Note)
		}
		if external.Item.Description != "" && external.Item.Description != description {
			parts = append(parts, "item_description="+external.Item.Description)
		}
		if len(parts) > 0 {
			value := strings.Join(parts, "; ")
			additional = &value
		}
		lineFacts := &accounting.LineFacts{SourceID: external.ID, Path: fmt.Sprintf("/%s/%sLine[%d]", root, root, index+1), TaxCategory: category, VATOrigin: origin, SellerItemID: external.Item.SellerID, StandardItemID: external.Item.StandardID, ItemName: strings.TrimSpace(external.Item.Name), ItemDescription: strings.TrimSpace(external.Item.Description), Note: strings.TrimSpace(external.Note)}
		lineFacts.NetAmount, err = amountFact(external.LineExtension, currency)
		if err != nil {
			return ParsedDocument{}, err
		}
		lineFacts.PriceAmount, err = amountFact(external.Price.Amount, currency)
		if err != nil {
			return ParsedDocument{}, err
		}
		if origin != accounting.Unknown {
			lineFacts.VATAmount = &accounting.AmountFact{Amount: vat, Currency: first(external.TaxTotal.Amount.Currency, currency), Origin: origin}
		}
		if strings.TrimSpace(external.Price.Base) != "" {
			base, err := parseAmount(external.Price.Base)
			if err != nil {
				return ParsedDocument{}, err
			}
			lineFacts.PriceBase = &base
		}
		lineFacts.Adjustments, err = adjustmentFacts(external.Adjustments, currency)
		if err != nil {
			return ParsedDocument{}, err
		}
		input.Lines = append(input.Lines, invoicing.Line{SourceFacts: lineFacts, Position: index + 1, Description: description, Unit: q.Unit, VATRate: rate, VATValue: vat, Quantity: quantityValue, UnitPrice: unitPrice, NetValue: net, TotalValue: lineTotal, AdditionalInfo: additional})
	}
	alignCalculatedVAT(input.Lines, facts.Subtotals, currency)
	return ParsedDocument{Invoice: input, BuyerCUI: buyerCUI, SupplierCUI: supplierCUI, Format: ParserTypeUBL, Version: facts.ParserVersion}, nil
}

// alignCalculatedVAT makes line VAT that Diana had to calculate (the e-Factura
// line carries no VAT amount) agree with the VAT the invoice declares for its
// category and rate (D-132). The issuer rounds VAT per category, so net × rate
// per line can differ by a fraction of a cent. Lines are rounded to two
// decimals and the remaining difference is spread one cent per line, largest
// net first. A group is left untouched when it has no single declared subtotal
// in the document currency or when the difference exceeds one cent per line:
// that is a real discrepancy, not rounding. Declared line VAT is never changed.
func alignCalculatedVAT(lines []invoicing.Line, subtotals []accounting.TaxSubtotal, currency string) {
	cent := big.NewRat(1, 100)
	key := func(code string, rate *money.Amount) (string, bool) {
		if rate == nil {
			return "", false
		}
		value, ok := new(big.Rat).SetString(rate.String())
		if !ok {
			return "", false
		}
		return code + ":" + value.RatString(), true
	}
	declared := map[string]*big.Rat{}
	ambiguous := map[string]bool{}
	for _, subtotal := range subtotals {
		group, ok := key(subtotal.Code, subtotal.Rate)
		amount, valid := new(big.Rat).SetString(subtotal.VAT.Amount.String())
		if !ok || !valid || subtotal.VAT.Currency != currency {
			continue
		}
		if _, exists := declared[group]; exists {
			ambiguous[group] = true
		}
		declared[group] = amount
	}
	groups := map[string][]int{}
	for index, line := range lines {
		if line.SourceFacts == nil {
			continue
		}
		if group, ok := key(line.SourceFacts.Code, line.SourceFacts.Rate); ok {
			groups[group] = append(groups[group], index)
		}
	}
	for group, indexes := range groups {
		target, ok := declared[group]
		if !ok || ambiguous[group] {
			continue
		}
		rounded := map[int]*big.Rat{}
		calculated := []int{}
		sum := new(big.Rat)
		for _, index := range indexes {
			value, valid := new(big.Rat).SetString(lines[index].VATValue.String())
			if !valid {
				calculated = nil
				break
			}
			if lines[index].SourceFacts.VATOrigin == accounting.Calculated {
				value, _ = new(big.Rat).SetString(roundHalfUp(value, 2))
				rounded[index] = value
				calculated = append(calculated, index)
			}
			sum.Add(sum, value)
		}
		if len(calculated) == 0 {
			continue
		}
		difference := new(big.Rat).Sub(target, sum)
		steps := new(big.Rat).Quo(difference, cent)
		if !steps.IsInt() || new(big.Rat).Abs(steps).Cmp(big.NewRat(int64(len(calculated)), 1)) > 0 {
			continue
		}
		sort.SliceStable(calculated, func(left, right int) bool {
			a, _ := new(big.Rat).SetString(lines[calculated[left]].NetValue.String())
			b, _ := new(big.Rat).SetString(lines[calculated[right]].NetValue.String())
			return a.Cmp(b) > 0
		})
		step := new(big.Rat).Set(cent)
		if difference.Sign() < 0 {
			step.Neg(step)
		}
		for position := 0; position < int(new(big.Rat).Abs(steps).Num().Int64()); position++ {
			rounded[calculated[position]].Add(rounded[calculated[position]], step)
		}
		for _, index := range calculated {
			vat, err := parseAmount(rounded[index].FloatString(2))
			if err != nil {
				continue
			}
			total, err := parseAmount(addDecimal(lines[index].NetValue.String(), vat.String()))
			if err != nil {
				continue
			}
			lines[index].VATValue, lines[index].TotalValue = vat, total
			if lines[index].SourceFacts.VATAmount != nil {
				lines[index].SourceFacts.VATAmount.Amount = vat
			}
		}
	}
}

// roundHalfUp rounds a decimal away from zero at the given number of decimals.
func roundHalfUp(value *big.Rat, decimals int) string {
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	scaled := new(big.Rat).Mul(value, scale)
	half := big.NewRat(1, 2)
	if scaled.Sign() < 0 {
		half.Neg(half)
	}
	scaled.Add(scaled, half)
	integer := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	return new(big.Rat).Quo(new(big.Rat).SetInt(integer), scale).FloatString(decimals)
}

func nonemptyTrimmed(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func safeInvoiceXML(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxResponseBytes {
		return nil, fmt.Errorf("%w: invalid ZIP size", ErrPermanent)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid ZIP", ErrPermanent)
	}
	if len(reader.File) == 0 || len(reader.File) > 8 {
		return nil, fmt.Errorf("%w: unsupported ZIP file count", ErrPermanent)
	}
	var selected *zip.File
	for _, file := range reader.File {
		clean := path.Clean(strings.ReplaceAll(file.Name, "\\", "/"))
		lower := strings.ToLower(clean)
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return nil, fmt.Errorf("%w: unsafe ZIP entry", ErrPermanent)
		}
		if strings.HasSuffix(lower, ".xml") && !strings.Contains(lower, "semnatura") {
			if selected != nil {
				return nil, fmt.Errorf("%w: multiple invoice XML files", ErrPermanent)
			}
			selected = file
		}
	}
	if selected == nil || selected.UncompressedSize64 > maxXMLBytes {
		return nil, fmt.Errorf("%w: missing or oversized invoice XML", ErrPermanent)
	}
	stream, err := selected.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open invoice XML", ErrPermanent)
	}
	defer stream.Close()
	result, err := readBounded(stream, maxXMLBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: oversized invoice XML", ErrPermanent)
	}
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(result, []byte{0xef, 0xbb, 0xbf}))
	if len(trimmed) == 0 || trimmed[0] != '<' {
		return nil, fmt.Errorf("%w: ZIP invoice is not XML", ErrPermanent)
	}
	return result, nil
}

func parseDate(raw string) (time.Time, error) {
	value, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid UBL date", ErrPermanent)
	}
	return value.UTC(), nil
}
func partyCUI(p ublParty) string { return strings.TrimSpace(first(p.TaxID, p.LegalID)) }
func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseAmount(raw string) (money.Amount, error) {
	normalized, err := normalizeDecimal(raw)
	if err != nil {
		return "", fmt.Errorf("%w: invalid decimal", ErrPermanent)
	}
	return money.Parse(normalized)
}
func parseAmountDefaultZero(raw string) (money.Amount, error) {
	if strings.TrimSpace(raw) == "" {
		return money.Parse("0")
	}
	return parseAmount(raw)
}
func normalizeDecimal(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("empty")
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(strings.TrimPrefix(value, "-"), "+")
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return "", fmt.Errorf("syntax")
	}
	for _, r := range strings.Join(parts, "") {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("syntax")
		}
	}
	integer := strings.TrimLeft(parts[0], "0")
	if integer == "" {
		integer = "0"
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = strings.TrimRight(parts[1], "0")
	}
	if len(fraction) > 4 {
		return "", fmt.Errorf("scale")
	}
	result := integer
	if fraction != "" {
		result += "." + fraction
	}
	if negative && result != "0" {
		result = "-" + result
	}
	return result, nil
}
func addDecimal(left, right string) string {
	a, ok := new(big.Rat).SetString(strings.TrimSpace(left))
	if !ok {
		return ""
	}
	b, ok := new(big.Rat).SetString(strings.TrimSpace(right))
	if !ok {
		return ""
	}
	return a.Add(a, b).FloatString(4)
}
func percentage(value, rate string) (string, error) {
	a, ok := new(big.Rat).SetString(value)
	if !ok {
		return "", fmt.Errorf("decimal")
	}
	b, ok := new(big.Rat).SetString(rate)
	if !ok {
		return "", fmt.Errorf("decimal")
	}
	return a.Mul(a, b).Quo(a, big.NewRat(100, 1)).FloatString(4), nil
}
