package contracts

import (
	"diana-contabilitate/backend/internal/money"
	"testing"
)

func TestExactCommercialVerificationNeedsUnitRateQuantity(t *testing.T) {
	price := money.MustParse("50")
	term := ServiceTerm{PricingModel: "UNIT_RATE", UnitPrice: &price, Currency: "RON", Unit: "SALARIAT", QuantitySource: "UNKNOWN"}
	if got := VerifyExactPrice(term, money.MustParse("50"), "RON", false); got.Outcome != CommercialNeedsInput {
		t.Fatalf("outcome=%s", got.Outcome)
	}
	if got := VerifyExactPrice(term, money.MustParse("55"), "RON", true); got.Outcome != CommercialMismatch {
		t.Fatalf("outcome=%s", got.Outcome)
	}
}
