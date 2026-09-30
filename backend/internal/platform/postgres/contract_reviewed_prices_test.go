package postgres

import (
	"testing"

	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
)

func TestReviewedServicePriceRulesRequireConfirmedValueAndPriceEvidence(t *testing.T) {
	price, currency := "500", "RON"
	terms := []contractingestion.ReviewedServiceTerm{{ServiceDescription: "Contabilitate", PricingModel: "FIXED_FEE", UnitPrice: price, Currency: currency, BillingFrequency: "MONTHLY"}}
	proposed := []contractingestion.ProposedServiceTerm{{UnitPrice: contractingestion.Field{Value: &price, Evidence: contractingestion.Evidence{Snippet: "Tarif contabilitate 500 lei lunar"}}, Currency: contractingestion.Field{Value: &currency}}}
	rules, skipped := reviewedServicePriceRules("document-1", "contract-1", terms, proposed)
	if len(rules) != 1 || len(skipped) != 0 || rules[0].Expression.Value != "500" || rules[0].Evidence[0].Snippet != "Tarif contabilitate 500 lei lunar" {
		t.Fatalf("source-backed reviewed price not promoted: %+v skipped=%+v", rules, skipped)
	}
	terms[0].UnitPrice = "600" // A changed invoice price cannot become the contract price.
	if got, skipped := reviewedServicePriceRules("document-1", "contract-1", terms, proposed); len(got) != 0 || len(skipped) != 1 || skipped[0].Reason != commercialvalidation.SkipPriceChangedFromSource {
		t.Fatalf("unsubstantiated reviewed price was promoted: %+v skipped=%+v", got, skipped)
	}
	terms[0].UnitPrice = "500"
	proposed[0].UnitPrice.Evidence.Snippet = "Servicii contabile, tarif conform anexei"
	if got, skipped := reviewedServicePriceRules("document-1", "contract-1", terms, proposed); len(got) != 0 || len(skipped) != 1 || skipped[0].Reason != commercialvalidation.SkipPriceNotInSource {
		t.Fatalf("price without quoted amount was promoted: %+v skipped=%+v", got, skipped)
	}
}
