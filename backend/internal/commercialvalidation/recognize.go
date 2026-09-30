package commercialvalidation

import (
	"regexp"
	"strings"
)

// RuleOriginSourceText marks a rule whose executable value was read
// deterministically from the cited clause text rather than proposed by AI.
const RuleOriginSourceText = "SOURCE_TEXT"

// ApplicableVATVariable names the legal VAT rate valid at invoice issue date.
const ApplicableVATVariable = "applicable_vat_rate"

var (
	sourceDaysPattern    = regexp.MustCompile(`\b(\d+)\s*(?:\([^)]{1,40}\)\s*)?(?:de\s+)?zile\b`)
	sourcePercentPattern = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*%`)
	sourceIssuePattern   = regexp.MustCompile(`\bemiter`)
)

// RecognizeSourceTextRule completes a narrative-only VAT or payment-term rule
// when the cited clause states exactly one reading. Any ambiguity (several
// values, working days, an unsupported or unclear date basis, exemptions)
// returns false so the clause stays in human review. The caller must still
// verify the result against the stored source like any reviewed rule.
func RecognizeSourceTextRule(rule Rule, snippet string) (Rule, bool) {
	if rule.Expression != nil || len(rule.RequiredVariables) > 0 || rule.Currency != "" {
		return Rule{}, false
	}
	source := strings.ToLower(snippet)
	narrative := strings.ToLower(rule.Narrative)
	var recognized Rule
	var ok bool
	switch rule.Kind {
	case RulePaymentDue:
		recognized, ok = recognizePaymentDue(rule, source, narrative)
	case RuleVAT:
		recognized, ok = recognizeVAT(rule, source, narrative)
	}
	if !ok || (rule.DateBasis != "" && rule.DateBasis != recognized.DateBasis) {
		return Rule{}, false
	}
	recognized.Blocking = true
	recognized.Origin = RuleOriginSourceText
	if ValidateRule(recognized) != nil {
		return Rule{}, false
	}
	return recognized, true
}

func recognizePaymentDue(rule Rule, source, narrative string) (Rule, bool) {
	if strings.Contains(source+narrative, "lucrătoare") || strings.Contains(source+narrative, "lucratoare") {
		return Rule{}, false // The engine counts calendar days only.
	}
	matches := sourceDaysPattern.FindAllStringSubmatchIndex(source, -1)
	if len(matches) == 0 {
		return Rule{}, false
	}
	days := source[matches[0][2]:matches[0][3]]
	for _, match := range matches[1:] {
		if source[match[2]:match[3]] != days {
			return Rule{}, false
		}
	}
	for _, match := range sourceDaysPattern.FindAllStringSubmatch(narrative, -1) {
		if match[1] != days {
			return Rule{}, false
		}
	}
	// Classify only the words after the term itself, up to the sentence end:
	// "în termen de 10 zile calendaristice de la data emiterii facturii".
	tail := source[matches[len(matches)-1][1]:]
	if end := strings.IndexAny(tail, ".;"); end >= 0 {
		tail = tail[:end]
	}
	for _, unclear := range []string{"recep", "livr", "prestăr", "prestar", "sfârșit", "sfarsit", "lun"} {
		if strings.Contains(tail, unclear) {
			return Rule{}, false
		}
	}
	var bases []DateBasis
	if sourceIssuePattern.MatchString(tail) {
		bases = append(bases, DateInvoiceIssue)
	}
	if strings.Contains(tail, "remiter") || strings.Contains(tail, "primir") || strings.Contains(tail, "transmiter") {
		bases = append(bases, DateReceipt)
	}
	if strings.Contains(tail, "accept") {
		bases = append(bases, DateAcceptance)
	}
	if len(bases) != 1 {
		return Rule{}, false
	}
	rule.DateBasis = bases[0]
	rule.Expression = &Expression{Op: "literal", Value: days, Scale: 2}
	return rule, true
}

func recognizeVAT(rule Rule, source, narrative string) (Rule, bool) {
	combined := source + " " + narrative
	if !strings.Contains(source, "tva") {
		return Rule{}, false
	}
	for _, excluded := range []string{"scutit", "invers", "neimpozab", "plătitor", "platitor", "penalit", "dobând", "doband"} {
		if strings.Contains(combined, excluded) {
			return Rule{}, false
		}
	}
	rule.DateBasis = DateInvoiceIssue
	// "cota legală în vigoare (21% la data semnării)" follows the law, not the
	// informative percentage, so it resolves to the rate valid per invoice.
	if strings.Contains(source, "legal") || strings.Contains(source, "aplicabil") || strings.Contains(source, "aferent") {
		rule.Expression = &Expression{Op: "variable", Variable: ApplicableVATVariable}
		rule.RequiredVariables = []string{ApplicableVATVariable}
		return rule, true
	}
	rate := ""
	for _, text := range []string{source, narrative} {
		for _, match := range sourcePercentPattern.FindAllStringSubmatch(text, -1) {
			value := strings.ReplaceAll(match[1], ",", ".")
			if rate != "" && rate != value {
				return Rule{}, false
			}
			rate = value
		}
	}
	if rate == "" || !sourcePercentPattern.MatchString(source) {
		return Rule{}, false
	}
	rule.Expression = &Expression{Op: "literal", Value: rate, Scale: 2}
	return rule, true
}
