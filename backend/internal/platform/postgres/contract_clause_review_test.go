package postgres

import (
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
)

func TestIdentityClauseIsCoveredOnlyWhenItNamesNothingButThePartiesCUIs(t *testing.T) {
	party := "1.1. BYTEGUARD IT SOLUTIONS S.R.L., cu sediul în Bd. Iuliu Maniu nr. 7, corp A, et. 2, București, Sector 6, înregistrată la Registrul Comerțului sub nr. J2018004511402, cod de înregistrare fiscală RO38920171"
	client := "1.2. SOFTCO2 S.R.L., cu sediul în Int Gheorghe Simionescu, Nr.19, București, înregistrată la Registrul Comerțului sub nr. J2024004283403, cod de înregistrare fiscală RO49678244"
	supplierOnly, parties := commercialvalidation.ClauseCoveredBySupplierIdentity, commercialvalidation.ClauseCoveredByPartyIdentity
	cases := []struct {
		name, snippet, supplier, buyer, want string
	}{
		{"party clause with trade register number", party, "RO38920171", "RO49678244", supplierOnly},
		{"supplier stored without RO", party, "38920171", "RO49678244", supplierOnly},
		{"CUI written with a space", "cod fiscal RO 38920171", "RO38920171", "RO49678244", supplierOnly},
		{"the client's own party clause", client, "RO38920171", "RO49678244", parties},
		{"both parties named", "Prestator RO38920171, Beneficiar RO49678244", "RO38920171", "RO49678244", parties},
		{"another company's CUI", "cod de înregistrare fiscală RO11223344", "RO38920171", "RO49678244", ""},
		{"a party disagreement", "Prestator RO38920171, semnat de RO11223344", "RO38920171", "RO49678244", ""},
		{"client CUI with no client known", client, "RO38920171", "", ""},
		{"no CUI at all", "BYTEGUARD IT SOLUTIONS S.R.L., J2018004511402", "RO38920171", "RO49678244", ""},
		{"dossier without supplier CUI", party, "", "RO49678244", ""},
	}
	for _, item := range cases {
		if got := identityCoverage(item.snippet, item.supplier, item.buyer); got != item.want {
			t.Errorf("%s: coverage=%q, want %q", item.name, got, item.want)
		}
	}
}

func TestPricingClauseIsCoveredWhenItRestatesAReviewedTariff(t *testing.T) {
	description := "Servicii curățenie birou (2 intervenții/săptămână, consumabile incluse)"
	tariff := commercialvalidation.Rule{ID: "service-cleaning", Kind: commercialvalidation.RuleUnitRate, Narrative: description + " · 850.00 RON / lună", Applicability: commercialvalidation.Applicability{ServiceID: "service-cleaning", Aliases: []string{description}}, Currency: "RON", Expression: &commercialvalidation.Expression{Op: "literal", Value: "850.00"}}
	rules := []commercialvalidation.Rule{tariff}
	narrative := description + ": 850,00 lei / lună"
	snippet := "Servicii curățenie birou (2 intervenții/săptămână,\nconsumabile incluse)\tlună\t850,00 lei\t1 lună"
	executable := &commercialvalidation.Rule{ID: "extracted-4", Kind: commercialvalidation.RuleUnitRate, Narrative: narrative, Expression: &commercialvalidation.Expression{Op: "literal", Value: "850.00"}}
	if service, covered := restatedTariff("UNIT_RATE", narrative, snippet, executable, rules); !covered || service.ID != tariff.ID {
		t.Fatalf("an executable clause restating the tariff is covered: %+v %v", service, covered)
	}
	if _, covered := restatedTariff("UNIT_RATE", narrative, snippet, nil, rules); !covered {
		t.Fatal("a narrative-only clause naming the service and citing its price is covered")
	}
	if _, covered := restatedTariff("UNIT_RATE", narrative, "Servicii curățenie birou (2 intervenții/săptămână, consumabile incluse) 900,00 lei", nil, rules); covered {
		t.Fatal("a clause citing another price is not covered")
	}
	if _, covered := restatedTariff("DISCOUNT", narrative, snippet, nil, rules); covered {
		t.Fatal("only fixed and unit prices can restate a tariff")
	}
	if _, covered := restatedTariff("UNIT_RATE", narrative, snippet, executable, nil); covered {
		t.Fatal("without an active tariff nothing is restated")
	}
}

func TestServiceStatesTellWhichTariffsInvoicesAreCheckedAgainstAndWhyNot(t *testing.T) {
	text := func(value string) *string { return &value }
	term := func(description, price, unit, model string) contractingestion.ReviewedServiceTerm {
		return contractingestion.ReviewedServiceTerm{ServiceDescription: description, PricingModel: model, UnitPrice: price, Currency: "RON", Unit: unit, BillingFrequency: "MONTHLY"}
	}
	proposed := func(price, snippet string) contractingestion.ProposedServiceTerm {
		return contractingestion.ProposedServiceTerm{UnitPrice: contractingestion.Field{Value: text(price), Evidence: contractingestion.Evidence{Snippet: snippet}}, Currency: contractingestion.Field{Value: text("RON")}}
	}
	terms := []contractingestion.ReviewedServiceTerm{
		term("Mentenanță IT — abonament lunar", "1800.00", "lună", "FIXED_FEE"),
		term("Hosting cloud — pachet Business 2 VM", "650.00", "lună", "FIXED_FEE"),
		term("Intervenții suplimentare", "175.00", "oră", "UNIT_RATE"),
	}
	source := []contractingestion.ProposedServiceTerm{proposed("1800.00", "1.800,00 lei"), proposed("650.00", "650,00 lei"), proposed("150.00", "150,00 lei")}
	hosting := stableID("service", "contract-1:1")
	snapshot := []commercialvalidation.Rule{{ID: hosting, Kind: commercialvalidation.RuleFixedPrice}, {ID: "contract-reference", Kind: commercialvalidation.RuleContractReference}}

	states := serviceStates("contract-1", "document-1", terms, source, snapshot)
	if len(states) != 3 {
		t.Fatalf("one state per confirmed service expected: %+v", states)
	}
	if states[0].Active || states[0].SkipReason != "" || states[0].RuleID != stableID("service", "contract-1:0") {
		t.Errorf("a source-backed tariff not yet in the snapshot is activatable: %+v", states[0])
	}
	if !states[1].Active || states[1].RuleID != hosting {
		t.Errorf("a tariff in the snapshot is enforced: %+v", states[1])
	}
	if states[2].Active || states[2].SkipReason != commercialvalidation.SkipPriceChangedFromSource {
		t.Errorf("a tariff changed from the source says why it is not enforced: %+v", states[2])
	}

	clausePrice := append(snapshot, commercialvalidation.Rule{ID: "extracted-price", Kind: commercialvalidation.RuleFixedPrice})
	if state := serviceStates("contract-1", "document-1", terms, source, clausePrice)[0]; state.Active || state.SkipReason != commercialvalidation.SkipPricedByClauses {
		t.Errorf("tariffs are not added next to prices confirmed from clauses: %+v", state)
	}
}

func TestSameRulesTreatsEmptyRuleSetsAsEqual(t *testing.T) {
	if !sameRules(nil, []commercialvalidation.Rule{}) {
		t.Fatal("an empty snapshot and an unchanged empty rule set must not create a new snapshot")
	}
	if sameRules([]commercialvalidation.Rule{{ID: "a"}}, []commercialvalidation.Rule{{ID: "b"}}) {
		t.Fatal("different rules are not the same")
	}
}

func TestFixedContractQuantityIsSeededOnlyWhenKeptFromTheSource(t *testing.T) {
	text := func(value string) *string { return &value }
	unitRate := stableID("service", "contract-1:0")
	rules := []commercialvalidation.Rule{{ID: unitRate, Kind: commercialvalidation.RuleUnitRate}, {ID: stableID("service", "contract-1:1"), Kind: commercialvalidation.RuleFixedPrice}}
	terms := func() []contractingestion.ReviewedServiceTerm {
		return []contractingestion.ReviewedServiceTerm{{QuantitySource: "CONTRACT_FIXED_QUANTITY", QuantityValue: "1"}, {QuantitySource: "CONTRACT_FIXED_QUANTITY", QuantityValue: "1"}}
	}
	proposed := []contractingestion.ProposedServiceTerm{{QuantityValue: contractingestion.Field{Value: text("1.00")}}, {QuantityValue: contractingestion.Field{Value: text("1")}}}
	seeds := fixedQuantitySeeds("contract-1", "CWF-0231", rules, terms(), proposed)
	if len(seeds) != 1 || seeds[0].Name != "unit_quantity_"+unitRate || seeds[0].Value != "1" || !strings.Contains(seeds[0].Reference, "CWF-0231") {
		t.Fatalf("only the unit-rate tariff's fixed quantity is seeded: %+v", seeds)
	}
	changed := terms()
	changed[0].QuantityValue = "2"
	if seeds := fixedQuantitySeeds("contract-1", "CWF-0231", rules, changed, proposed); len(seeds) != 0 {
		t.Fatalf("a quantity changed at review is left for the reviewer to source: %+v", seeds)
	}
	reported := terms()
	reported[0].QuantitySource = "INVOICE_REPORTED_QUANTITY"
	if seeds := fixedQuantitySeeds("contract-1", "CWF-0231", rules, reported, proposed); len(seeds) != 0 {
		t.Fatalf("a quantity the contract does not fix is not seeded: %+v", seeds)
	}
}
