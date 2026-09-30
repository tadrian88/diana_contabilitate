package llmusage

import (
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/apperrors"
)

const (
	TimeZone      = "Europe/Bucharest"
	MaxPeriodDays = 366
)

// Period is an inclusive range of Romanian calendar days. Start and End are
// the UTC instants bounding it; End is exclusive.
type Period struct {
	From, To   accountingdate.Date
	Start, End time.Time
}

// ParsePeriod defaults to the current month up to today.
func ParsePeriod(fromRaw, toRaw string, now time.Time) (Period, error) {
	location, err := time.LoadLocation(TimeZone)
	if err != nil {
		return Period{}, err
	}
	today := now.In(location)
	from := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, location)
	to := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location)
	if value := strings.TrimSpace(fromRaw); value != "" {
		if from, err = parseDay(value, location); err != nil {
			return Period{}, err
		}
	}
	if value := strings.TrimSpace(toRaw); value != "" {
		if to, err = parseDay(value, location); err != nil {
			return Period{}, err
		}
	}
	if to.Before(from) {
		return Period{}, fmt.Errorf("%w: period ends before it starts", apperrors.ErrValidation)
	}
	civilFrom := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	civilTo := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if int(civilTo.Sub(civilFrom).Hours()/24)+1 > MaxPeriodDays {
		return Period{}, fmt.Errorf("%w: period longer than %d days", apperrors.ErrValidation, MaxPeriodDays)
	}
	// time.Date normalizes the next day in local time, so DST days stay whole.
	end := time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, location)
	return Period{From: accountingdate.Date(from.Format("2006-01-02")), To: accountingdate.Date(to.Format("2006-01-02")), Start: from.UTC(), End: end.UTC()}, nil
}

func parseDay(value string, location *time.Location) (time.Time, error) {
	if _, err := accountingdate.Parse(value); err != nil {
		return time.Time{}, fmt.Errorf("%w: %v", apperrors.ErrValidation, err)
	}
	return time.ParseInLocation("2006-01-02", value, location)
}
