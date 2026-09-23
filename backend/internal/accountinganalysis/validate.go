package accountinganalysis

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

var ErrInvalidProposal = errors.New("invalid accounting analysis proposal")

type AccountCatalog interface{ Postable(code string) bool }

type Catalog map[string]bool

func (c Catalog) Postable(code string) bool { return c[code] }

type ValidationIssue struct{ Code, Path, Message string }

// Validate is the non-LLM trust boundary. Every provider response and every
// edited review payload must cross it before persistence.
func Validate(input Input, proposal Proposal, fragments []legislation.Fragment, accounts AccountCatalog) []ValidationIssue {
	issues := []ValidationIssue{}
	add := func(code, path, message string) { issues = append(issues, ValidationIssue{code, path, message}) }
	if proposal.SchemaVersion != SchemaVersion {
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
		if accounts == nil || !accounts.Postable(entry.DebitAccount) {
			add("ACCOUNT", path+".debitAccount", "cont debit inexistent sau nepostabil")
		}
		if accounts == nil || !accounts.Postable(entry.CreditAccount) {
			add("ACCOUNT", path+".creditAccount", "cont credit inexistent sau nepostabil")
		}
		if input.Profile == nil || !input.Profile.AccountAllowed(entry.DebitAccount) || !input.Profile.AccountAllowed(entry.CreditAccount) {
			add("PROFILE_ACCOUNT", path, "contul nu aparține vocabularului aprobat al clientului")
		}
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
