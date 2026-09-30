package accountinganalysis

import (
	"strings"
	"unicode/utf8"

	"diana-contabilitate/backend/internal/legislation"
)

// Retrieval bounds keep the provider envelope small enough for a single,
// timely call while still giving every unresolved dimension citable evidence.
const (
	RetrievalMaxFragmentChars = 20000
	RetrievalMaxTotalChars    = 60000
)

// RetrievalQuery is one dimension-focused lexical lookup. Terms are OR-ed by
// the store; each term is a plain phrase whose words must all appear. Terms
// avoid Romanian diacritics because the corpus mixes cedilla/comma forms.
type RetrievalQuery struct {
	Dimension    string
	Terms        []string
	Kinds        []string
	Limit        int
	CitationKeys []string
}

// IncomingAccountFunctions are the OMFP 1802/2014 synthetic accounts whose
// function („Cu ajutorul acestui cont se ține evidența…”) is always sent for a
// purchase whose account is unresolved: materials and utilities (60x), services
// (61x, 62x) and fixed assets in progress (231). A lexical lookup on the invoice
// words alone rarely ranks these short fragments among the first results, so
// the provider chose analytic accounts without the legal text that separates,
// for example, 622 (fees) from 628 (other third-party services). Larger chapters
// (assets, inventories) stay with the lexical lookup.
var IncomingAccountFunctions = []string{
	"601", "602", "603", "604", "605", "606", "607", "608", "609",
	"611", "612", "613", "614",
	"621", "622", "623", "624", "625", "626", "627", "628",
	"231", "471",
}

// AccountFunctionCitationKey is the corpus citation key of an OMFP account function.
func AccountFunctionCitationKey(account string) string {
	return "OMFP 1802/2014 contul " + account
}

// RetrievalPlan returns one query per unresolved dimension. The queries only
// select evidence; they never decide an accounting value.
func RetrievalPlan(input Input) []RetrievalQuery {
	needed := map[string]bool{}
	for _, line := range input.Lines {
		for _, dimension := range line.UnresolvedDimensions {
			needed[dimension] = true
		}
	}
	plan := []RetrievalQuery{}
	if needed["VAT_TREATMENT"] {
		plan = append(plan, RetrievalQuery{Dimension: "VAT_TREATMENT", Kinds: []string{"LAW"}, Limit: 3, Terms: []string{"cota standard", "faptul generator", "exigibilitatea"}})
	}
	if needed["VAT_DEDUCTIBILITY"] {
		plan = append(plan, RetrievalQuery{Dimension: "VAT_DEDUCTIBILITY", Kinds: []string{"LAW"}, Limit: 3, Terms: []string{"dreptul de deducere", "exercitarea dreptului de deducere", "deducere"}})
	}
	if needed["EXPENSE_TAX_TREATMENT"] {
		plan = append(plan, RetrievalQuery{Dimension: "EXPENSE_TAX_TREATMENT", Kinds: []string{"LAW"}, Limit: 3, Terms: []string{"cheltuieli deductibile", "cheltuieli nedeductibile", "deductibilitate limitata"}})
	}
	if needed["ACCOUNT"] && input.Direction == Incoming {
		keys := make([]string, 0, len(IncomingAccountFunctions))
		for _, account := range IncomingAccountFunctions {
			keys = append(keys, AccountFunctionCitationKey(account))
		}
		plan = append(plan, RetrievalQuery{Dimension: "ACCOUNT", Kinds: []string{"ORDER"}, Limit: len(keys), CitationKeys: keys})
	}
	if needed["ACCOUNT"] {
		terms := []string{"serviciile executate", "onorariile", "comisioanele"}
		seen := map[string]bool{}
		for _, term := range terms {
			seen[term] = true
		}
		add := func(value string) {
			if !utf8.ValidString(value) {
				return
			}
			for _, word := range strings.Fields(strings.ToLower(value)) {
				word = strings.Trim(word, ".,;:!?()[]{}\"'/-")
				if len([]rune(word)) < 4 || seen[word] || len(terms) >= 24 {
					continue
				}
				seen[word] = true
				terms = append(terms, word)
			}
		}
		for _, line := range input.Lines {
			add(line.Description)
			if line.Facts != nil {
				add(line.Facts.ItemName)
				add(line.Facts.ItemDescription)
			}
		}
		plan = append(plan, RetrievalQuery{Dimension: "ACCOUNT", Kinds: []string{"ORDER"}, Limit: 4, Terms: terms})
	}
	return plan
}

// Query converts a plan item to the store query for the invoice date.
func (q RetrievalQuery) Query(input Input, allowTestOnly bool) legislation.Query {
	return legislation.Query{Terms: q.Terms, ApplicableDate: input.IssueDate, Limit: q.Limit, AllowTestOnly: allowTestOnly, Kinds: q.Kinds, CitationKeys: q.CitationKeys}
}

// MergeFragments keeps plan order, removes duplicates and applies the size
// budget. Oversized fragments are skipped rather than truncated so that every
// citation still refers to the exact, hash-verified fragment text.
func MergeFragments(groups [][]legislation.Fragment) []legislation.Fragment {
	result := []legislation.Fragment{}
	seen := map[string]bool{}
	total := 0
	for _, group := range groups {
		for _, fragment := range group {
			size := len([]rune(fragment.Text))
			if seen[fragment.ID] || size > RetrievalMaxFragmentChars || total+size > RetrievalMaxTotalChars {
				continue
			}
			seen[fragment.ID] = true
			total += size
			result = append(result, fragment)
		}
	}
	return result
}

// AccountRelevantForDirection narrows the global catalog sent to the provider
// when the profile has no explicit account list: purchases go to fixed assets
// (class 2), inventories (class 3), expenses (class 6) or prepaid expenses
// (471); sales go to revenue (class 7). An explicit profile list is never
// narrowed, and deterministic validation still accepts any allowed account.
func AccountRelevantForDirection(code string, direction Direction) bool {
	if code == "" {
		return false
	}
	switch direction {
	case Incoming:
		return strings.ContainsRune("236", rune(code[0])) || strings.HasPrefix(code, "471")
	case Outgoing:
		return code[0] == '7'
	}
	return true
}
