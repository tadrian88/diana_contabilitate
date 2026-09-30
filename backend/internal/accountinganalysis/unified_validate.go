package accountinganalysis

import (
	"fmt"
	"strings"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

type ValidatedDecision struct {
	InvoiceLineID string
	Dimension     string
	Proposal      DimensionProposal
	TypedValue    *accounting.Value
	Citations     []accounting.LegalCitation
	Issues        []ValidationIssue
}

func (d ValidatedDecision) Valid() bool { return len(d.Issues) == 0 && d.TypedValue != nil }

// ValidateUnified is deliberately granular. It never discards a valid
// dimension because a sibling dimension, account, or citation is invalid.
func ValidateUnified(input Input, proposal Proposal, fragments []legislation.Fragment, catalog AccountCatalog) ([]ValidatedDecision, []ValidationIssue) {
	results := []ValidatedDecision{}
	aggregate := []ValidationIssue{}
	lineByID := map[string]Line{}
	for _, line := range input.Lines {
		lineByID[line.ID] = line
	}
	resolved := map[string]bool{}
	for _, item := range input.ResolvedDimensions {
		resolved[item.InvoiceLineID+":"+item.Dimension] = true
	}
	corpus := map[string]legislation.Fragment{}
	for _, fragment := range fragments {
		corpus[fragment.ID] = fragment
	}
	seen := map[string]bool{}
	addGlobal := func(code, path, message string) {
		aggregate = append(aggregate, ValidationIssue{Code: code, Path: path, Message: message})
	}
	if proposal.SchemaVersion != SchemaVersion {
		addGlobal("SCHEMA_VERSION", "schemaVersion", "versiune de schemă necunoscută")
	}
	if proposal.Source != SourceAIProposal {
		addGlobal("SOURCE", "source", "sursa trebuie să fie AI_PROPOSAL")
	}
	for lineIndex, lineProposal := range proposal.Lines {
		line, lineExists := lineByID[lineProposal.InvoiceLineID]
		for decisionIndex, decision := range lineProposal.Decisions {
			path := fmt.Sprintf("lines[%d].decisions[%d]", lineIndex, decisionIndex)
			result := ValidatedDecision{InvoiceLineID: lineProposal.InvoiceLineID, Dimension: decision.Dimension, Proposal: decision}
			add := func(code, field, message string) {
				issue := ValidationIssue{Code: code, Path: path + field, Message: message}
				result.Issues = append(result.Issues, issue)
				aggregate = append(aggregate, issue)
			}
			if !lineExists {
				add("FOREIGN_LINE", ".invoiceLineId", "linia nu aparține facturii analizate")
			}
			if !isAccountingDimension(decision.Dimension) {
				add("DIMENSION", ".dimension", "dimensiune contabilă necunoscută")
			}
			key := lineProposal.InvoiceLineID + ":" + decision.Dimension
			if seen[key] {
				add("DUPLICATE_DIMENSION", ".dimension", "dimensiune duplicată pentru aceeași linie")
			}
			seen[key] = true
			if resolved[key] {
				add("DIMENSION_ALREADY_RESOLVED", ".dimension", "AI nu poate suprascrie o decizie deterministă sau deja finală")
			}
			if strings.TrimSpace(decision.Explanation) == "" {
				add("EXPLANATION", ".explanation", "explicația concisă este obligatorie")
			}
			if !validConfidence(decision.Confidence) {
				add("CONFIDENCE", ".confidence", "nivel de încredere necunoscut")
			}
			if lineExists && decision.Dimension == "VAT_TREATMENT" {
				decision.ProposedValue = withSourceTaxFacts(decision.ProposedValue, line)
				result.Proposal = decision
			}
			if decision.Insufficient {
				add("INSUFFICIENT_FACTS", ".insufficient", "informațiile disponibile nu susțin o propunere validă")
			} else if err := decision.ProposedValue.Validate(decision.Dimension); err != nil {
				add("INVALID_TYPED_VALUE", ".proposedValue", err.Error())
			} else {
				value := decision.ProposedValue
				result.TypedValue = &value
			}
			if result.TypedValue != nil && decision.Dimension == "ACCOUNT" {
				for _, issue := range validateAnalysisAccount(catalog, input.Profile, result.TypedValue.Account, path+".proposedValue.account") {
					issue.Path = path + ".proposedValue.account"
					result.Issues = append(result.Issues, issue)
					aggregate = append(aggregate, issue)
				}
			}
			if result.TypedValue != nil && lineExists {
				validateContextualValue(input, line, decision.Dimension, *result.TypedValue, add)
			}
			if len(decision.Citations) == 0 {
				add("LEGAL_CITATION", ".citations", "propunerea AI necesită cel puțin o citare verificabilă")
			}
			for citationIndex, citation := range decision.Citations {
				fragment, ok := corpus[citation.FragmentID]
				verified := ok && fragment.Valid() && fragment.VersionID == citation.VersionID && fragment.CitationKey == citation.CitationKey && strings.EqualFold(fragment.ContentHash, citation.ContentHash)
				result.Citations = append(result.Citations, citation.Evidence(verified))
				if !verified {
					add("LEGAL_CITATION", fmt.Sprintf(".citations[%d]", citationIndex), "citarea nu poate fi verificată în corpusul rulării")
				}
			}
			results = append(results, result)
		}
	}
	for _, line := range input.Lines {
		for _, dimension := range accounting.Dimensions {
			key := line.ID + ":" + dimension
			if !resolved[key] && !seen[key] {
				addGlobal("MISSING_DIMENSION", "lines", "lipsește propunerea pentru "+key)
			}
		}
	}
	return results, aggregate
}

// withSourceTaxFacts copies the e-Factura line tax category and rate into a
// VAT_TREATMENT proposal only when the provider omitted both. They are source
// facts, not judgments; values the provider did send are never replaced, so a
// mismatch still fails VAT_SOURCE_MISMATCH.
func withSourceTaxFacts(value accounting.Value, line Line) accounting.Value {
	rate := sourceTaxRate(line)
	if value.SourceCategory != "" || value.SourceRate != nil || rate == nil || strings.TrimSpace(line.Facts.Code) == "" {
		return value
	}
	value.SourceCategory = line.Facts.Code
	value.SourceRate = rate
	return value
}

// sourceTaxRate is the line's VAT rate as a source fact. EN 16931 (BR-O-05)
// forbids a rate on a line not subject to VAT (category O), so such a line's
// rate is zero by definition rather than unknown. Any other missing rate stays
// missing.
func sourceTaxRate(line Line) *money.Amount {
	if line.Facts == nil {
		return nil
	}
	if line.Facts.Rate != nil {
		rate := *line.Facts.Rate
		return &rate
	}
	if strings.TrimSpace(line.Facts.Code) == "O" {
		zero := money.MustParse("0")
		return &zero
	}
	return nil
}

func validateContextualValue(input Input, line Line, dimension string, value accounting.Value, add func(string, string, string)) {
	switch dimension {
	case "VAT_TREATMENT":
		if rate := sourceTaxRate(line); rate == nil || value.SourceRate == nil || !rate.Equal(*value.SourceRate) || strings.TrimSpace(value.SourceCategory) != strings.TrimSpace(line.Facts.Code) {
			add("VAT_SOURCE_MISMATCH", ".proposedValue", "cota sau categoria TVA nu corespunde faptelor liniei")
		}
		if input.SourceFacts != nil && input.SourceFacts.CashAccounting == "YES" && value.Timing != "DEFERRED" {
			add("VAT_CASH_ACCOUNTING_MISMATCH", ".proposedValue.timing", "factura indică TVA la încasare, iar momentul propus nu este amânat")
		}
	case "VAT_DEDUCTIBILITY":
		if input.Direction == Outgoing && value.Kind != "NOT_APPLICABLE" {
			add("DIRECTION_MISMATCH", ".proposedValue", "dreptul de deducere TVA nu este aplicabil facturii emise")
		}
		if line.VAT.Equal("0") && value.Kind != "NOT_APPLICABLE" {
			add("ZERO_VAT_MISMATCH", ".proposedValue", "linia fără TVA nu poate avea drept de deducere TVA")
		}
		if input.Profile != nil && input.Profile.VATRegistration == "NOT_REGISTERED" && value.Kind != "NONE" && value.Kind != "NOT_APPLICABLE" {
			add("FISCAL_PROFILE_MISMATCH", ".proposedValue", "profilul neînregistrat în scopuri de TVA nu permite deducerea propusă")
		}
		if input.Profile != nil && input.Profile.DeductionActivity == "WITHOUT_DEDUCTION_RIGHT" && value.Kind != "NONE" {
			add("FISCAL_PROFILE_MISMATCH", ".proposedValue", "profilul indică activitate fără drept de deducere TVA")
		}
	case "EXPENSE_TAX_TREATMENT":
		if input.Direction == Outgoing && value.Kind != "NOT_APPLICABLE" {
			add("DIRECTION_MISMATCH", ".proposedValue", "tratamentul fiscal al cheltuielii nu este aplicabil facturii emise")
		}
		if input.Profile != nil && input.Profile.TaxRegime == "MICROENTERPRISE" && value.Kind != "NOT_APPLICABLE" {
			add("FISCAL_PROFILE_MISMATCH", ".proposedValue", "profilul microîntreprinderii nu permite o concluzie de impozit pe profit")
		}
	}
}

func isAccountingDimension(value string) bool {
	for _, dimension := range accounting.Dimensions {
		if value == dimension {
			return true
		}
	}
	return false
}

func validConfidence(value Confidence) bool {
	return value == ConfidenceHigh || value == ConfidenceMedium || value == ConfidenceLow || value == ConfidenceUnknown
}

func validationResults(issues []ValidationIssue) []accounting.ValidationResult {
	result := make([]accounting.ValidationResult, 0, len(issues))
	for _, issue := range issues {
		item := accounting.ValidationResult{Code: issue.Code, Message: issue.Message}
		for _, account := range issue.SuggestedAccounts {
			item.SuggestedAccounts = append(item.SuggestedAccounts, account.Code)
		}
		result = append(result, item)
	}
	return result
}

func ValidationResults(issues []ValidationIssue) []accounting.ValidationResult {
	return validationResults(issues)
}
