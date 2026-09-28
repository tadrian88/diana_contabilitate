package accountinganalysis

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	accountdomain "diana-contabilitate/backend/internal/accounts"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

var ErrInvalidProposal = errors.New("invalid accounting analysis proposal")

type AccountCatalog interface{ Postable(code string) bool }

type DetailedAccountCatalog interface {
	Account(code string) (*accountdomain.Account, bool)
	PostableChildren(code string) []accountdomain.Account
}

type Catalog struct {
	Entries  map[string]accountdomain.Account
	Children map[string][]accountdomain.Account
}

func (c Catalog) Postable(code string) bool {
	item, ok := c.Entries[code]
	return ok && item.Active && item.Postable
}
func (c Catalog) Account(code string) (*accountdomain.Account, bool) {
	item, ok := c.Entries[code]
	return &item, ok
}
func (c Catalog) PostableChildren(code string) []accountdomain.Account { return c.Children[code] }

type ValidationIssue struct {
	Code              string                  `json:"code"`
	Path              string                  `json:"path"`
	Message           string                  `json:"message"`
	AccountCode       string                  `json:"accountCode,omitempty"`
	SuggestedAccounts []accountdomain.Account `json:"suggestedAccounts,omitempty"`
}

// Validate is the non-LLM trust boundary. Every provider response and every
// edited review payload must cross it before persistence.
func ValidateLegacy(input Input, proposal Proposal, fragments []legislation.Fragment, catalog AccountCatalog) []ValidationIssue {
	issues := []ValidationIssue{}
	add := func(code, path, message string) {
		issues = append(issues, ValidationIssue{Code: code, Path: path, Message: message})
	}
	if proposal.SchemaVersion != LegacySchemaVersion {
		add("SCHEMA_VERSION", "schemaVersion", "versiune de schemă necunoscută")
	}
	if proposal.ClientID != input.ClientID {
		add("TENANT_MISMATCH", "clientId", "clientul propunerii nu corespunde facturii")
	}
	if proposal.InvoiceID != input.InvoiceID || proposal.InvoiceRevision != input.InvoiceRevision {
		add("INVOICE_VERSION_MISMATCH", "invoiceId", "factura sau revizia nu corespunde")
	}
	if proposal.Source == SourceLLMLegislation && !proposal.RequiresReview {
		add("REVIEW_REQUIRED", "requiresReview", "o propunere LLM necesită validare umană")
	}
	if proposal.Confidence != ConfidenceHigh && proposal.Confidence != ConfidenceMedium && proposal.Confidence != ConfidenceLow && proposal.Confidence != ConfidenceUnknown {
		add("CONFIDENCE", "confidence", "confidence trebuie să fie categorie, nu probabilitate")
	}
	if strings.TrimSpace(proposal.ReasoningSummary) == "" {
		add("REASONING", "reasoningSummary", "rezumatul motivării este obligatoriu")
	}

	lines := map[string]Line{}
	for _, line := range input.Lines {
		lines[line.ID] = line
	}
	usedLines := map[string]bool{}
	creditControl, debitControl := new(big.Rat), new(big.Rat)
	vatPosting := new(big.Rat)
	for index, entry := range proposal.Entries {
		path := fmt.Sprintf("entries[%d]", index)
		amount, ok := rat(entry.Amount)
		if !ok || amount.Sign() <= 0 {
			add("AMOUNT", path+".amount", "suma trebuie să fie pozitivă")
		}
		if entry.Currency != input.Currency {
			add("CURRENCY", path+".currency", "moneda nu corespunde facturii")
		}
		issues = append(issues, validateAnalysisAccount(catalog, input.Profile, entry.DebitAccount, path+".debitAccount")...)
		issues = append(issues, validateAnalysisAccount(catalog, input.Profile, entry.CreditAccount, path+".creditAccount")...)
		if strings.TrimSpace(entry.Explanation) == "" {
			add("EXPLANATION", path+".explanation", "explicația este obligatorie")
		}
		if len(entry.InvoiceLineIDs) == 0 {
			add("LINE_COVERAGE", path+".invoiceLineIds", "înregistrarea trebuie legată de linii sursă")
		}
		seen := map[string]bool{}
		for _, id := range entry.InvoiceLineIDs {
			if _, exists := lines[id]; !exists {
				add("FOREIGN_LINE", path+".invoiceLineIds", "linie inexistentă sau din altă factură")
			}
			if seen[id] {
				add("DUPLICATE_LINE", path+".invoiceLineIds", "linie duplicată în aceeași înregistrare")
			}
			seen[id], usedLines[id] = true, true
		}
		if entry.Phase == InvoicePhase && ok {
			if input.Direction == Incoming && entry.CreditAccount == "401" {
				creditControl.Add(creditControl, amount)
			}
			if input.Direction == Outgoing && entry.DebitAccount == "4111" {
				debitControl.Add(debitControl, amount)
			}
			if isVATAccount(entry.DebitAccount) || isVATAccount(entry.CreditAccount) {
				vatPosting.Add(vatPosting, amount)
			}
		} else if entry.Phase != PaymentPhase && entry.Phase != CollectionPhase {
			add("PHASE", path+".phase", "fază contabilă necunoscută")
		}
	}
	invoiceTotal, totalOK := rat(input.Total)
	if totalOK && input.Direction == Incoming && creditControl.Cmp(invoiceTotal) != 0 {
		add("INVOICE_RECONCILIATION", "entries", "creditul 401 la factură nu reconciliază totalul")
	}
	if totalOK && input.Direction == Outgoing && debitControl.Cmp(invoiceTotal) != 0 {
		add("INVOICE_RECONCILIATION", "entries", "debitul 4111 la factură nu reconciliază totalul")
	}
	for id := range lines {
		if !usedLines[id] {
			add("LINE_COVERAGE", "entries", "linia "+id+" nu este acoperită")
		}
	}

	treatments := map[string]bool{}
	expectedVAT := new(big.Rat)
	for _, line := range input.Lines {
		if value, ok := rat(line.VAT); ok {
			expectedVAT.Add(expectedVAT, value)
		}
	}
	for index, treatment := range proposal.LineTreatments {
		path := fmt.Sprintf("lineTreatments[%d]", index)
		line, exists := lines[treatment.InvoiceLineID]
		if !exists {
			add("FOREIGN_LINE", path+".invoiceLineId", "linie inexistentă")
		} else {
			if !line.Net.Equal(treatment.VATBase) || !line.VAT.Equal(treatment.VATAmount) {
				add("VAT_SOURCE_MISMATCH", path, "baza sau TVA nu corespunde sursei")
			}
		}
		if treatments[treatment.InvoiceLineID] {
			add("DUPLICATE_TREATMENT", path, "tratament duplicat")
		}
		treatments[treatment.InvoiceLineID] = true
		if !oneOf(treatment.VATDeductibility, "FULL", "LIMITED", "NONE", "NOT_APPLICABLE", "NEEDS_REVIEW") {
			add("VAT_DEDUCTIBILITY", path, "deductibilitate TVA invalidă")
		}
		if !oneOf(treatment.ExpenseDeductibility, "FULL", "LIMITED", "NONE", "NOT_APPLICABLE", "NEEDS_REVIEW") {
			add("EXPENSE_DEDUCTIBILITY", path, "deductibilitate fiscală invalidă")
		}
	}
	for id := range lines {
		if !treatments[id] {
			add("TREATMENT_COVERAGE", "lineTreatments", "lipsește tratamentul liniei "+id)
		}
	}
	if expectedVAT.Sign() == 0 && vatPosting.Sign() != 0 {
		add("ZERO_VAT_POSTING", "entries", "factura fără TVA nu poate genera o sumă TVA")
	}
	if expectedVAT.Sign() != 0 && vatPosting.Cmp(expectedVAT) != 0 {
		add("VAT_RECONCILIATION", "entries", "înregistrările TVA nu reconciliază TVA sursă")
	}

	corpus := map[string]legislation.Fragment{}
	for _, fragment := range fragments {
		corpus[fragment.ID] = fragment
	}
	for index, citation := range proposal.Citations {
		fragment, exists := corpus[citation.FragmentID]
		if !exists || !fragment.Valid() || fragment.VersionID != citation.VersionID || fragment.CitationKey != citation.CitationKey || !strings.EqualFold(fragment.ContentHash, citation.ContentHash) {
			add("LEGAL_CITATION", fmt.Sprintf("citations[%d]", index), "citarea nu poate fi verificată în corpusul utilizat")
		}
	}
	if proposal.Source == SourceLLMLegislation && len(proposal.Citations) == 0 {
		add("LEGAL_CITATION", "citations", "analiza LLM necesită cel puțin o citare verificabilă")
	}
	return issues
}

func validateAnalysisAccount(catalog AccountCatalog, profile interface{ AccountAllowed(string) bool }, code, path string) []ValidationIssue {
	if detailed, ok := catalog.(DetailedAccountCatalog); ok {
		item, exists := detailed.Account(code)
		if !exists {
			item = nil
		}
		var allowed func(string) bool
		if profile != nil {
			allowed = profile.AccountAllowed
		}
		err := accountdomain.ValidatePostingAccount(item, code, allowed)
		if err == nil {
			return nil
		}
		issue, _ := accountdomain.AsValidationIssue(err)
		result := ValidationIssue{Code: string(issue.Code), Path: path, Message: issue.Message, AccountCode: code}
		if issue.Code == accountdomain.AccountNotPostable {
			for _, candidate := range detailed.PostableChildren(code) {
				if profile == nil || profile.AccountAllowed(candidate.Code) {
					result.SuggestedAccounts = append(result.SuggestedAccounts, candidate)
				}
			}
		}
		return []ValidationIssue{result}
	}
	if catalog == nil || !catalog.Postable(code) {
		return []ValidationIssue{{Code: string(accountdomain.AccountNotFound), Path: path, Message: "cont inexistent sau nepostabil", AccountCode: code}}
	}
	if profile == nil || !profile.AccountAllowed(code) {
		return []ValidationIssue{{Code: string(accountdomain.AccountNotAllowedProfile), Path: path, Message: "contul nu aparține vocabularului aprobat al clientului", AccountCode: code}}
	}
	return nil
}

func rat(value money.Amount) (*big.Rat, bool) { return new(big.Rat).SetString(value.String()) }
func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
func isVATAccount(value string) bool { return value == "4426" || value == "4427" || value == "4428" }
