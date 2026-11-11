package report

import (
	"errors"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// ErrInvalidRange: bad from/to, granularity or comparison parameters (400).
var ErrInvalidRange = errors.New("invalid report parameters")

const (
	// MaxRangeDays caps a report at about ten years (8 keys x 3660 rows per query).
	MaxRangeDays = 3660
	// autoDayMaxDays is the longest range grouped by day (about two months).
	autoDayMaxDays = 62
	// autoMonthMaxDays is the longest range grouped by month (two years).
	autoMonthMaxDays = 731
	hoursPerDay      = 24
	monthsPerYear    = 12
)

// Range is an inclusive range of calendar days, each as midnight UTC.
type Range struct {
	From, To time.Time
}

// Days returns the number of calendar days in r.
func (r Range) Days() int {
	return int(r.To.Sub(r.From).Hours()/hoursPerDay) + 1
}

// Contains reports whether day lies in r.
func (r Range) Contains(day time.Time) bool {
	return !day.Before(r.From) && !day.After(r.To)
}

// Finished returns r without today. Today's counters are still running, so a report
// covers finished days only: a range ending today ends yesterday instead (excluded is
// true). A range of today alone has no finished day and wraps ErrInvalidRange.
func (r Range) Finished(today time.Time) (out Range, excluded bool, err error) {
	if !r.To.Equal(today) {
		return r, false, nil
	}
	if r.From.Equal(today) {
		return Range{}, false, fmt.Errorf("%w: today is still in progress; a report covers "+
			"finished days only", ErrInvalidRange)
	}
	return Range{From: r.From, To: today.AddDate(0, 0, -1)}, true, nil
}

// String formats r as "YYYY-MM-DD..YYYY-MM-DD".
func (r Range) String() string {
	return r.From.Format(period.DayLayout) + ".." + r.To.Format(period.DayLayout)
}

// ParseDay parses a YYYY-MM-DD day as midnight UTC (the calendar day of Range).
func ParseDay(s string) (time.Time, error) { return parseDay("day", s) }

// ParseRange parses from/to (YYYY-MM-DD, inclusive) and checks from <= to <= today and
// the maximum length. today is the caller's local day (server TZ).
func ParseRange(from, to, today string) (Range, error) {
	f, err := parseDay("from", from)
	if err != nil {
		return Range{}, err
	}
	t, err := parseDay("to", to)
	if err != nil {
		return Range{}, err
	}
	now, err := parseDay("today", today)
	if err != nil {
		return Range{}, err
	}
	r := Range{From: f, To: t}
	switch {
	case t.Before(f):
		return Range{}, fmt.Errorf("%w: from %s is after to %s", ErrInvalidRange, from, to)
	case t.After(now):
		return Range{}, fmt.Errorf("%w: to %s is after today %s", ErrInvalidRange, to, today)
	case r.Days() > MaxRangeDays:
		return Range{}, fmt.Errorf("%w: range of %d days exceeds %d days",
			ErrInvalidRange, r.Days(), MaxRangeDays)
	}
	return r, nil
}

// parseDay parses a YYYY-MM-DD parameter.
func parseDay(name, s string) (time.Time, error) {
	t, err := time.Parse(period.DayLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s %q (want YYYY-MM-DD)", ErrInvalidRange, name, s)
	}
	return t, nil
}

// Granularity is how buckets group the days of a range.
type Granularity int

const (
	// ByDay is one bucket per day.
	ByDay Granularity = iota
	// ByMonth is one bucket per calendar month.
	ByMonth
	// ByYear is one bucket per calendar year.
	ByYear
)

// String returns the API name of g.
func (g Granularity) String() string {
	switch g {
	case ByMonth:
		return "month"
	case ByYear:
		return "year"
	default:
		return "day"
	}
}

// ParseGranularity parses "day", "month" or "year"; "" picks AutoGranularity(r).
func ParseGranularity(s string, r Range) (Granularity, error) {
	for _, g := range []Granularity{ByDay, ByMonth, ByYear} {
		if s == g.String() {
			return g, nil
		}
	}
	if s == "" {
		return AutoGranularity(r), nil
	}
	return ByDay, fmt.Errorf("%w: granularity %q (want day, month or year)", ErrInvalidRange, s)
}

// AutoGranularity groups up to ~2 months by day, up to 2 years by month, longer by year.
func AutoGranularity(r Range) Granularity {
	switch n := r.Days(); {
	case n <= autoDayMaxDays:
		return ByDay
	case n <= autoMonthMaxDays:
		return ByMonth
	default:
		return ByYear
	}
}

// bounds returns the first and last day of the bucket containing day.
func (g Granularity) bounds(day time.Time) (first, last time.Time) {
	switch g {
	case ByMonth:
		first = time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
		return first, first.AddDate(0, 1, -1)
	case ByYear:
		first = time.Date(day.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
		return first, first.AddDate(1, 0, -1)
	default:
		return day, day
	}
}

// key returns the bucket key of day (YYYY-MM-DD, YYYY-MM or YYYY).
func (g Granularity) key(day time.Time) string {
	switch g {
	case ByMonth:
		return day.Format(period.MonthLayout)
	case ByYear:
		return day.Format(period.YearLayout)
	default:
		return day.Format(period.DayLayout)
	}
}

// CompareMode selects the comparison period.
type CompareMode int

const (
	// CompareNone disables the comparison.
	CompareNone CompareMode = iota
	// ComparePrevious compares with the period directly before.
	ComparePrevious
	// CompareLastYear compares with the same dates one year earlier.
	CompareLastYear
)

// String returns the API name of m.
func (m CompareMode) String() string {
	switch m {
	case ComparePrevious:
		return "previous"
	case CompareLastYear:
		return "last_year"
	default:
		return "none"
	}
}

// ParseCompare parses "previous", "last_year" or "none"; "" is the default last_year
// (seasons make the previous period a poor comparison for most ranges).
func ParseCompare(s string) (CompareMode, error) {
	if s == "" {
		return CompareLastYear, nil
	}
	for _, m := range []CompareMode{CompareNone, ComparePrevious, CompareLastYear} {
		if s == m.String() {
			return m, nil
		}
	}
	return CompareNone, fmt.Errorf("%w: compare %q (want previous, last_year or none)",
		ErrInvalidRange, s)
}

// Of returns the comparison range of r; ok is false for CompareNone.
func (m CompareMode) Of(r Range) (cmp Range, ok bool) {
	switch m {
	case ComparePrevious:
		return previous(r), true
	case CompareLastYear:
		return Range{From: yearBefore(r.From), To: yearBefore(r.To)}, true
	default:
		return Range{}, false
	}
}

// previous is the period directly before r:
//   - whole months: the same number of calendar months (October -> September, a year ->
//     the year before, a quarter -> the quarter before);
//   - a period to date starting on the 1st: the same days of the period before (1-29 Oct
//     -> 1-29 Sep; 1 Jan-29 Oct -> the same dates a year earlier; 1 Apr-15 Jun -> 1 Jan-
//     15 Mar), a day past the month end becomes its last day;
//   - otherwise the same number of days directly before.
func previous(r Range) Range {
	if n, ok := wholeMonths(r); ok {
		return Range{From: r.From.AddDate(0, -n, 0), To: r.From.AddDate(0, 0, -1)}
	}
	if r.From.Day() == 1 {
		n := spanMonths(r)
		if r.From.Month() == time.January && r.From.Year() == r.To.Year() && n > 1 {
			n = monthsPerYear // year to date
		}
		return Range{From: monthsBefore(r.From, n), To: monthsBefore(r.To, n)}
	}
	return Range{From: r.From.AddDate(0, 0, -r.Days()), To: r.From.AddDate(0, 0, -1)}
}

// wholeMonths returns the number of calendar months r covers when it starts on a
// first and ends on a last day of a month.
func wholeMonths(r Range) (int, bool) {
	if r.From.Day() != 1 || r.To.AddDate(0, 0, 1).Day() != 1 {
		return 0, false
	}
	return spanMonths(r), true
}

// spanMonths returns the number of calendar months r touches.
func spanMonths(r Range) int {
	return (r.To.Year()-r.From.Year())*monthsPerYear + int(r.To.Month()-r.From.Month()) + 1
}

// yearBefore returns day one year earlier; 29 February becomes 28 February.
func yearBefore(day time.Time) time.Time {
	return monthsBefore(day, monthsPerYear)
}

// monthsBefore returns day n calendar months earlier; a day past the end of that month
// becomes its last day (31 Mar -> 28 Feb, 29 Feb -> 28 Feb a year earlier).
func monthsBefore(day time.Time, n int) time.Time {
	first := time.Date(day.Year(), day.Month()-time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day.Day(), last), 0, 0, 0, 0, time.UTC)
}
