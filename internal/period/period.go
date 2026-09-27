// Package period is the single source of day/month/year keys. A Period is derived
// once from a captured instant so the poller, aggregator, storage and CLI never
// disagree on date formatting.
package period

import (
	"errors"
	"fmt"
	"time"
)

// Key layouts used for storage period columns.
const (
	// DayLayout formats daily keys (YYYY-MM-DD).
	DayLayout = "2006-01-02"
	// MonthLayout formats monthly keys (YYYY-MM).
	MonthLayout = "2006-01"
	// YearLayout formats yearly keys (YYYY).
	YearLayout = "2006"
)

// ErrInvalidKey is returned when a period key does not match its layout.
var ErrInvalidKey = errors.New("invalid period key")

// Level identifies a storage period granularity.
type Level int

const (
	// Daily is the daily_values level.
	Daily Level = iota
	// Monthly is the monthly_values level.
	Monthly
	// Yearly is the yearly_values level.
	Yearly
	// Total is the total_values level (never closes).
	Total
)

// String returns the level name used in logs and reports.
func (l Level) String() string {
	switch l {
	case Daily:
		return "daily"
	case Monthly:
		return "monthly"
	case Yearly:
		return "yearly"
	case Total:
		return "total"
	default:
		return "unknown"
	}
}

// Period holds the day, month and year keys of one captured instant.
type Period struct {
	// At is the instant the keys were derived from.
	At time.Time
	// Day is the YYYY-MM-DD key.
	Day string
	// Month is the YYYY-MM key.
	Month string
	// Year is the YYYY key.
	Year string
}

// Of derives the period keys of t in t's own location. Callers pass a clock reading in
// local time (time.Local, i.e. the TZ environment variable).
func Of(t time.Time) Period {
	return Period{
		At:    t,
		Day:   t.Format(DayLayout),
		Month: t.Format(MonthLayout),
		Year:  t.Format(YearLayout),
	}
}

// Key returns the key of p at the given level ("" for Total).
func (p Period) Key(l Level) string {
	switch l {
	case Daily:
		return p.Day
	case Monthly:
		return p.Month
	case Yearly:
		return p.Year
	default:
		return ""
	}
}

// AddDays shifts a day key by n calendar days.
func AddDays(day string, n int) (string, error) {
	t, err := time.Parse(DayLayout, day)
	if err != nil {
		return "", fmt.Errorf("%w: day %q", ErrInvalidKey, day)
	}
	return t.AddDate(0, 0, n).Format(DayLayout), nil
}

// MonthBounds returns the first and last day keys of a month key.
func MonthBounds(month string) (first, last string, err error) {
	t, err := time.Parse(MonthLayout, month)
	if err != nil {
		return "", "", fmt.Errorf("%w: month %q", ErrInvalidKey, month)
	}
	return t.Format(DayLayout), t.AddDate(0, 1, -1).Format(DayLayout), nil
}

// YearBounds returns the first and last day keys of a year key.
func YearBounds(year string) (first, last string, err error) {
	t, err := time.Parse(YearLayout, year)
	if err != nil {
		return "", "", fmt.Errorf("%w: year %q", ErrInvalidKey, year)
	}
	return t.Format(DayLayout), t.AddDate(1, 0, -1).Format(DayLayout), nil
}

// AddMonths shifts a month key by n months.
func AddMonths(month string, n int) (string, error) {
	t, err := time.Parse(MonthLayout, month)
	if err != nil {
		return "", fmt.Errorf("%w: month %q", ErrInvalidKey, month)
	}
	return t.AddDate(0, n, 0).Format(MonthLayout), nil
}

// AddYears shifts a year key by n years.
func AddYears(year string, n int) (string, error) {
	t, err := time.Parse(YearLayout, year)
	if err != nil {
		return "", fmt.Errorf("%w: year %q", ErrInvalidKey, year)
	}
	return t.AddDate(n, 0, 0).Format(YearLayout), nil
}

// MonthsBetween lists month keys from (exclusive) after to (inclusive) through.
// An empty after starts at through. Keys compare lexically, which matches time order.
func MonthsBetween(after, through string) ([]string, error) {
	return stepBetween(after, through, AddMonths)
}

// YearsBetween lists year keys from (exclusive) after to (inclusive) through.
func YearsBetween(after, through string) ([]string, error) {
	return stepBetween(after, through, AddYears)
}

func stepBetween(after, through string, step func(string, int) (string, error)) ([]string, error) {
	if after == "" {
		return []string{through}, nil
	}
	var out []string
	cur, err := step(after, 1)
	for err == nil && cur <= through {
		out = append(out, cur)
		cur, err = step(cur, 1)
	}
	return out, err
}
