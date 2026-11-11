package service

import (
	"context"
	"fmt"

	"github.com/dombyte/solis/internal/report"
)

// ReportQuery is one /api/report request; empty Compare and Granularity pick the
// defaults (same period last year, automatic grouping).
type ReportQuery struct {
	From, To, Compare, Granularity string
	// Today is the local day (server TZ, YYYY-MM-DD), captured once by the caller.
	Today string
}

// Report validates q, reads the daily rows of the range (and of the comparison period)
// and computes the report. A range ending today is reported up to yesterday. Invalid
// parameters wrap report.ErrInvalidRange.
func (s *ReadService) Report(ctx context.Context, q ReportQuery) (report.Report, error) {
	in, err := reportInput(q)
	if err != nil {
		return report.Report{}, err
	}
	s.d.Log.Debug().Stringer("range", in.Range).Stringer("granularity", in.Granularity).
		Msg("building report")
	if in.Days, err = s.reportDays(ctx, in.Range); err != nil {
		return report.Report{}, err
	}
	if in.Compare != nil {
		if in.Compare.Days, err = s.reportDays(ctx, in.Compare.Range); err != nil {
			return report.Report{}, err
		}
	}
	return report.Build(in), nil
}

// reportInput parses the query parameters.
func reportInput(q ReportQuery) (report.Input, error) {
	r, excluded, err := finishedRange(q)
	if err != nil {
		return report.Input{}, err
	}
	g, err := report.ParseGranularity(q.Granularity, r)
	if err != nil {
		return report.Input{}, err
	}
	mode, err := report.ParseCompare(q.Compare)
	if err != nil {
		return report.Input{}, err
	}
	in := report.Input{Range: r, Granularity: g, TodayExcluded: excluded}
	if cmp, ok := mode.Of(r); ok {
		in.Compare = &report.CompareInput{Mode: mode, Range: cmp}
	}
	return in, nil
}

// finishedRange parses the range of q and cuts today off: its counters are still
// running, so the report (and its comparison period) covers finished days only.
func finishedRange(q ReportQuery) (r report.Range, todayExcluded bool, err error) {
	r, err = report.ParseRange(q.From, q.To, q.Today)
	if err != nil {
		return report.Range{}, false, err
	}
	today, err := report.ParseDay(q.Today)
	if err != nil {
		return report.Range{}, false, err
	}
	return r.Finished(today)
}

// reportDays reads the complete days of r: one bounded query per energy series.
func (s *ReadService) reportDays(ctx context.Context, r report.Range) ([]report.Day, error) {
	keys := report.DailyKeys()
	values := make(map[string]map[string]float64, len(keys))
	for _, key := range keys {
		rows, err := s.d.Store.GetDailyHistory(ctx, key, r.From, r.To)
		if err != nil {
			return nil, fmt.Errorf("service: report rows of %s: %w", key, err)
		}
		byDay := make(map[string]float64, len(rows))
		for _, p := range rows {
			byDay[p.Date] = p.Value
		}
		values[key] = byDay
	}
	return report.Assemble(values)
}
