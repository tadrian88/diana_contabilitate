package invoicing

import (
	"diana-contabilitate/backend/internal/fiscalidentity"
	"strings"
	"time"
)

// DuplicatePolicy isolates the provisional fingerprint so later accounting
// validation can replace/version it without coupling ValidationTask or pipeline
// lifecycle behavior to the current formula.
type DuplicatePolicy struct{ Version string }

var ProvisionalDuplicatePolicy = DuplicatePolicy{Version: "PROVISIONAL_V1"}

// NormalizeBusinessIdentifier keeps punctuation significant and normalizes only
// casing and whitespace. More aggressive supplier/document canonicalization is
// a product decision and must not be inferred by the ingestion layer.
func NormalizeBusinessIdentifier(value string) string {
	return fiscalidentity.ForComparison(value, "")
}

func InvoiceIssueDay(value time.Time) time.Time {
	return ProvisionalDuplicatePolicy.IssueDay(value)
}

func (DuplicatePolicy) NormalizeIdentifier(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(value), " "))
}

func (DuplicatePolicy) IssueDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
