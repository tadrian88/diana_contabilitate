package commercialvalidation

import "diana-contabilitate/backend/internal/fiscalidentity"

// IdentityRuleNamesSupplier reports whether an identity rule states the
// supplier's CUI. Invoices are checked against an identity rule by their
// supplier CUI, so a rule stating the client's or anyone else's CUI would
// make every invoice of the contract nonconforming.
func IdentityRuleNamesSupplier(rule Rule, supplierCUI string) bool {
	return rule.Expression != nil && fiscalidentity.Same(rule.Expression.Value, supplierCUI)
}
