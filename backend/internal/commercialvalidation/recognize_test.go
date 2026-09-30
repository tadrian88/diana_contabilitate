package commercialvalidation

import "testing"

func narrativeRule(kind RuleKind, narrative, snippet string) Rule {
	return Rule{ID: "clause", Kind: kind, Narrative: narrative, Evidence: []Evidence{{Snippet: snippet}}}
}

func TestRecognizeSourceTextPaymentDue(t *testing.T) {
	cases := []struct {
		name, narrative, snippet string
		days                     string
		basis                    DateBasis
		ok                       bool
	}{
		{"issue date", "5.1. Plata se efectuează prin transfer bancar în contul PRESTATOR indicat mai sus, în termen de 10 zile calendaristice de la data emiterii facturii.", "în termen de 10 zile calendaristice de la data emiterii facturii", "10", DateInvoiceIssue, true},
		{"receipt", "Plata în 30 de zile de la primirea facturii.", "Plata în 30 de zile de la primirea facturii.", "30", DateReceipt, true},
		{"parenthetical words", "Plata în 15 (cincisprezece) zile de la data emiterii.", "în 15 (cincisprezece) zile de la data emiterii", "15", DateInvoiceIssue, true},
		{"working days", "Plata în 10 zile lucrătoare de la emiterea facturii.", "10 zile lucrătoare de la emiterea facturii", "", "", false},
		{"two terms", "Plata în 10 zile de la emitere, respectiv 30 zile pentru avans.", "10 zile de la emitere, respectiv 30 zile pentru avans", "", "", false},
		{"narrative contradicts", "Plata în 15 zile de la emitere.", "în 10 zile de la data emiterii", "", "", false},
		{"unclear basis", "Plata în 10 zile de la recepția serviciilor.", "în 10 zile de la recepția serviciilor", "", "", false},
		{"end of month", "Plata în 10 zile de la sfârșitul lunii.", "în 10 zile de la sfârșitul lunii", "", "", false},
		{"no basis", "Plata în 10 zile.", "Plata în 10 zile.", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := RecognizeSourceTextRule(narrativeRule(RulePaymentDue, tc.narrative, tc.snippet), tc.snippet)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v (%+v)", ok, tc.ok, rule)
			}
			if !ok {
				return
			}
			if rule.Expression.Op != "literal" || rule.Expression.Value != tc.days || rule.DateBasis != tc.basis || !rule.Blocking || rule.Origin != RuleOriginSourceText {
				t.Fatalf("unexpected rule %+v / %+v", rule, rule.Expression)
			}
		})
	}
}

func TestRecognizeSourceTextKeepsProposedDateBasis(t *testing.T) {
	rule := narrativeRule(RulePaymentDue, "Plata în 10 zile de la data emiterii facturii.", "10 zile de la data emiterii facturii")
	rule.DateBasis = DateReceipt
	if _, ok := RecognizeSourceTextRule(rule, rule.Evidence[0].Snippet); ok {
		t.Fatal("recognition must not replace an extracted date basis")
	}
}

func TestRecognizeSourceTextVAT(t *testing.T) {
	legal := "Prețurile nu includ TVA. TVA se aplică în cota legală în vigoare (21% la data semnării)."
	rule, ok := RecognizeSourceTextRule(narrativeRule(RuleVAT, "4.3. "+legal, legal), legal)
	if !ok || rule.Expression.Op != "variable" || rule.Expression.Variable != ApplicableVATVariable || len(rule.RequiredVariables) != 1 || rule.DateBasis != DateInvoiceIssue {
		t.Fatalf("legal VAT clause must follow the applicable rate: ok=%v %+v", ok, rule)
	}
	fixed := "La prețuri se adaugă TVA 19%."
	rule, ok = RecognizeSourceTextRule(narrativeRule(RuleVAT, fixed, fixed), fixed)
	if !ok || rule.Expression.Op != "literal" || rule.Expression.Value != "19" {
		t.Fatalf("explicit VAT rate must be literal: ok=%v %+v", ok, rule)
	}
	for _, text := range []string{
		"TVA 9% pentru cazare și 19% pentru restul serviciilor.",
		"Serviciile sunt scutite de TVA.",
		"Se aplică taxarea inversă pentru TVA.",
		"Prestatorul nu este plătitor de TVA.",
		"Prețurile includ TVA.",
	} {
		if rule, ok := RecognizeSourceTextRule(narrativeRule(RuleVAT, text, text), text); ok {
			t.Fatalf("%q must stay in review, got %+v", text, rule)
		}
	}
}
