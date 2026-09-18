package contracts

import "diana-contabilitate/backend/internal/money"

type CommercialVerificationOutcome string

const (
	CommercialMatch         CommercialVerificationOutcome = "MATCH"
	CommercialMismatch      CommercialVerificationOutcome = "MISMATCH"
	CommercialNeedsInput    CommercialVerificationOutcome = "NEEDS_INPUT"
	CommercialNotApplicable CommercialVerificationOutcome = "NOT_APPLICABLE"
)

type CommercialVerificationResult struct {
	Outcome CommercialVerificationOutcome
	Reasons []string
}

// VerifyExactPrice is deliberately narrow: callers must first deterministically
// select a service term. It performs no fuzzy service matching and does not
// affect the accounting-classification pipeline.
func VerifyExactPrice(term ServiceTerm, invoiceUnitPrice money.Amount, currency string, quantityKnown bool) CommercialVerificationResult {
	if term.UnitPrice == nil {
		return CommercialVerificationResult{Outcome: CommercialNotApplicable, Reasons: []string{"Contractul nu conține un preț comparabil."}}
	}
	if term.Currency != currency || term.UnitPrice.String() != invoiceUnitPrice.String() {
		return CommercialVerificationResult{Outcome: CommercialMismatch, Reasons: []string{"Prețul unitar sau moneda diferă de termenul contractual."}}
	}
	if term.PricingModel == "UNIT_RATE" && !quantityKnown {
		return CommercialVerificationResult{Outcome: CommercialNeedsInput, Reasons: []string{"Cantitatea relevantă trebuie confirmată înainte de verificarea valorii."}}
	}
	return CommercialVerificationResult{Outcome: CommercialMatch, Reasons: []string{"Prețul și moneda coincid exact cu termenul contractual."}}
}
