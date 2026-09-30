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

// Kind is the type of a party identifier on an issued invoice (D-124).
type Kind string

const (
	KindCUI   Kind = "CUI"
	KindCNP   Kind = "CNP"
	KindOther Kind = "OTHER"
)

var cnpDigits = regexp.MustCompile(`^[0-9]{13}$`)

// cnpPlaceholder is the value e-Factura accepts for a natural-person buyer
// who did not give a CNP.
const cnpPlaceholder = "0000000000000"

// CNP validates a Romanian personal numeric code (13 digits, control digit
// with weights 279146358279) or the e-Factura placeholder of thirteen zeros.
func CNP(raw string) (string, bool) {
	value := compact(raw)
	if !cnpDigits.MatchString(value) {
		return "", false
	}
	if value == cnpPlaceholder {
		return value, true
	}
	if value[0] == '0' {
		return "", false
	}
	const weights = "279146358279"
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(value[i]-'0') * int(weights[i]-'0')
	}
	control := sum % 11
	if control == 10 {
		control = 1
	}
	if int(value[12]-'0') != control {
		return "", false
	}
	return value, true
}

// Classify identifies a customer: a CNP first (a CUI never has 13 digits),
// then a Romanian CUI with or without RO, otherwise the conservative generic
// form. The normalized value is empty when nothing usable remains.
func Classify(raw, country string) (Kind, string) {
	if value, ok := CNP(raw); ok {
		return KindCNP, value
	}
	if value, ok := Romanian(raw); ok {
		return KindCUI, value
	}
	return KindOther, strings.ToUpper(strings.Join(strings.Fields(raw), " "))
}

// Mask hides a CNP for presentation (first three digits + ***). Other
// identifiers are returned unchanged.
func Mask(kind Kind, value string) string {
	if kind != KindCNP {
		return value
	}
	return MaskIfCNP(value)
}

// MaskIfCNP masks the value when it is a CNP, whatever field it came from.
func MaskIfCNP(raw string) string {
	value, ok := CNP(raw)
	if !ok {
		return raw
	}
	return value[:3] + "***"
}

func compact(raw string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(raw)))
}
