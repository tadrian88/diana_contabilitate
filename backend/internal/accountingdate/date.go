// Package accountingdate models civil accounting dates without timezone arithmetic.
package accountingdate

import (
	"fmt"
	"time"
)

type Date string

func Parse(value string) (Date, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02") != value {
		return "", fmt.Errorf("invalid accounting date %q", value)
	}
	return Date(value), nil
}

// FromTime preserves calendar components at existing transport/persistence boundaries.
func FromTime(value time.Time) Date {
	if value.IsZero() {
		return ""
	}
	return Date(value.Format("2006-01-02"))
}
func (d Date) Valid() bool { _, err := Parse(string(d)); return err == nil }
func (d Date) Within(from Date, to *Date) bool {
	return d.Valid() && from.Valid() && (to == nil || (to.Valid() && *to >= from)) && d >= from && (to == nil || d <= *to)
}
