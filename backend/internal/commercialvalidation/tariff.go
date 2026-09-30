package commercialvalidation

import (
	"math/big"
	"strings"
	"unicode"
)

// A contract's tariff table can reach the rule set twice: as the reviewed
// service tariff (a rule whose id starts with ServiceTariffPrefix) and as a
// pricing clause the extraction read from the same table row. Both describe
// one service, so the clause adds no check of its own. The comparison is
// between two rules of the same contract, never against an invoiced amount.
const ServiceTariffPrefix = "service-"

// IsServiceTariff reports whether rule was built from a reviewed service
// tariff.
func IsServiceTariff(rule Rule) bool {
	return strings.HasPrefix(rule.ID, ServiceTariffPrefix)
}

// NamesService reports whether text states the service label word for word,
// ignoring case, diacritics, punctuation and line breaks.
func NamesService(text, label string) bool {
	needle := foldedWords(label)
	if needle == "" {
		return false
	}
	return strings.Contains(" "+foldedWords(text)+" ", " "+needle+" ")
}

// RestatedServiceTariff returns the reviewed service tariff among rules that
// clause only restates: both are a fixed or unit price, the clause's plain
// literal equals the tariff, its currency (when stated) is the tariff's, it
// has no applicability of its own and its text names the service.
func RestatedServiceTariff(clause Rule, rules []Rule) (Rule, bool) {
	if IsServiceTariff(clause) || !plainPriceKind(clause.Kind) || !emptyApplicability(clause.Applicability) {
		return Rule{}, false
	}
	price, ok := literalPrice(clause.Expression)
	if !ok {
		return Rule{}, false
	}
	text := clause.Narrative
	for _, evidence := range clause.Evidence {
		text += "\n" + evidence.Snippet
	}
	for _, service := range rules {
		if !IsServiceTariff(service) || !plainPriceKind(service.Kind) {
			continue
		}
		if clause.Currency != "" && service.Currency != "" && !strings.EqualFold(clause.Currency, service.Currency) {
			continue
		}
		if tariff, valid := literalPrice(service.Expression); !valid || tariff.Cmp(price) != 0 {
			continue
		}
		if NamesService(text, ServiceLabel(service)) {
			return service, true
		}
	}
	return Rule{}, false
}

// withoutRestatedTariffs drops the clauses that only restate a reviewed
// service tariff, so one service is offered once.
func withoutRestatedTariffs(rules []Rule) []Rule {
	kept := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if _, restated := RestatedServiceTariff(rule, rules); !restated {
			kept = append(kept, rule)
		}
	}
	return kept
}

func plainPriceKind(kind RuleKind) bool {
	return kind == RuleFixedPrice || kind == RuleUnitRate
}

func emptyApplicability(applicability Applicability) bool {
	return applicability.ServiceID == "" && len(applicability.SKUs) == 0 && len(applicability.Aliases) == 0 && len(applicability.DocumentTypes) == 0 &&
		applicability.BillingFrequency == "" && applicability.ContractYear == nil && applicability.Tranche == nil
}

func literalPrice(expression *Expression) (*big.Rat, bool) {
	if expression == nil || expression.Op != "literal" || len(expression.Args) > 0 || len(expression.Tiers) > 0 {
		return nil, false
	}
	return rat(strings.TrimSpace(expression.Value))
}

func foldedWords(text string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(foldDiacritics(text)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}
