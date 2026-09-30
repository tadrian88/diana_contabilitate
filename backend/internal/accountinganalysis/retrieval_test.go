package accountinganalysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	salesPlan := RetrievalPlan(outgoing)
	salesKeys := strings.Join(salesPlan[0].CitationKeys, "|")
	if salesPlan[0].Dimension != "ACCOUNT" || strings.Contains(salesKeys, "contul 628") {
		t.Fatalf("sales must not receive purchase account functions: %#v", salesPlan[0])
	}
	for _, account := range OutgoingAccountFunctions {
		if !strings.Contains(salesKeys, AccountFunctionCitationKey(account)) {
			t.Fatalf("missing sales account %s in %s", account, salesKeys)
		}
	}
	// With every dimension unresolved, the account functions still come first,
	// so the long Fiscal Code articles cannot crowd them out of the budget.
	all := input
	all.Lines = []Line{{ID: "l1", Description: "Promovare online", UnresolvedDimensions: []string{"VAT_TREATMENT", "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT", "ACCOUNT"}}}
	full := RetrievalPlan(all)
	if len(full) != 8 || full[0].Dimension != "ACCOUNT" || len(full[0].CitationKeys) == 0 {
		t.Fatalf("account functions must lead the plan: %#v", full)
	}
	for index, want := range map[int][]string{1: {"ART. 282"}, 2: {"ART. 297", "ART. 298", "ART. 299"}, 3: {"ART. 25"}} {
		if strings.Join(full[index].CitationKeys, "|") != strings.Join(want, "|") || full[index].Kinds[0] != "LAW" || len(full[index].Terms) != 0 {
			t.Fatalf("fiscal articles %d = %#v, want %v", index, full[index], want)
		}
	}
	for _, planned := range full[4:] {
		if len(planned.CitationKeys) != 0 || len(planned.Terms) == 0 {
			t.Fatalf("lexical lookups come after the fixed evidence: %#v", planned)
		}
	}
	functions := make([]legislation.Fragment, len(IncomingAccountFunctions))
	for index := range functions {
		functions[index] = legislation.Fragment{ID: fmt.Sprintf("function-%d", index), Text: strings.Repeat("x", 540)}
	}
	fiscal := []legislation.Fragment{}
	// Observed sizes: art. 282, 297, 298, 299 and art. 25 (above the former 20 000 cap).
	for index, size := range []int{10853, 3826, 2596, 2939, 26147} {
		fiscal = append(fiscal, legislation.Fragment{ID: fmt.Sprintf("law-%d", index), Text: strings.Repeat("x", size)})
	}
	if merged := MergeFragments([][]legislation.Fragment{functions, fiscal}); len(merged) != len(functions)+len(fiscal) {
		t.Fatalf("account functions and the fixed Fiscal Code articles must both fit: %d fragments", len(merged))
	}
	resolved := Input{IssueDate: "2026-06-01", Direction: Incoming, Lines: []Line{{ID: "l1", UnresolvedDimensions: []string{"VAT_DEDUCTIBILITY"}}}}
	for _, planned := range RetrievalPlan(resolved) {
		if planned.Dimension == "ACCOUNT" {
			t.Fatalf("a resolved account must not retrieve account functions: %#v", planned)
		}
		if planned.Dimension != "VAT_DEDUCTIBILITY" {
			t.Fatalf("only the unresolved dimension may retrieve evidence: %#v", planned)
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
	if !AccountRelevantForDirection("704", Outgoing) || !AccountRelevantForDirection("7041", Outgoing) || !AccountRelevantForDirection("167", Outgoing) || !AccountRelevantForDirection("419", Outgoing) || !AccountRelevantForDirection("472", Outgoing) || AccountRelevantForDirection("4111", Outgoing) || AccountRelevantForDirection("628", Outgoing) || AccountRelevantForDirection("", Incoming) {
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

func TestOutgoingVATRetrievalUsesChargeabilityArticles(t *testing.T) {
	input := Input{Direction: Outgoing, Lines: []Line{{ID: "l1", Description: "chirie lunara birouri", UnresolvedDimensions: []string{"VAT_TREATMENT", "ACCOUNT"}}}}
	plan := RetrievalPlan(input)
	if len(plan) < 3 || plan[0].Dimension != "ACCOUNT" || plan[1].Dimension != "VAT_TREATMENT" {
		t.Fatalf("plan = %#v", plan)
	}
	if got, want := strings.Join(plan[1].CitationKeys, "|"), strings.Join(OutgoingFiscalArticles["VAT_TREATMENT"], "|"); got != want {
		t.Fatalf("VAT articles = %s, want %s", got, want)
	}
	lexical := plan[len(plan)-1]
	if lexical.Dimension != "ACCOUNT" || lexical.Terms[0] != "venituri din" {
		t.Fatalf("sales lexical terms = %#v", lexical.Terms)
	}
}

// TestRetrievalKeysExistInSnapshots guards every exact citation key against
// the repository legislation snapshots, so a key typo cannot silently send
// no evidence.
func TestRetrievalKeysExistInSnapshots(t *testing.T) {
	keys := map[string]bool{}
	files, _ := filepath.Glob("../../legislation/source-snapshots/*.json")
	if len(files) == 0 {
		t.Skip("legislation snapshots are not available")
	}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if key, ok := typed["citationKey"].(string); ok {
				keys[key] = true
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err = json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		walk(document)
	}
	want := []string{}
	for _, account := range append(append([]string{}, IncomingAccountFunctions...), OutgoingAccountFunctions...) {
		want = append(want, AccountFunctionCitationKey(account))
	}
	for _, articles := range []map[string][]string{IncomingFiscalArticles, OutgoingFiscalArticles} {
		for _, list := range articles {
			want = append(want, list...)
		}
	}
	for _, key := range want {
		if !keys[key] {
			t.Errorf("citation key %q is not in the legislation snapshots", key)
		}
	}
}
