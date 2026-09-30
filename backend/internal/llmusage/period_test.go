package llmusage_test

import (
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/llmusage"
)

func TestParsePeriodDefaultsToMonthToDateInBucharest(t *testing.T) {
	// 22:30 UTC on 31 October is already 1 November in Bucharest.
	period, err := llmusage.ParsePeriod("", "", time.Date(2026, 10, 31, 22, 30, 0, 0, time.UTC))
	if err != nil || period.From != "2026-11-01" || period.To != "2026-11-01" {
		t.Fatalf("period=%+v err=%v", period, err)
	}
	if !period.Start.Equal(time.Date(2026, 10, 31, 22, 0, 0, 0, time.UTC)) || !period.End.Equal(time.Date(2026, 11, 1, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("bounds %s - %s", period.Start, period.End)
	}
}

func TestParsePeriodKeepsWholeDaysAcrossDST(t *testing.T) {
	// Bucharest leaves summer time on 25 October 2026: that day lasts 25 hours.
	period, err := llmusage.ParsePeriod("2026-10-25", "2026-10-25", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !period.Start.Equal(time.Date(2026, 10, 24, 21, 0, 0, 0, time.UTC)) || !period.End.Equal(time.Date(2026, 10, 25, 22, 0, 0, 0, time.UTC)) || period.End.Sub(period.Start) != 25*time.Hour {
		t.Fatalf("bounds %s - %s", period.Start, period.End)
	}
}

func TestParsePeriodRejectsInvalidRanges(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, tc := range [][2]string{{"2026-09-10", "2026-09-01"}, {"2025-01-01", "2026-01-02"}, {"2026-9-1", ""}, {"2026-02-30", ""}, {"", "yesterday"}} {
		if _, err := llmusage.ParsePeriod(tc[0], tc[1], now); !errors.Is(err, apperrors.ErrValidation) {
			t.Fatalf("%v accepted: %v", tc, err)
		}
	}
	if _, err := llmusage.ParsePeriod("2025-01-01", "2026-01-01", now); err != nil {
		t.Fatalf("366 days must be accepted: %v", err)
	}
}
