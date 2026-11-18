package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
)

// defaultHistoryDays is the default history range (calendar days) when start is omitted.
const defaultHistoryDays = 30

// DataQuery is one /api/data request: a key and an optional start/end range.
type DataQuery struct {
	Key, Start, End string
	// Now anchors the default range (captured once by the caller).
	Now time.Time
}

// DataResult is the answer to a DataQuery; exactly one of the value fields is set.
type DataResult struct {
	Register solis.Register
	Current  *solis.Value            // latest cached value (no range)
	Total    *history.TotalDataPoint // lifetime value of a total key
	Status   *StatusHistory          // decoded change history of a status key
	Rows     any                     // []*history.{Daily,Monthly,Yearly}DataPoint
}

// HistoryCapable reports whether start/end apply to a register store: daily, monthly and
// yearly registers have history rows and status registers a change log (totals have a
// single lifetime value, cache-only registers no history at all).
func HistoryCapable(s solis.Store) bool {
	return s == solis.StoreDaily || s == solis.StoreMonthly || s == solis.StoreYearly ||
		s == solis.StoreStatus
}

// Data answers /api/data by register store: no range = latest cached value; a range on
// a daily/monthly/yearly key = history rows; a total key = lifetime value; a status key =
// decoded change history (all of it, or the local days of the range). A range on any
// other key is ErrWrongKind.
func (s *ReadService) Data(ctx context.Context, q DataQuery) (DataResult, error) {
	reg, err := s.Register(q.Key)
	if err != nil {
		return DataResult{}, err
	}
	hasRange := q.Start != "" || q.End != ""
	if hasRange && !HistoryCapable(reg.Store) {
		return DataResult{Register: reg}, &KeyError{
			Key: q.Key, Err: ErrWrongKind,
			Detail: "historical queries (start/end) are only supported for daily, " +
				"monthly, yearly and status registers",
		}
	}
	return s.dispatch(ctx, reg, q, hasRange)
}

// dispatch reads what the register's store returns (see Data).
func (s *ReadService) dispatch(ctx context.Context, reg solis.Register, q DataQuery,
	hasRange bool,
) (DataResult, error) {
	res := DataResult{Register: reg}
	var err error
	switch {
	case reg.Store == solis.StoreTotal:
		res.Total, err = s.Total(ctx, q.Key)
	case reg.Store == solis.StoreStatus:
		res.Status, err = s.statusRange(ctx, q, hasRange)
	case hasRange:
		res.Rows, err = s.periodHistory(ctx, reg, q)
	default:
		res.Current, err = s.Current(q.Key)
	}
	return res, err
}

func (s *ReadService) periodHistory(ctx context.Context, reg solis.Register, q DataQuery) (
	any, error,
) {
	tr, err := ParseTimeRange(q.Start, q.End, q.Now)
	if err != nil {
		return nil, err
	}
	switch reg.Store {
	case solis.StoreMonthly:
		return s.MonthlyHistory(ctx, reg.Key, tr.Start, tr.End)
	case solis.StoreYearly:
		return s.YearlyHistory(ctx, reg.Key, tr.Start, tr.End)
	default:
		return s.DailyHistory(ctx, reg.Key, tr.Start, tr.End)
	}
}

// statusRange reads the status history of q.Key: everything without a range, otherwise
// the changes from local midnight of the start day through the end day.
func (s *ReadService) statusRange(ctx context.Context, q DataQuery, hasRange bool) (
	*StatusHistory, error,
) {
	start, end := time.Time{}, maxTime()
	if hasRange {
		tr, err := ParseTimeRange(q.Start, q.End, q.Now)
		if err != nil {
			return nil, err
		}
		start, end = localDayBounds(tr, q.Now.Location())
	}
	sh, err := s.StatusHistory(ctx, q.Key, start, end)
	if err != nil {
		return nil, err
	}
	return &sh, nil
}

// localDayBounds turns a calendar-day range into instants: from local midnight of the
// start day to the last millisecond (the stored precision) before local midnight after
// the end day, so DST days keep their 23 or 25 hours.
func localDayBounds(tr TimeRange, loc *time.Location) (start, end time.Time) {
	midnight := func(d time.Time) time.Time {
		y, m, day := d.Date()
		return time.Date(y, m, day, 0, 0, 0, 0, loc)
	}
	return midnight(tr.Start), midnight(tr.End.AddDate(0, 0, 1)).Add(-time.Millisecond)
}

// TimeRange is a parsed history range.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// ParseTimeRange parses start/end as YYYY-MM-DD, YYYY-MM or YYYY; start defaults to 30
// days before today and end to today (the local day of now). A month or year end is
// inclusive: it expands to the last day of that period (end=2026 covers all of 2026).
// Both bounds are calendar days at midnight UTC, like the parsed parameters, so a default
// never compares as a different instant on the same day.
func ParseTimeRange(start, end string, now time.Time) (TimeRange, error) {
	today := calendarDay(now)
	s, _, err := parseTime(start, today.AddDate(0, 0, -defaultHistoryDays))
	if err != nil {
		return TimeRange{}, err
	}
	e, layout, err := parseTime(end, today)
	if err != nil {
		return TimeRange{}, err
	}
	switch layout {
	case period.MonthLayout:
		e = e.AddDate(0, 1, -1)
	case period.YearLayout:
		e = e.AddDate(1, 0, -1)
	}
	if s.After(e) {
		return TimeRange{}, fmt.Errorf("%w: start %s is after end %s", ErrInvalidRange,
			s.Format(period.DayLayout), e.Format(period.DayLayout))
	}
	return TimeRange{Start: s, End: e}, nil
}

// calendarDay returns the calendar day of t (in t's own location) as midnight UTC.
func calendarDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// parseTime returns the parsed time and the matching layout ("" for the default).
func parseTime(s string, def time.Time) (time.Time, string, error) {
	if s == "" {
		return def, "", nil
	}
	for _, layout := range []string{period.DayLayout, period.MonthLayout, period.YearLayout} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, layout, nil
		}
	}
	return time.Time{}, "", fmt.Errorf("%w: %q (want YYYY-MM-DD, YYYY-MM or YYYY)",
		ErrInvalidRange, s)
}
