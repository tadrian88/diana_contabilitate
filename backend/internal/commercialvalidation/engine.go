package commercialvalidation

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
)

var whitespace = regexp.MustCompile(`\s+`)

type Engine struct{}

func (Engine) Validate(input Input, now time.Time) Run {
	run := Run{InvoiceID: input.Invoice.ID, SnapshotID: input.Snapshot.ID, DossierID: input.Snapshot.DossierID, EngineVersion: EngineVersion, InvoiceRevision: input.Invoice.Revision, SnapshotVersion: input.Snapshot.Version, CreatedAt: now, CompletedAt: now}
	variables := make(map[string]string, len(input.Variables)+8)
	for name, value := range input.Variables {
		variables[name] = value.Value
	}
	variables["invoice_total"] = input.Invoice.Total.Amount.String()
	variables["invoice_line_count"] = fmt.Sprintf("%d", len(input.Invoice.Lines))
	quantityTotal := new(big.Rat)
	for _, line := range input.Invoice.Lines {
		if quantity, ok := rat(line.Quantity.String()); ok {
			quantityTotal.Add(quantityTotal, quantity)
		}
	}
	variables["invoice_quantity_total"] = decimal(quantityTotal, 4)
	findings := []Finding{}
	checkedLines := map[string]bool{}
	matchedRules := map[string][]string{}
	serviceCandidates := []ServiceCandidate{}
	if (!input.Snapshot.EffectiveFrom.IsZero() && input.Invoice.IssueDay.Before(input.Snapshot.EffectiveFrom)) || (input.Snapshot.EffectiveTo != nil && input.Invoice.IssueDay.After(*input.Snapshot.EffectiveTo)) {
		expected := input.Snapshot.EffectiveFrom.Format("02.01.2006") + " – fără dată de sfârșit"
		if input.Snapshot.EffectiveTo != nil {
			expected = input.Snapshot.EffectiveFrom.Format("02.01.2006") + " – " + input.Snapshot.EffectiveTo.Format("02.01.2006")
		}
		findings = append(findings, Finding{RuleID: "contract-validity", Code: "CONTRACT_NOT_EFFECTIVE", Outcome: Nonconform, Actual: input.Invoice.IssueDay.Format("02.01.2006"), ActualSource: "Data emiterii facturii", Expected: expected, Reason: "Contractul asociat nu este valabil la data facturii."})
	}
	if input.Snapshot.Coverage != CoverageComplete {
		findings = append(findings, Finding{RuleID: "snapshot-coverage", Code: "CONTRACT_COVERAGE_INCOMPLETE", Outcome: Unverifiable, Reason: "Dosarul contractual nu are acoperire comercială completă."})
	}
	if input.Invoice.DocumentType == invoicing.DocumentTypeCreditNote && input.Original == nil {
		findings = append(findings, Finding{RuleID: "credit-note", Code: "ORIGINAL_INVOICE_UNAVAILABLE", Outcome: Unverifiable, Reason: "Factura storno nu poate fi reconciliată fără factura originală."})
	} else if input.Invoice.DocumentType == invoicing.DocumentTypeCreditNote {
		findings = append(findings, validateCreditNote(input)...)
	}
	for _, rule := range input.Snapshot.Rules {
		applies, applicabilityMissing := ruleApplies(rule, input, variables)
		if applicabilityMissing != "" {
			findings = append(findings, Finding{RuleID: rule.ID, Code: "RULE_APPLICABILITY_MISSING", Outcome: Unverifiable, MissingInputs: []string{applicabilityMissing}, Reason: "Lipsește informația necesară stabilirii aplicabilității regulii.", Evidence: rule.Evidence})
			continue
		}
		if !applies {
			continue
		}
		if err := ValidateRule(rule); err != nil {
			findings = append(findings, Finding{RuleID: rule.ID, Code: "RULE_INVALID", Outcome: Unverifiable, Reason: "Regula confirmată nu poate fi evaluată.", Evidence: rule.Evidence})
			continue
		}
		if missing := missingDateBasis(rule, input); missing != "" {
			findings = append(findings, Finding{RuleID: rule.ID, Code: "RULE_DATE_BASIS_MISSING", Outcome: Unverifiable, MissingInputs: []string{missing}, Reason: "Lipsește data cerută explicit de baza temporală a regulii.", Evidence: rule.Evidence})
			continue
		}
		missingVariables := []string{}
		for _, name := range rule.RequiredVariables {
			if strings.TrimSpace(variables[name]) == "" {
				missingVariables = append(missingVariables, name)
			}
		}
		if len(missingVariables) > 0 {
			findings = append(findings, missingVariablesFinding(rule, input, missingVariables, "Lipsesc variabilele necesare regulii contractuale."))
			continue
		}
		switch rule.Kind {
		case RuleContractReference:
			actual := strings.TrimSpace(input.Reference)
			expected := strings.TrimSpace(rule.Expression.Value)
			outcome := Conform
			code, reason := "CONTRACT_REFERENCE_MATCH", "Referința facturii corespunde contractului asociat."
			if actual == "" {
				outcome, code, reason = Unverifiable, "CONTRACT_REFERENCE_MISSING", "Factura nu conține o referință contractuală verificabilă."
			} else if normalize(actual) != normalize(expected) {
				outcome, code, reason = Nonconform, "CONTRACT_REFERENCE_MISMATCH", "Referința declarată în factură diferă de contractul asociat după identitatea părților."
			}
			findings = append(findings, Finding{RuleID: rule.ID, Code: code, Outcome: outcome, Actual: actual, ActualSource: input.ReferenceSource, Expected: expected, Reason: reason, Evidence: rule.Evidence})
		case RuleFixedPrice, RuleUnitRate, RuleTieredPrice, RuleDiscount, RuleTranche, RuleProrata, RuleMinimum, RuleMaximum, RuleCostPlus, RuleFX, RuleVAT:
			if rule.Kind != RuleVAT {
				serviceCandidates = append(serviceCandidates, ServiceCandidate{RuleID: rule.ID, Label: rule.Narrative})
			}
			lines := matchingLines(rule, input.Invoice, input.Aliases)
			if len(lines) == 0 {
				continue
			}
			if rule.Kind != RuleVAT {
				for _, line := range lines {
					checkedLines[line.ID] = true
					matchedRules[line.ID] = append(matchedRules[line.ID], rule.ID)
				}
			}
			expected, missing, err := EvaluateExpression(*rule.Expression, variables)
			if err != nil || len(missing) > 0 {
				findings = append(findings, missingVariablesFinding(rule, input, missing, "Lipsesc variabilele necesare calculului contractual."))
				continue
			}
			for _, line := range lines {
				if rule.Currency != "" && rule.Kind != RuleVAT {
					lineCurrency := linePriceCurrency(line, input.Invoice.Total.Currency, rule.Currency)
					if lineCurrency == "" {
						findings = append(findings, Finding{RuleID: rule.ID, LineID: line.ID, Code: "LINE_PRICE_CURRENCY_MISSING", Outcome: Unverifiable, Expected: rule.Currency, MissingInputs: []string{"line_price_currency"}, Reason: "Moneda prețului de linie nu este disponibilă; moneda totalului nu poate fi substituită.", Evidence: rule.Evidence})
						continue
					}
					if lineCurrency != rule.Currency {
						findings = append(findings, Finding{RuleID: rule.ID, LineID: line.ID, Code: "CURRENCY_MISMATCH", Outcome: Nonconform, Actual: lineCurrency, Expected: rule.Currency, Reason: "Moneda prețului de linie diferă de moneda regulii contractuale.", Evidence: rule.Evidence})
						continue
					}
				}
				actual := commercialActual(rule.Kind, line)
				outcome, code, reason := Conform, "PRICE_MATCH", "Prețul liniei corespunde calculului contractual."
				if value, ok := rat(expected); !ok {
					outcome, code, reason = Unverifiable, "EXPECTED_PRICE_INVALID", "Calculul nu a produs un preț comparabil."
				} else if invoiceValue, valid := rat(actual); !valid || invoiceValue.Cmp(value) != 0 {
					outcome, code, reason = Nonconform, "PRICE_MISMATCH", "Prețul liniei diferă de calculul contractual."
				}
				findings = append(findings, Finding{RuleID: rule.ID, LineID: line.ID, Code: code, Outcome: outcome, Actual: actual, Expected: expected, Calculation: rule.Narrative, Reason: reason, Evidence: rule.Evidence})
				if rule.Kind == RuleUnitRate {
					quantityName := "unit_quantity_" + rule.ID
					quantity, hasQuantity := input.Variables[quantityName]
					if !hasQuantity || strings.TrimSpace(quantity.SourceReference) == "" {
						findings = append(findings, Finding{RuleID: rule.ID, LineID: line.ID, Code: "UNIT_QUANTITY_SOURCE_MISSING", Outcome: Unverifiable, MissingInputs: []string{quantityName}, Reason: "Cantitatea facturată la tarif unitar necesită o sursă verificabilă independentă.", Evidence: rule.Evidence})
					} else {
						quantityOutcome, quantityCode, quantityReason := compareDecimal(line.Quantity.String(), quantity.Value, "UNIT_QUANTITY_MATCH", "UNIT_QUANTITY_MISMATCH", "Cantitatea facturată corespunde sursei confirmate.", "Cantitatea facturată diferă de sursa confirmată.")
						findings = append(findings, Finding{RuleID: rule.ID, LineID: line.ID, Code: quantityCode, Outcome: quantityOutcome, Actual: line.Quantity.String(), Expected: quantity.Value, Reason: quantityReason, Evidence: rule.Evidence})
					}
				}
			}
		case RulePaymentDue:
			if input.Invoice.DueDate == nil {
				findings = append(findings, Finding{RuleID: rule.ID, Code: "PAYMENT_DUE_DATE_MISSING", Outcome: Unverifiable, MissingInputs: []string{"invoice_due_date"}, Reason: "Factura nu conține o scadență comparabilă.", Evidence: rule.Evidence})
				continue
			}
			expected, missing, err := EvaluateExpression(*rule.Expression, variables)
			if err != nil || len(missing) > 0 {
				findings = append(findings, Finding{RuleID: rule.ID, Code: "RULE_INPUT_MISSING", Outcome: Unverifiable, MissingInputs: missing, Reason: "Lipsesc datele necesare calculului scadenței.", Evidence: rule.Evidence})
				continue
			}
			baseDate, _ := dateBasisValue(rule, input)
			actual := fmt.Sprintf("%d", calendarDays(baseDate, *input.Invoice.DueDate))
			outcome, code, reason := compareDecimal(actual, expected, "PAYMENT_DUE_MATCH", "PAYMENT_DUE_MISMATCH", "Scadența corespunde clauzei contractuale.", "Scadența facturii diferă de clauza contractuală.")
			actualSource := ""
			if ruleIndicatesRemittance(rule) {
				actualSource = input.RemittanceSource
			}
			findings = append(findings, Finding{RuleID: rule.ID, Code: code, Outcome: outcome, Actual: actual, ActualSource: actualSource, Expected: expected, Calculation: rule.Narrative, Reason: reason, Evidence: rule.Evidence})
		case RuleFrequency:
			if input.Invoice.SourceFacts == nil || (input.Invoice.SourceFacts.PeriodStart == "" && input.Invoice.SourceFacts.PeriodEnd == "") {
				findings = append(findings, Finding{RuleID: rule.ID, Code: "SERVICE_PERIOD_MISSING", Outcome: Unverifiable, MissingInputs: []string{"service_period"}, Reason: "Periodicitatea nu poate fi verificată fără perioada serviciului.", Evidence: rule.Evidence})
			} else {
				outcome, code, reason := validateFrequency(input.Invoice.SourceFacts.PeriodStart, input.Invoice.SourceFacts.PeriodEnd, rule.Applicability.BillingFrequency)
				findings = append(findings, Finding{RuleID: rule.ID, Code: code, Outcome: outcome, Actual: input.Invoice.SourceFacts.PeriodStart + "/" + input.Invoice.SourceFacts.PeriodEnd, Expected: rule.Applicability.BillingFrequency, Reason: reason, Evidence: rule.Evidence})
			}
		case RuleCreditNote:
			// Credit-note invariants are evaluated once at invoice level above.
			continue
		case RuleIdentity:
			expected := ""
			if rule.Expression != nil {
				expected = rule.Expression.Value
			}
			actual := valueOrEmpty(input.Invoice.SupplierCUI)
			outcome, code, reason := Conform, "SUPPLIER_IDENTITY_MATCH", "Identitatea furnizorului corespunde regulii confirmate."
			if expected == "" || actual == "" {
				outcome, code, reason = Unverifiable, "SUPPLIER_IDENTITY_MISSING", "Identitatea furnizorului nu poate fi comparată."
			} else if normalize(actual) != normalize(expected) {
				outcome, code, reason = Nonconform, "SUPPLIER_IDENTITY_MISMATCH", "Identitatea furnizorului diferă de contract."
			}
			findings = append(findings, Finding{RuleID: rule.ID, Code: code, Outcome: outcome, Actual: actual, Expected: expected, Reason: reason, Evidence: rule.Evidence})
		default:
			findings = append(findings, Finding{RuleID: rule.ID, Code: "RULE_REQUIRES_INPUT", Outcome: Unverifiable, Reason: "Regula necesită o sursă sau un evaluator specializat neconfigurat.", Evidence: rule.Evidence})
		}
	}
	for _, line := range input.Invoice.Lines {
		if !checkedLines[line.ID] {
			findings = append(findings, Finding{RuleID: "line-coverage", LineID: line.ID, Code: "SERVICE_LINE_UNCOVERED", Outcome: Unverifiable, Actual: line.Description, Reason: "Linia facturii nu are o regulă comercială confirmată aplicabilă.", ServiceCandidates: serviceCandidates})
		} else if len(matchedRules[line.ID]) > 1 {
			findings = append(findings, Finding{RuleID: "line-coverage", LineID: line.ID, Code: "SERVICE_LINE_AMBIGUOUS", Outcome: Unverifiable, Actual: line.Description, Reason: "Linia se potrivește cu mai multe servicii contractuale; asocierea trebuie clarificată."})
		}
	}
	run.Findings = findings
	run.Outcome = aggregate(findings)
	return run
}

func missingVariablesFinding(rule Rule, input Input, missing []string, fallbackReason string) Finding {
	finding := Finding{RuleID: rule.ID, Code: "RULE_INPUT_MISSING", Outcome: Unverifiable, MissingInputs: missing, Reason: fallbackReason, Evidence: rule.Evidence}
	for _, name := range missing {
		value, exists := input.UnavailableVariables[name]
		if !exists {
			continue
		}
		finding.Code = "RULE_INPUT_OUTSIDE_VALIDITY"
		finding.Actual = value.Value
		finding.ActualSource = value.SourceReference
		finding.Expected = "valabilă la " + input.Invoice.IssueDay.Format("02.01.2006")
		from, to := "fără început precizat", "fără sfârșit precizat"
		if value.PeriodStart != nil {
			from = value.PeriodStart.Format("02.01.2006")
		}
		if value.PeriodEnd != nil {
			to = value.PeriodEnd.Format("02.01.2006")
		}
		finding.Calculation = from + " – " + to
		finding.Reason = "Valoarea este salvată, dar perioada ei de valabilitate nu acoperă data facturii."
		break
	}
	return finding
}

func calendarDays(from, to time.Time) int {
	fromDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toDay.Sub(fromDay).Hours() / 24)
}

func commercialActual(kind RuleKind, line invoicing.Line) string {
	if kind == RuleVAT {
		return line.VATRate.String()
	}
	if kind == RuleFixedPrice || kind == RuleUnitRate || kind == RuleTieredPrice || kind == RuleDiscount || kind == RuleTranche {
		return line.UnitPrice.String()
	}
	return line.NetValue.String()
}

func linePriceCurrency(line invoicing.Line, invoiceCurrency, expectedCurrency string) string {
	if line.SourceFacts != nil && line.SourceFacts.PriceAmount != nil {
		return line.SourceFacts.PriceAmount.Currency
	}
	if invoiceCurrency == expectedCurrency {
		return invoiceCurrency
	}
	return ""
}

func compareDecimal(actual, expected, matchCode, mismatchCode, matchReason, mismatchReason string) (Outcome, string, string) {
	want, wantOK := rat(expected)
	got, gotOK := rat(actual)
	if !wantOK || !gotOK {
		return Unverifiable, "EXPECTED_VALUE_INVALID", "Valorile nu pot fi comparate numeric."
	}
	if got.Cmp(want) != 0 {
		return Nonconform, mismatchCode, mismatchReason
	}
	return Conform, matchCode, matchReason
}

func validateFrequency(startRaw, endRaw, frequency string) (Outcome, string, string) {
	start, startErr := time.Parse("2006-01-02", startRaw)
	end, endErr := time.Parse("2006-01-02", endRaw)
	if startErr != nil || endErr != nil || end.Before(start) {
		return Unverifiable, "SERVICE_PERIOD_INVALID", "Perioada serviciului nu poate fi interpretată."
	}
	months := 0
	switch frequency {
	case "MONTHLY":
		months = 1
	case "QUARTERLY":
		months = 3
	case "ANNUAL":
		months = 12
	default:
		return Unverifiable, "FREQUENCY_REQUIRES_CONTEXT", "Frecvența contractuală nu poate fi dedusă numai din interval."
	}
	expectedEnd := start.AddDate(0, months, 0).AddDate(0, 0, -1)
	if !end.Equal(expectedEnd) {
		return Nonconform, "FREQUENCY_MISMATCH", "Perioada facturată diferă de frecvența contractuală."
	}
	return Conform, "FREQUENCY_MATCH", "Perioada facturată corespunde frecvenței contractuale."
}

func missingDateBasis(rule Rule, input Input) string {
	switch rule.DateBasis {
	case "", DateInvoiceIssue:
		return ""
	case DateServiceStart:
		if input.Invoice.SourceFacts == nil || input.Invoice.SourceFacts.PeriodStart == "" {
			return "service_period_start"
		}
		if _, err := time.Parse("2006-01-02", input.Invoice.SourceFacts.PeriodStart); err != nil {
			return "service_period_start"
		}
	case DateServiceEnd:
		if input.Invoice.SourceFacts == nil || input.Invoice.SourceFacts.PeriodEnd == "" {
			return "service_period_end"
		}
		if _, err := time.Parse("2006-01-02", input.Invoice.SourceFacts.PeriodEnd); err != nil {
			return "service_period_end"
		}
	case DateReceipt:
		if ruleIndicatesRemittance(rule) {
			if input.RemittanceDate == nil {
				return "remittance_date"
			}
			break
		}
		if input.ReceiptDate == nil {
			return "receipt_date"
		}
	case DateAcceptance:
		if input.AcceptanceDate == nil {
			return "acceptance_date"
		}
	case DateAnniversary:
		if input.Snapshot.EffectiveFrom.IsZero() {
			return "contract_effective_from"
		}
	}
	return ""
}

func dateBasisValue(rule Rule, input Input) (time.Time, bool) {
	switch rule.DateBasis {
	case "", DateInvoiceIssue:
		return input.Invoice.IssueDay, true
	case DateServiceStart:
		if input.Invoice.SourceFacts != nil {
			value, err := time.Parse("2006-01-02", input.Invoice.SourceFacts.PeriodStart)
			return value, err == nil
		}
	case DateServiceEnd:
		if input.Invoice.SourceFacts != nil {
			value, err := time.Parse("2006-01-02", input.Invoice.SourceFacts.PeriodEnd)
			return value, err == nil
		}
	case DateReceipt:
		if ruleIndicatesRemittance(rule) && input.RemittanceDate != nil {
			return *input.RemittanceDate, true
		}
		if input.ReceiptDate != nil {
			return *input.ReceiptDate, true
		}
	case DateAcceptance:
		if input.AcceptanceDate != nil {
			return *input.AcceptanceDate, true
		}
	case DateAnniversary:
		if !input.Snapshot.EffectiveFrom.IsZero() {
			return input.Snapshot.EffectiveFrom, true
		}
	}
	return time.Time{}, false
}

func ruleIndicatesRemittance(rule Rule) bool {
	if rule.DateBasis != DateReceipt {
		return false
	}
	text := strings.ToLower(rule.Narrative)
	for _, evidence := range rule.Evidence {
		text += " " + strings.ToLower(evidence.Snippet)
	}
	return strings.Contains(text, "remiter") || strings.Contains(text, "transmiter")
}

func validateCreditNote(input Input) []Finding {
	result := []Finding{}
	preceding := ""
	if input.Invoice.SourceFacts != nil {
		preceding = strings.TrimSpace(input.Invoice.SourceFacts.PrecedingInvoice)
	}
	if preceding == "" || input.Original.DocumentNumber == "" {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_REFERENCE_MISSING", Outcome: Unverifiable, MissingInputs: []string{"preceding_invoice_number"}, Reason: "Referința storno la factura originală nu poate fi verificată."})
	} else if invoicing.NormalizeBusinessIdentifier(preceding) != invoicing.NormalizeBusinessIdentifier(input.Original.DocumentNumber) {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_REFERENCE_MISMATCH", Outcome: Nonconform, Actual: preceding, Expected: input.Original.DocumentNumber, Reason: "Referința storno nu corespunde facturii originale."})
	} else {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_REFERENCE_MATCH", Outcome: Conform, Actual: preceding, Expected: input.Original.DocumentNumber, Reason: "Referința storno corespunde facturii originale."})
	}
	if input.Invoice.ClientID != "" && input.Original.ClientID != "" && input.Invoice.ClientID != input.Original.ClientID {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_CLIENT_MISMATCH", Outcome: Nonconform, Reason: "Factura originală aparține altui client."})
	}
	credit, creditOK := rat(input.Invoice.Total.Amount.String())
	original, originalOK := rat(input.Original.Total.Amount.String())
	if !creditOK || !originalOK {
		return []Finding{{RuleID: "credit-note", Code: "CREDIT_NOTE_VALUE_INVALID", Outcome: Unverifiable, Reason: "Valorile storno nu pot fi comparate."}}
	}
	if input.Invoice.Total.Currency != input.Original.Total.Currency {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_CURRENCY_MISMATCH", Outcome: Nonconform, Actual: input.Invoice.Total.Currency, Expected: input.Original.Total.Currency, Reason: "Moneda storno diferă de factura originală."})
	}
	if credit.Sign() >= 0 {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_SIGN_MISMATCH", Outcome: Nonconform, Actual: input.Invoice.Total.Amount.String(), Expected: "valoare negativă", Reason: "Storno trebuie să inverseze semnul valorii originale."})
	} else {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_SIGN_MATCH", Outcome: Conform, Actual: input.Invoice.Total.Amount.String(), Expected: "valoare negativă", Reason: "Semnul storno este corect."})
	}
	absCredit := new(big.Rat).Abs(credit)
	absOriginal := new(big.Rat).Abs(original)
	if absCredit.Cmp(absOriginal) > 0 {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_EXCEEDS_ORIGINAL", Outcome: Nonconform, Actual: input.Invoice.Total.Amount.String(), Expected: input.Original.Total.Amount.String(), Reason: "Valoarea absolută storno depășește factura originală."})
	} else {
		result = append(result, Finding{RuleID: "credit-note", Code: "CREDIT_NOTE_PROPORTION_VALID", Outcome: Conform, Actual: input.Invoice.Total.Amount.String(), Expected: input.Original.Total.Amount.String(), Reason: "Valoarea storno nu depășește factura originală."})
	}
	return result
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func ValidateRule(rule Rule) error {
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Narrative) == "" || len(rule.Evidence) == 0 {
		return ErrInvalidRule
	}
	if !ValidRuleKind(rule.Kind) {
		return ErrInvalidRule
	}
	if rule.DateBasis != "" && rule.DateBasis != DateInvoiceIssue && rule.DateBasis != DateServiceStart && rule.DateBasis != DateServiceEnd && rule.DateBasis != DateReceipt && rule.DateBasis != DateAcceptance && rule.DateBasis != DateAnniversary {
		return ErrInvalidRule
	}
	for _, evidence := range rule.Evidence {
		if strings.TrimSpace(evidence.Snippet) == "" {
			return ErrInvalidRule
		}
	}
	if rule.Kind == RuleContractReference || rule.Kind == RuleIdentity {
		if rule.Expression == nil || rule.Expression.Op != "literal" || strings.TrimSpace(rule.Expression.Value) == "" {
			return ErrInvalidRule
		}
		return nil // Identity and reference literals are text, not decimal AST values.
	}
	if requiresExpression(rule.Kind) {
		if rule.Expression == nil {
			return ErrInvalidRule
		}
		return ValidateExpression(*rule.Expression)
	}
	if rule.Expression != nil {
		return ValidateExpression(*rule.Expression)
	}
	return nil
}

func ValidRuleKind(kind RuleKind) bool {
	switch kind {
	case RuleIdentity, RuleContractReference, RuleFixedPrice, RuleUnitRate,
		RuleTieredPrice, RuleDiscount, RuleTranche, RuleProrata,
		RuleMinimum, RuleMaximum, RuleCostPlus, RuleFX, RuleVAT,
		RulePaymentDue, RuleFrequency, RuleCreditNote:
		return true
	default:
		return false
	}
}

func requiresExpression(kind RuleKind) bool {
	switch kind {
	case RuleFixedPrice, RuleUnitRate, RuleTieredPrice, RuleDiscount, RuleTranche, RuleProrata, RuleMinimum, RuleMaximum, RuleCostPlus, RuleFX, RuleVAT, RulePaymentDue:
		return true
	default:
		return false
	}
}

func ruleApplies(rule Rule, input Input, variables map[string]string) (bool, string) {
	if len(rule.Applicability.DocumentTypes) == 0 {
		// Continue with the remaining applicability dimensions.
	} else {
		matched := false
		for _, value := range rule.Applicability.DocumentTypes {
			if value == string(input.Invoice.DocumentType) {
				matched = true
				break
			}
		}
		if !matched {
			return false, ""
		}
	}
	if rule.Applicability.ContractYear != nil {
		if input.Snapshot.EffectiveFrom.IsZero() {
			return false, "contract_effective_from"
		}
		year := input.Invoice.IssueDay.Year() - input.Snapshot.EffectiveFrom.Year() + 1
		anniversary := time.Date(input.Invoice.IssueDay.Year(), input.Snapshot.EffectiveFrom.Month(), input.Snapshot.EffectiveFrom.Day(), 0, 0, 0, 0, time.UTC)
		if input.Invoice.IssueDay.Before(anniversary) {
			year--
		}
		if year != *rule.Applicability.ContractYear {
			return false, ""
		}
	}
	if rule.Applicability.Tranche != nil {
		value := variables["tranche"]
		if value == "" {
			return false, "tranche"
		}
		if value != fmt.Sprintf("%d", *rule.Applicability.Tranche) {
			return false, ""
		}
	}
	return true, ""
}

func matchingLines(rule Rule, invoice invoicing.Invoice, aliases []Alias) []invoicing.Line {
	if rule.Kind == RuleVAT && rule.Applicability.ServiceID == "" && len(rule.Applicability.Aliases) == 0 && len(rule.Applicability.SKUs) == 0 {
		return invoice.Lines
	}
	labels := append([]string(nil), rule.Applicability.Aliases...)
	serviceID := rule.Applicability.ServiceID
	if serviceID == "" {
		serviceID = rule.ID
	}
	for _, alias := range aliases {
		if alias.ServiceID == serviceID {
			labels = append(labels, alias.NormalizedLabel)
		}
	}
	for _, sku := range rule.Applicability.SKUs {
		labels = append(labels, sku)
	}
	if len(labels) == 0 {
		return nil
	}
	result := []invoicing.Line{}
	for _, line := range invoice.Lines {
		description := normalize(line.Description)
		additional := ""
		if line.AdditionalInfo != nil {
			additional = normalize(*line.AdditionalInfo)
		}
		for _, label := range labels {
			needle := normalize(label)
			if needle != "" && (strings.Contains(description, needle) || strings.Contains(additional, needle)) {
				result = append(result, line)
				break
			}
		}
	}
	return result
}

func normalize(value string) string {
	return whitespace.ReplaceAllString(strings.ToUpper(strings.TrimSpace(value)), " ")
}

func aggregate(findings []Finding) Outcome {
	result := Conform
	for _, finding := range findings {
		if finding.Override != nil {
			continue
		}
		if finding.Outcome == Nonconform {
			return Nonconform
		}
		if finding.Outcome == Unverifiable {
			result = Unverifiable
		}
	}
	return result
}

func SortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].RuleID == findings[j].RuleID {
			return findings[i].LineID < findings[j].LineID
		}
		return findings[i].RuleID < findings[j].RuleID
	})
}
