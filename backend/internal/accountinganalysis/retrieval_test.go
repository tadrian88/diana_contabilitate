package accountinganalysis

import (
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

func TestRetrievalPlanTargetsOnlyUnresolvedDimensions(t *testing.T) {
	input := Input{IssueDate: "2026-09-04", Lines: []Line{{ID: "l1", Description: "PRESTARI SERVICII CF. CTR.", UnresolvedDimensions: []string{"ACCOUNT", "VAT_DEDUCTIBILITY"}}}}
	plan := RetrievalPlan(input)
	if len(plan) != 2 || plan[0].Dimension != "VAT_DEDUCTIBILITY" || plan[1].Dimension != "ACCOUNT" {
		t.Fatalf("unexpected plan %#v", plan)
	}
	if plan[0].Kinds[0] != "LAW" || plan[1].Kinds[0] != "ORDER" {
		t.Fatal("dimension must target its source kind")
	}
	joined := strings.Join(plan[1].Terms, "|")
	if !strings.Contains(joined, "prestari") || !strings.Contains(joined, "servicii") || strings.Contains(joined, "|cf|") {
		t.Fatal(joined)
	}
	query := plan[0].Query(input, true)
	if query.ApplicableDate != "2026-09-04" || !query.AllowTestOnly || query.Limit != 3 {
		t.Fatalf("%#v", query)
	}
	if len(RetrievalPlan(Input{Lines: []Line{{ID: "l1"}}})) != 0 {
		t.Fatal("resolved invoice must not retrieve")
	}
}

func TestIncomingAccountRetrievalAlwaysIncludesOMFPAccountFunctions(t *testing.T) {
	input := Input{IssueDate: "2026-06-01", Direction: Incoming, Lines: []Line{{ID: "l1", Description: "Promovare online", UnresolvedDimensions: []string{"ACCOUNT"}}}}
	plan := RetrievalPlan(input)
	if len(plan) != 2 || plan[0].Dimension != "ACCOUNT" || plan[1].Dimension != "ACCOUNT" || len(plan[0].Terms) != 0 || len(plan[1].Terms) == 0 {
		t.Fatalf("account functions must come first, then the lexical lookup: %#v", plan)
	}
	keys := strings.Join(plan[0].CitationKeys, "|")
	for _, account := range []string{"622", "623", "628", "602", "611", "231"} {
		if !strings.Contains(keys, "OMFP 1802/2014 contul "+account) {
			t.Fatalf("missing account %s in %s", account, keys)
		}
	}
	query := plan[0].Query(input, true)
	if query.Kinds[0] != "ORDER" || query.Limit != len(IncomingAccountFunctions) || len(query.CitationKeys) != len(IncomingAccountFunctions) {
		t.Fatalf("%#v", query)
	}

	outgoing := input
	outgoing.Direction = Outgoing
	for _, planned := range RetrievalPlan(outgoing) {
		if len(planned.CitationKeys) > 0 {
			t.Fatalf("sales must not receive purchase account functions: %#v", planned)
		}
	}
	resolved := Input{IssueDate: "2026-06-01", Direction: Incoming, Lines: []Line{{ID: "l1", UnresolvedDimensions: []string{"VAT_DEDUCTIBILITY"}}}}
	for _, planned := range RetrievalPlan(resolved) {
		if len(planned.CitationKeys) > 0 {
			t.Fatalf("a resolved account must not retrieve account functions: %#v", planned)
		}
	}
}

func TestMergeFragmentsDeduplicatesAndBoundsSize(t *testing.T) {
	fragment := func(id string, size int) legislation.Fragment {
		return legislation.Fragment{ID: id, Text: strings.Repeat("x", size)}
	}
	small := fragment("a", 100)
	huge := fragment("huge", RetrievalMaxFragmentChars+1)
	mid := func(id string) legislation.Fragment { return fragment(id, 19000) }
	merged := MergeFragments([][]legislation.Fragment{{small, huge}, {small, mid("m1"), mid("m2")}, {mid("m3"), mid("m4"), fragment("c", 10)}})
	ids := []string{}
	for _, item := range merged {
		ids = append(ids, item.ID)
	}
	// huge exceeds the per-fragment cap; m4 would exceed the total budget; the
	// duplicate small is kept once; the later small fragment still fits.
	if strings.Join(ids, ",") != "a,m1,m2,m3,c" {
		t.Fatal(ids)
	}
}

func TestAccountRelevantForDirection(t *testing.T) {
	for code, want := range map[string]bool{"6224": true, "628": true, "2131": true, "371": true, "471": true, "4111": false, "401": false, "704": false, "5121": false} {
		if AccountRelevantForDirection(code, Incoming) != want {
			t.Fatal("incoming", code)
		}
	}
	if !AccountRelevantForDirection("704", Outgoing) || AccountRelevantForDirection("628", Outgoing) || AccountRelevantForDirection("", Incoming) {
		t.Fatal("outgoing")
	}
}

// The prompt vocabulary must stay aligned with the deterministic validator.
func TestPromptVocabularyShapesValidate(t *testing.T) {
	rate := money.MustParse("21")
	half := money.MustParse("50")
	hundred := money.MustParse("100")
	valid := map[string][]accounting.Value{
		"ACCOUNT":               {{Kind: "ACCOUNT", Account: "6224"}},
		"VAT_TREATMENT":         {{Kind: "ORDINARY", Timing: "IMMEDIATE", SourceCategory: "S", SourceRate: &rate}, {Kind: "SPECIAL_UNSUPPORTED", Timing: "UNSUPPORTED", SourceCategory: "S", SourceRate: &rate, Reason: "r"}},
		"VAT_DEDUCTIBILITY":     {{Kind: "FULL"}, {Kind: "LIMITED", Percentage: &half, Basis: "b"}, {Kind: "NONE", Reason: "r"}, {Kind: "NOT_APPLICABLE", Reason: "r"}},
		"EXPENSE_TAX_TREATMENT": {{Kind: "FULLY_DEDUCTIBLE"}, {Kind: "LIMITED", Percentage: &half, Basis: "b"}, {Kind: "PERIOD_LIMIT_CATEGORY", Category: "c", Basis: "b"}, {Kind: "NONDEDUCTIBLE", Reason: "r"}, {Kind: "NOT_APPLICABLE", Reason: "r"}},
	}
	kinds := map[string]bool{}
	for dimension, values := range valid {
		for _, value := range values {
			if err := value.Validate(dimension); err != nil {
				t.Fatal(dimension, value.Kind, err)
			}
			kinds[value.Kind] = true
		}
	}
	schemaKinds := proposalSchema()["properties"].(map[string]any)["lines"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["decisions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["proposedValue"].(map[string]any)["properties"].(map[string]any)["kind"].(map[string]any)["enum"].([]string)
	if len(schemaKinds) != len(kinds) {
		t.Fatalf("schema kinds %v differ from vocabulary %v", schemaKinds, kinds)
	}
	for _, kind := range schemaKinds {
		if !kinds[kind] {
			t.Fatal("schema kind without vocabulary example", kind)
		}
	}
	for _, invalid := range []accounting.Value{{Kind: "FULL", Percentage: &hundred}, {Kind: "ORDINARY", Timing: "IMMEDIATE", Category: "S", SourceRate: &rate}} {
		dimension := "VAT_DEDUCTIBILITY"
		if invalid.Kind == "ORDINARY" {
			dimension = "VAT_TREATMENT"
		}
		if invalid.Validate(dimension) == nil {
			t.Fatal("observed invalid provider shape must stay invalid", invalid)
		}
	}
	if !strings.Contains(analysisPrompt, "never send \"percentage\" with FULL") || !strings.Contains(analysisPrompt, "Never put the tax category in \"category\"") {
		t.Fatal("prompt must document the observed failure modes")
	}
}
