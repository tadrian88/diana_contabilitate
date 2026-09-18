// Package fiscalidentity owns fiscal identifier comparison semantics.
// It deliberately separates entity identity from VAT-registration status.
package fiscalidentity

import (
	"regexp"
	"strings"
	"unicode"
)

var romanianDigits = regexp.MustCompile(`^[1-9][0-9]{1,9}$`)

// Romanian normalizes a Romanian CUI for identity comparison. The optional RO
// VAT prefix and whitespace are ignored; no VAT status is inferred.
func Romanian(raw string) (string, bool) {
	value := strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(raw)))
	value = strings.TrimPrefix(value, "RO")
	if !romanianDigits.MatchString(value) {
		return "", false
	}
	return value, true
}

// ForComparison applies Romanian semantics only when the country is Romanian,
// or when the value itself is unambiguously a numeric Romanian CUI form.
// Arbitrary foreign tax identifiers keep punctuation and prefixes significant.
func ForComparison(raw, country string) string {
	if strings.EqualFold(strings.TrimSpace(country), "RO") {
		if value, ok := Romanian(raw); ok {
			return value
		}
		return ""
	}
	if value, ok := Romanian(raw); ok {
		return value
	}
	return strings.ToUpper(strings.Join(strings.Fields(raw), " "))
}

func SameRomanian(left, right string) bool {
	l, lok := Romanian(left)
	r, rok := Romanian(right)
	return lok && rok && l == r
}

// Same compares Romanian forms canonically and otherwise falls back to the
// conservative generic representation used for legacy/foreign identifiers.
func Same(left, right string) bool {
	if l, lok := Romanian(left); lok {
		if r, rok := Romanian(right); rok {
			return l == r
		}
	}
	l, r := ForComparison(left, ""), ForComparison(right, "")
	return l != "" && l == r
}
