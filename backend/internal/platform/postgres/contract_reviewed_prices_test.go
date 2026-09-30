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

func TestReviewedServicePriceRulesPairEachServiceWithItsOwnProposalRow(t *testing.T) {
	maintenance, hosting, currency := "1800.00", "650.00", "RON"
	proposed := []contractingestion.ProposedServiceTerm{
		{ServiceDescription: contractingestion.Field{Evidence: contractingestion.Evidence{Snippet: "Mentenanță IT"}}, UnitPrice: contractingestion.Field{Value: &maintenance, Evidence: contractingestion.Evidence{Snippet: "1.800,00 lei / lună"}}, Currency: contractingestion.Field{Value: &currency}},
		{ServiceDescription: contractingestion.Field{Evidence: contractingestion.Evidence{Snippet: "Hosting cloud"}}, UnitPrice: contractingestion.Field{Value: &hosting, Evidence: contractingestion.Evidence{Snippet: "650,00 lei / lună"}}, Currency: contractingestion.Field{Value: &currency}},
	}
	second, manual := 1, contractingestion.ManualServiceTerm
	// The reviewer removed the first service and added one the extraction missed.
	terms := []contractingestion.ReviewedServiceTerm{
		{ServiceDescription: "Hosting cloud", PricingModel: "FIXED_FEE", UnitPrice: hosting, Currency: currency, BillingFrequency: "MONTHLY", SourceIndex: &second},
		{ServiceDescription: "Backup", PricingModel: "FIXED_FEE", UnitPrice: "1800.00", Currency: currency, BillingFrequency: "MONTHLY", SourceIndex: &manual},
	}
	rules, skipped := reviewedServicePriceRules("document-1", "contract-1", terms, proposed)
	if len(rules) != 1 || rules[0].Expression.Value != hosting || rules[0].Evidence[0].Snippet != "650,00 lei / lună" || rules[0].Evidence[1].Snippet != "Hosting cloud" {
		t.Fatalf("hosting not paired with its own row: %+v", rules)
	}
	if len(skipped) != 1 || skipped[0].Position != 2 || skipped[0].Reason != commercialvalidation.SkipSourceMissing {
		t.Fatalf("a service added at review took another row's evidence: %+v", skipped)
	}
}
