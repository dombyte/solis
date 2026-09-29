package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
)

// defaultHistoryWindow is the default history range when start is omitted.
const defaultHistoryWindow = 30 * 24 * time.Hour

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

// HistoryCapable reports whether start/end apply to a register store: only daily,
// monthly and yearly registers have history rows (totals have a single lifetime value).
func HistoryCapable(s solis.Store) bool {
	return s == solis.StoreDaily || s == solis.StoreMonthly || s == solis.StoreYearly
}

// Data answers /api/data by register store: no range = latest cached value; a range on
// a daily/monthly/yearly key = history rows; a total key = lifetime value; a status key =
// decoded change history. A range on any other key is ErrWrongKind.
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
				"monthly and yearly registers",
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
		var sh StatusHistory
		if sh, err = s.StatusHistory(ctx, q.Key); err == nil {
			res.Status = &sh
		}
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

// TimeRange is a parsed history range.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// ParseTimeRange parses start/end as YYYY-MM-DD, YYYY-MM or YYYY; start defaults to 30
// days before now and end to now. A month or year end is inclusive: it expands to the
// last day of that period (end=2026 covers all of 2026).
func ParseTimeRange(start, end string, now time.Time) (TimeRange, error) {
	s, _, err := parseTime(start, now.Add(-defaultHistoryWindow))
	if err != nil {
		return TimeRange{}, err
	}
	e, layout, err := parseTime(end, now)
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
