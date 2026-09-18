package classification

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/money"
	"testing"
)

func domainInput() InvoiceContext {
	f, l, p, pack := accountingtest.Fixture("A")
	return InvoiceContext{ModelVersion: accounting.ModelVersion, ClientID: "A", SupplierID: accountingtest.SupplierCUI, IssueDate: "2026-09-15", DocumentType: "INVOICE", Currency: "RON", SourceFacts: f, Snapshot: &accounting.Snapshot{Profile: p, Pack: pack}, Lines: []LineContext{{ID: "line", Description: "TEST_ONLY service", VATRate: money.MustParse("21"), VATValue: money.MustParse("21"), SourceFacts: l}}}
}
func TestDomainFourDecisionsAndNoProductionSynthetic(t *testing.T) {
	in := domainInput()
	out, err := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if err != nil || len(out.Proposals) != 4 {
		t.Fatal(err, out)
	}
	for _, p := range out.Proposals {
		if p.RequiresReview || p.TypedValue == nil || p.Evidence == nil || p.Rule == nil || p.Dimension == DimensionDeductibility || p.Dimension == DimensionVAT {
			t.Fatal(p)
		}
	}
	out, _ = (DomainPolicy{}).Evaluate(domainInput())
	for _, p := range out.Proposals {
		if !p.RequiresReview {
			t.Fatal("synthetic enabled in production")
		}
	}
}

func TestDomainDraftAndUnapprovedPackNeverClassify(t *testing.T) {
	for _, edit := range []func(*InvoiceContext){
		func(in *InvoiceContext) { in.Snapshot.Pack = nil }, // draft exists only offline
		func(in *InvoiceContext) { in.Snapshot.Pack.Approval = accounting.Approval{} },
		func(in *InvoiceContext) { in.SupplierID = "WRONG-SUPPLIER" },
		func(in *InvoiceContext) {
			end := in.IssueDate
			in.Snapshot.Pack.EffectiveTo = &end
			in.IssueDate = "2026-09-16"
		},
	} {
		in := domainInput()
		edit(&in)
		out, err := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range out.Proposals {
			if !p.RequiresReview {
				t.Fatal("draft/unapproved/out-of-scope execution", p)
			}
		}
	}
}
func TestDomainMissingAndUnsupportedContext(t *testing.T) {
	cases := []struct {
		name string
		edit func(*InvoiceContext)
	}{
		{"unknown profile", func(i *InvoiceContext) { i.Snapshot.Profile = nil }},
		{"unknown registration", func(i *InvoiceContext) { i.Snapshot.Profile.VATRegistration = "UNKNOWN" }},
		{"client isolation", func(i *InvoiceContext) { i.ClientID = "B" }},
		{"account policy", func(i *InvoiceContext) { i.Snapshot.Profile.ChartPolicy = "" }},
		{"special", func(i *InvoiceContext) { i.Lines[0].SourceFacts.Code = "AE" }},
		{"zero", func(i *InvoiceContext) {
			i.Lines[0].SourceFacts.Rate = accountingtest.Rate("0")
			i.Lines[0].VATRate = money.MustParse("0")
		}},
		{"missing", func(i *InvoiceContext) { i.Lines[0].SourceFacts.Rate = nil }},
		{"cash profile", func(i *InvoiceContext) { i.Snapshot.Profile.CashAccounting = "YES" }},
		{"cash source", func(i *InvoiceContext) { i.SourceFacts.CashAccounting = "YES" }},
		{"pro rata", func(i *InvoiceContext) { i.Snapshot.Profile.ProRata = "YES" }},
		{"credit", func(i *InvoiceContext) { i.DocumentType = "CREDIT_NOTE" }},
		{"currency", func(i *InvoiceContext) { i.Currency = "EUR" }},
		{"tax date", func(i *InvoiceContext) { i.SourceFacts.TaxPointDate = "2026-08-01" }},
		{"unknown supplier registration", func(i *InvoiceContext) {
			for n := range i.Snapshot.Pack.Rules {
				i.Snapshot.Pack.Rules[n].Predicate.SupplierVATRegistration = "UNKNOWN"
			}
		}},
		{"unknown supplier cash", func(i *InvoiceContext) {
			for n := range i.Snapshot.Pack.Rules {
				i.Snapshot.Pack.Rules[n].Predicate.SupplierCashAccounting = "UNKNOWN"
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := domainInput()
			c.edit(&in)
			out, err := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range out.Proposals {
				if !p.RequiresReview {
					t.Fatal("unsupported automatic", p)
				}
			}
		})
	}
}
func TestDomainEffectiveDatingAndAmbiguity(t *testing.T) {
	in := domainInput()
	in.IssueDate = "2025-12-31"
	out, _ := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if !out.Proposals[0].RequiresReview {
		t.Fatal("future profile")
	}
	in = domainInput()
	duplicate := in.Snapshot.Pack.Rules[0]
	duplicate.VersionID += "-v2"
	duplicate.Version = 2
	in.Snapshot.Pack.Rules = append(in.Snapshot.Pack.Rules, duplicate)
	out, _ = (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if out.Proposals[0].Source != SourceAmbiguous {
		t.Fatal("overlap picked recency")
	}
}
func TestDomainMicroExplicitInapplicability(t *testing.T) {
	in := domainInput()
	in.Snapshot.Profile.TaxRegime = "MICROENTERPRISE"
	out, _ := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	p := out.Proposals[3]
	if p.TypedValue == nil || p.TypedValue.Kind != "NOT_APPLICABLE" || !p.RequiresReview {
		t.Fatal(p)
	}
}
func TestDomainConfirmedContractOnly(t *testing.T) {
	in := domainInput()
	for n := range in.Snapshot.Pack.Rules {
		in.Snapshot.Pack.Rules[n].Predicate.ContractReference = "TEST_ONLY_CONTRACT"
		in.Snapshot.Pack.Rules[n].Predicate.SellerItemID = ""
	}
	out, _ := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if !out.Proposals[0].RequiresReview {
		t.Fatal("unconfirmed context accepted")
	}
	in.Snapshot.ContractID = "confirmed-contract"
	in.Snapshot.ContractReference = "TEST_ONLY_CONTRACT"
	out, _ = (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if out.Proposals[0].RequiresReview {
		t.Fatal("confirmed structured contract ignored")
	}
}

func TestDomainAllSpecialCategoriesStayManual(t *testing.T) {
	for _, code := range []string{"Z", "E", "AE", "K", "G", "O", "L", "M", "UNKNOWN", ""} {
		t.Run(code, func(t *testing.T) {
			in := domainInput()
			in.Lines[0].SourceFacts.Code = code
			out, _ := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
			for _, p := range out.Proposals {
				if !p.RequiresReview {
					t.Fatal("special category automated", p)
				}
			}
		})
	}
}
func TestDomainRuleDateBoundariesAndClientOverride(t *testing.T) {
	in := domainInput()
	end := in.IssueDate
	for n := range in.Snapshot.Pack.Rules {
		in.Snapshot.Pack.Rules[n].EffectiveTo = &end
	}
	out, _ := (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if out.Proposals[0].RequiresReview {
		t.Fatal("inclusive date boundary")
	}
	in.IssueDate = "2026-09-16"
	out, _ = (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if !out.Proposals[0].RequiresReview {
		t.Fatal("expired rule automated")
	}
	in = domainInput()
	override := in.Snapshot.Pack.Rules[0]
	override.ID += "-override"
	override.VersionID += "-override"
	override.ParentID = in.Snapshot.Pack.Rules[0].ID
	override.Scope = "CLIENT_OVERRIDE"
	override.Result.Account = "628.TEST2"
	in.Snapshot.Pack.Rules = append(in.Snapshot.Pack.Rules, override)
	out, _ = (DomainPolicy{AllowTestOnly: true}).Evaluate(in)
	if out.Proposals[0].TypedValue == nil || out.Proposals[0].TypedValue.Account != "628.TEST2" {
		t.Fatal("direct override not selected")
	}
}
