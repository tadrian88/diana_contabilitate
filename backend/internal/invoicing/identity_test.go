package invoicing

import (
	"testing"
	"time"
)

func TestNormalizeBusinessIdentifierIsConservative(t *testing.T) {
	if got := NormalizeBusinessIdentifier("  ro-123\t /  ab  7 "); got != "RO-123 / AB 7" {
		t.Fatalf("got %q", got)
	}
}

func TestInvoiceIssueDayPreservesDocumentCalendarDate(t *testing.T) {
	location := time.FixedZone("document-zone", -5*60*60)
	input := time.Date(2026, time.January, 2, 23, 30, 0, 0, location)
	got := InvoiceIssueDay(input)
	want := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}
