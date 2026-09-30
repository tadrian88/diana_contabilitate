package commercialvalidation

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"diana-contabilitate/backend/internal/invoicing"
)

// Service suggestions rank a contract's priced services for one uncovered
// invoice line using only the wording and the unit of measure. The tariff is
// deliberately never a signal: choosing the service whose price equals the
// invoiced amount would make the price check that follows the mapping
// circular. A suggestion only pre-selects an option for a reviewer; nothing
// here confirms a mapping.
const (
	suggestionMinScore  = 0.6
	suggestionMinMargin = 0.25
	unitCompatibleBonus = 0.1
	unitMismatchPenalty = 0.3
)

var suggestionStopwords = map[string]bool{
	"a": true, "al": true, "ale": true, "cf": true, "ctr": true, "ron": true, "lei": true, "eur": true, "conform": true, "contract": true, "contractului": true, "cu": true,
	"de": true, "din": true, "factura": true, "facturii": true, "in": true, "la": true, "luna": true, "lunar": true,
	"lunara": true, "lunare": true, "nr": true, "pe": true, "pentru": true, "prestari": true, "prestare": true,
	"privind": true, "servicii": true, "serviciu": true, "serviciul": true, "si": true, "sau": true,
	"ianuarie": true, "februarie": true, "martie": true, "aprilie": true, "mai": true, "iunie": true, "iulie": true,
	"august": true, "septembrie": true, "octombrie": true, "noiembrie": true, "decembrie": true,
	"and": true, "for": true, "of": true, "the": true, "services": true, "service": true,
}

// Canonical units: UN/ECE Recommendation 20 codes used by e-Factura plus the
// Romanian words contracts use for the same unit.
var canonicalUnits = map[string]string{
	"MON": "MONTH", "LUNA": "MONTH", "LUNI": "MONTH", "LUNAR": "MONTH",
	"HUR": "HOUR", "ORA": "HOUR", "ORE": "HOUR", "H": "HOUR",
	"DAY": "DAY", "ZI": "DAY", "ZILE": "DAY",
	"ANN": "YEAR", "AN": "YEAR", "ANI": "YEAR", "ANUAL": "YEAR",
	"C62": "PIECE", "H87": "PIECE", "EA": "PIECE", "BUC": "PIECE", "BUCATA": "PIECE", "BUCATI": "PIECE",
	"KGM": "KILOGRAM", "KG": "KILOGRAM",
}

// SuggestServices returns every priced service as a candidate for line,
// ordered by descending score, and marks at most one as suggested when the
// best match is both strong and clearly ahead of the runner-up. A clause that
// only restates a reviewed service tariff is not offered next to it.
func SuggestServices(line invoicing.Line, rules []Rule) []ServiceCandidate {
	rules = withoutRestatedTariffs(rules)
	lineWords := suggestionWords(lineName(line))
	if len(lineWords) == 0 {
		lineWords = suggestionWords(lineDescription(line))
	}
	lineUnit := canonicalUnit(line.Unit)
	candidates := make([]ServiceCandidate, 0, len(rules))
	for _, rule := range rules {
		label := ServiceLabel(rule)
		unit := serviceUnit(rule)
		candidate := ServiceCandidate{RuleID: rule.ID, Label: label, PricingKind: rule.Kind, Unit: unit, BillingFrequency: rule.Applicability.BillingFrequency, Evidence: rule.Evidence, LineWordCount: len(lineWords), LineUnit: line.Unit, UnitMatch: UnitUnknown}
		serviceWords := suggestionWords(label)
		shared := sharedWords(lineWords, serviceWords)
		score := 0.0
		if len(lineWords) > 0 && len(serviceWords) > 0 {
			score = 0.75*float64(len(shared))/float64(len(lineWords)) + 0.25*float64(len(shared))/float64(len(serviceWords))
		}
		if serviceCanonical := canonicalUnit(unit); lineUnit != "" && serviceCanonical != "" {
			if lineUnit == serviceCanonical {
				candidate.UnitMatch = UnitCompatible
				if score > 0 {
					score += unitCompatibleBonus
				}
			} else {
				candidate.UnitMatch = UnitIncompatible
				score -= unitMismatchPenalty
			}
		}
		candidate.SharedWords = shared
		candidate.Score = math.Round(math.Max(0, math.Min(1, score))*100) / 100
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	if len(candidates) > 0 && candidates[0].Score >= suggestionMinScore && candidates[0].UnitMatch != UnitIncompatible {
		if len(candidates) == 1 || candidates[0].Score-candidates[1].Score >= suggestionMinMargin {
			candidates[0].Suggested = true
		}
	}
	return candidates
}

func lineName(line invoicing.Line) string {
	if line.SourceFacts != nil && strings.TrimSpace(line.SourceFacts.ItemName) != "" {
		return line.SourceFacts.ItemName
	}
	return line.Description
}

func lineDescription(line invoicing.Line) string {
	if line.SourceFacts != nil && strings.TrimSpace(line.SourceFacts.ItemDescription) != "" {
		return line.SourceFacts.ItemDescription
	}
	if line.AdditionalInfo != nil {
		return *line.AdditionalInfo
	}
	return ""
}

// ServiceLabel is the contractual wording of the service without its price:
// the confirmed service description, else the narrative before the
// " · <price>" suffix that reviewed service prices carry.
func ServiceLabel(rule Rule) string {
	if len(rule.Applicability.Aliases) > 0 && strings.TrimSpace(rule.Applicability.Aliases[0]) != "" {
		return rule.Applicability.Aliases[0]
	}
	if index := strings.Index(rule.Narrative, " · "); index > 0 {
		return rule.Narrative[:index]
	}
	return rule.Narrative
}

// serviceUnit prefers the explicit unit and falls back to the " / <unit>"
// suffix that unit-rate narratives carry.
func serviceUnit(rule Rule) string {
	if strings.TrimSpace(rule.Unit) != "" {
		return strings.TrimSpace(rule.Unit)
	}
	if rule.Kind == RuleUnitRate {
		if index := strings.LastIndex(rule.Narrative, " / "); index >= 0 {
			return strings.TrimSpace(rule.Narrative[index+3:])
		}
	}
	return ""
}

func canonicalUnit(raw string) string {
	return canonicalUnits[strings.ToUpper(foldDiacritics(strings.TrimSpace(raw)))]
}

func suggestionWords(text string) []string {
	seen := map[string]bool{}
	words := []string{}
	for _, word := range strings.FieldsFunc(strings.ToLower(foldDiacritics(text)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if suggestionStopwords[word] || isAmountLike(word) || seen[word] {
			continue
		}
		seen[word] = true
		words = append(words, word)
	}
	return words
}

func sharedWords(line, service []string) []string {
	available := map[string]bool{}
	for _, word := range service {
		available[word] = true
	}
	shared := []string{}
	for _, word := range line {
		if available[word] {
			shared = append(shared, word)
		}
	}
	return shared
}

// isAmountLike drops years, prices and their decimals ("2026", "650", "00")
// so that neither a date nor an amount can make two wordings look alike.
// Short model numbers such as the "2" in "Business 2 VM" remain words.
func isAmountLike(word string) bool {
	for _, r := range word {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(word) >= 3 || word == "00"
}

var diacriticFolds = strings.NewReplacer(
	"ă", "a", "â", "a", "î", "i", "ș", "s", "ş", "s", "ț", "t", "ţ", "t",
	"Ă", "A", "Â", "A", "Î", "I", "Ș", "S", "Ş", "S", "Ț", "T", "Ţ", "T",
)

func foldDiacritics(value string) string {
	return diacriticFolds.Replace(value)
}
