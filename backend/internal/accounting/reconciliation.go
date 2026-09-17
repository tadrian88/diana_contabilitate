package accounting

import (
	"diana-contabilitate/backend/internal/money"
	"fmt"
	"math/big"
)

func rat(v string) *big.Rat {
	r, ok := new(big.Rat).SetString(v)
	if !ok {
		return new(big.Rat)
	}
	return r
}
func newRat(v money.Amount) (*big.Rat, bool) {
	if !v.Valid() {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(v.String())
	return r, ok
}
func Equal(a, b money.Amount) bool { return a.Valid() && b.Valid() && a.Equal(b) }

type SourceLine struct {
	ID                    string
	Facts                 *LineFacts
	Net, VAT, Total, Rate money.Amount
}

// Reconcile is deliberately bounded: ordinary invoices with no document
// adjustments and one currency. Unsupported allocations fail instead of rounding
// or synthesizing source totals. Explicit zero remains distinguishable from nil.
func Reconcile(f *SourceFacts, lines []SourceLine, gross money.Amount, currency string) error {
	fail := func(s string) error { return fmt.Errorf("Date sursă: %s", s) }
	if f == nil || f.ParserVersion == "" || f.SourceDocumentID == "" || f.SourceHash == "" {
		return fail("lipsește proveniența e-Factura")
	}
	if currency != "RON" || f.TaxCurrency != "" && f.TaxCurrency != "RON" || f.TypeCode != "380" || f.PrecedingInvoice != "" {
		return fail("documentul/moneda necesită tratament neacceptat")
	}
	if f.SupplierCountry != "RO" || f.BuyerCountry != "RO" || f.SupplierVATID == "" || f.BuyerVATID == "" {
		return fail("identitatea fiscală și țara trebuie confirmate")
	}
	if !oneOf(f.CashAccounting, "NO", "UNKNOWN") || f.TaxPointCode != "" {
		return fail("momentul deducerii nu este confirmat pentru domeniul inițial")
	}
	if len(f.Adjustments) > 0 {
		return fail("ajustările la nivel de document necesită alocare/revizuire")
	}
	netTotal, vatTotal := new(big.Rat), new(big.Rat)
	groups := map[string]*big.Rat{}
	if len(lines) == 0 || !gross.Valid() {
		return fail("lipsesc liniile/totalul")
	}
	for _, l := range lines {
		if l.Facts == nil || l.Facts.Rate == nil || !l.Facts.Rate.Valid() || !Equal(*l.Facts.Rate, l.Rate) || l.Facts.Code != "S" || l.Facts.Scheme != "VAT" || l.Facts.ExemptionCode != "" || l.Facts.ExemptionReason != "" || !oneOf(string(l.Facts.VATOrigin), string(Declared), string(Calculated)) {
			return fail("categoria/cota TVA ori originea valorii nu sunt suficiente")
		}
		for _, a := range []*AmountFact{l.Facts.NetAmount, l.Facts.VATAmount, l.Facts.PriceAmount} {
			if a != nil && (a.Currency != currency || !a.Amount.Valid()) {
				return fail("moneda/valoarea sursă a liniei este invalidă")
			}
		}
		if l.Facts.NetAmount != nil && !l.Facts.NetAmount.Amount.Equal(l.Net) || l.Facts.VATAmount != nil && (!l.Facts.VATAmount.Amount.Equal(l.VAT) || l.Facts.VATAmount.Origin != l.Facts.VATOrigin) {
			return fail("valorile monetare ale liniei diferă de faptele sursă")
		}
		expectedVAT := new(big.Rat).Quo(new(big.Rat).Mul(rat(l.Net.String()), rat(l.Rate.String())), rat("100"))
		if !l.VAT.Equal(money.Amount(expectedVAT.FloatString(2))) && !l.VAT.Equal(money.Amount(expectedVAT.FloatString(4))) {
			return fail("TVA liniei diferă de baza/cota sursă")
		}
		if len(l.Facts.Adjustments) > 0 || l.Facts.PriceBase != nil && (!l.Facts.PriceBase.Valid() || !l.Facts.PriceBase.Equal(money.MustParse("1"))) {
			return fail("prețul de bază/ajustările liniei necesită o mapare explicită")
		}
		if !l.Net.Valid() || !l.VAT.Valid() || !l.Total.Valid() || rat(l.Net.String()).Sign() < 0 || rat(l.VAT.String()).Sign() < 0 || rat(l.Rate.String()).Sign() <= 0 {
			return fail("valori de linie neacceptate")
		}
		if new(big.Rat).Add(rat(l.Net.String()), rat(l.VAT.String())).Cmp(rat(l.Total.String())) != 0 {
			return fail("valoare + TVA diferă de totalul liniei")
		}
		netTotal.Add(netTotal, rat(l.Net.String()))
		vatTotal.Add(vatTotal, rat(l.VAT.String()))
		key := l.Facts.Code + ":" + rat(l.Rate.String()).RatString()
		if groups[key] == nil {
			groups[key] = new(big.Rat)
		}
		groups[key].Add(groups[key], rat(l.Net.String()))
	}
	if new(big.Rat).Add(netTotal, vatTotal).Cmp(rat(gross.String())) != 0 {
		return fail("totalul facturii diferă de suma liniilor")
	}
	check := func(a *AmountFact, want *big.Rat) bool {
		return a != nil && a.Origin == Declared && a.Amount.Valid() && a.Currency == currency && rat(a.Amount.String()).Cmp(want) == 0
	}
	if !check(f.LineExtension, netTotal) || !check(f.TaxExclusive, netTotal) || !check(f.TaxInclusive, rat(gross.String())) {
		return fail("totalurile monetare distincte nu se reconciliază")
	}
	found := false
	for _, v := range f.VATTotals {
		if v.Currency == currency {
			if found || v.Origin != Declared || !v.Amount.Valid() || rat(v.Amount.String()).Cmp(vatTotal) != 0 {
				return fail("totalul TVA nu se reconciliază")
			}
			found = true
		}
	}
	if !found || len(f.Subtotals) != len(groups) {
		return fail("lipsește defalcarea TVA")
	}
	seen := map[string]bool{}
	subtotalVAT := new(big.Rat)
	for _, s := range f.Subtotals {
		if s.Rate == nil || !s.Rate.Valid() || s.Scheme != "VAT" || s.ExemptionCode != "" || s.ExemptionReason != "" {
			return fail("defalcare TVA neacceptată")
		}
		key := s.Code + ":" + rat(s.Rate.String()).RatString()
		expected := groups[key]
		if expected == nil || seen[key] || !check(&s.Base, expected) || !s.VAT.Amount.Valid() || s.VAT.Currency != currency || s.VAT.Origin != Declared {
			return fail("bazele categoriilor TVA nu se reconciliază")
		}
		seen[key] = true
		categoryLines := new(big.Rat)
		for _, l := range lines {
			if l.Facts.Code == s.Code && Equal(l.Rate, *s.Rate) {
				categoryLines.Add(categoryLines, rat(l.VAT.String()))
			}
		}
		if categoryLines.Cmp(rat(s.VAT.Amount.String())) != 0 {
			return fail("TVA categoriei diferă de TVA liniilor; revizuiți rotunjirea")
		}
		subtotalVAT.Add(subtotalVAT, rat(s.VAT.Amount.String()))
	}
	if subtotalVAT.Cmp(vatTotal) != 0 {
		return fail("defalcarea TVA diferă de total")
	}
	payable := rat(gross.String())
	for _, a := range []*AmountFact{f.Prepaid, f.Rounding} {
		if a != nil && (!a.Amount.Valid() || a.Currency != currency || a.Origin != Declared) {
			return fail("avans/rotunjire invalidă")
		}
	}
	if f.Prepaid != nil {
		payable.Sub(payable, rat(f.Prepaid.Amount.String()))
	}
	if f.Rounding != nil {
		payable.Add(payable, rat(f.Rounding.Amount.String()))
	}
	if !check(f.Payable, payable) {
		return fail("suma de plată nu se reconciliază")
	}
	return nil
}

// MatchProduct uses supported source money precision without binary floats.
func MatchProduct(a, b, total money.Amount) bool {
	if !a.Valid() || !b.Valid() || !total.Valid() {
		return false
	}
	product := new(big.Rat).Mul(rat(a.String()), rat(b.String()))
	return product.Cmp(rat(total.String())) == 0 || total.Equal(money.Amount(product.FloatString(2))) || total.Equal(money.Amount(product.FloatString(4)))
}
