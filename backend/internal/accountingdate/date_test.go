package accountingdate

import (
	"testing"
	"time"
)

func TestCivilAccountingDatesHaveNoTimezoneArithmetic(t *testing.T) {
	for _, value := range []string{"2025-08-01", "2024-02-29"} {
		if _, err := Parse(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"2025-02-29", "2025-8-1", "2025-08-01T00:00:00Z", ""} {
		if _, err := Parse(value); err == nil {
			t.Fatal(value)
		}
	}
	local := time.Date(2025, 8, 1, 0, 0, 0, 0, time.FixedZone("RO", 3*3600))
	if FromTime(local) != "2025-08-01" {
		t.Fatal("calendar date shifted")
	}
}
