package aggregator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/aggregation"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

// runState accumulates one run's writes and the values for the cache.
type runState struct {
	write   storage.ComputedWrite
	current aggregation.Values
	base    aggregation.Values
	baseYr  string
}

// execute performs one run at the captured instant now.
func (a *Aggregator) execute(ctx context.Context, now time.Time) error {
	p := period.Of(now)
	st, err := a.d.Store.CloseState(ctx)
	if err != nil {
		return err
	}
	plan, err := aggregation.BuildPlan(p, aggregation.Watermarks{
		FrozenMonth: st.FrozenMonth, FrozenYear: st.FrozenYear,
		FrozenNetDay: st.FrozenNetDay, ClosedThrough: st.ClosedThrough,
	})
	if err != nil {
		return err
	}
	rs := &runState{
		write: storage.ComputedWrite{At: now}, current: aggregation.Values{},
		base: st.Baseline, baseYr: st.BaselineYear,
	}
	steps := []func(context.Context, period.Period, aggregation.Plan, *runState) error{
		a.netDays, a.months, a.years, a.totals,
	}
	for _, step := range steps {
		if err := step(ctx, p, plan, rs); err != nil {
			return err
		}
	}
	if err := a.d.Store.WriteComputed(ctx, rs.write); err != nil {
		if !storage.IsRejection(err) {
			return err
		}
		// Rejected rows were skipped; everything else (rows, freezes, fold) committed,
		// so the run succeeded and the cache is merged (review AGG-M3).
		a.logRejections(err)
	}
	a.d.Cache.Merge(eventbus.DomainAggregator, a.values(rs.current, now), now)
	return nil
}

// logRejections classifies each rejected row: a write into a period that was closed
// meanwhile is lag (debug); a write-domain or unknown-key rejection is an aggregator
// bug that a restart cannot fix, so it is logged as an error instead of turning the
// component Recovering on every run.
func (a *Aggregator) logRejections(err error) {
	for _, e := range joinedErrors(err) {
		if errors.Is(e, storage.ErrPeriodClosed) {
			a.d.Log.Debug().Err(e).Msg("computed row rejected: period closed meanwhile")
			continue
		}
		a.d.Log.Error().Err(e).Str("tag", "write_domain").
			Msg("computed row rejected by storage (aggregator bug)")
	}
}

// joinedErrors unpacks an errors.Join result into its parts (a single error otherwise).
func joinedErrors(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		return j.Unwrap()
	}
	return []error{err}
}

// sums runs one SQL sum per daily source key.
func (a *Aggregator) sums(ctx context.Context, sources []string, from, to string) (
	aggregation.Values, error,
) {
	out := make(aggregation.Values, len(sources))
	for _, k := range sources {
		v, err := a.d.Store.SumDaily(ctx, k, from, to)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// netDays recomputes net daily rows (export - import of the stored daily rows).
func (a *Aggregator) netDays(ctx context.Context, p period.Period, plan aggregation.Plan,
	rs *runState,
) error {
	pairs := a.d.Registry.NetPairs(period.Daily)
	var keys []string
	for _, n := range pairs {
		keys = append(keys, n.Export, n.Import)
	}
	for _, j := range plan.Days {
		sums, err := a.sums(ctx, keys, j.From, j.To)
		if err != nil {
			return err
		}
		a.collect(rs, j, p.Day, aggregation.ApplyNet(pairs, sums))
	}
	return nil
}

// months recomputes every open month (and freezes the ones fully closed).
func (a *Aggregator) months(ctx context.Context, p period.Period, plan aggregation.Plan,
	rs *runState,
) error {
	for _, j := range plan.Months {
		vals, _, err := a.levelValues(ctx, period.Monthly, j)
		if err != nil {
			return err
		}
		a.collect(rs, j, p.Month, vals)
	}
	return nil
}

// years recomputes every open year; a fully closed year also folds into the baseline.
func (a *Aggregator) years(ctx context.Context, p period.Period, plan aggregation.Plan,
	rs *runState,
) error {
	for _, j := range plan.Years {
		vals, sums, err := a.levelValues(ctx, period.Yearly, j)
		if err != nil {
			return err
		}
		a.collect(rs, j, p.Year, vals)
		if j.Freeze && j.Key > rs.baseYr {
			add := aggregation.ApplyEdges(a.d.Registry.Edges(period.Total), sums)
			rs.write.Folds = append(rs.write.Folds, storage.BaselineFold{Year: j.Key, Add: add})
			rs.base, rs.baseYr = aggregation.Fold(rs.base, add), j.Key
		}
	}
	return nil
}

// levelValues computes a month or year: edge sums plus net values from the same sums.
func (a *Aggregator) levelValues(ctx context.Context, l period.Level, j aggregation.Job) (
	aggregation.Values, aggregation.Values, error,
) {
	edges := a.d.Registry.Edges(l)
	sums, err := a.sums(ctx, aggregation.SourceKeys(edges), j.From, j.To)
	if err != nil {
		return nil, nil, err
	}
	vals := aggregation.ApplyEdges(edges, sums)
	return aggregation.Merge(vals, aggregation.ApplyNet(a.d.Registry.NetPairs(l), vals)), sums,
		nil
}

// totals composes baseline + daily rows since the last folded year (never closes).
func (a *Aggregator) totals(ctx context.Context, p period.Period, _ aggregation.Plan,
	rs *runState,
) error {
	from := ""
	if rs.baseYr != "" {
		next, err := period.AddYears(rs.baseYr, 1)
		if err != nil {
			return fmt.Errorf("aggregator: baseline year: %w", err)
		}
		from = next + "-01-01"
	}
	edges := a.d.Registry.Edges(period.Total)
	since, err := a.sums(ctx, aggregation.SourceKeys(edges), from, p.Day)
	if err != nil {
		return err
	}
	vals := aggregation.Totals(edges, rs.base, since)
	vals = aggregation.Merge(vals, aggregation.ApplyNet(a.d.Registry.NetPairs(period.Total), vals))
	a.collect(rs, aggregation.Job{Level: period.Total}, "", vals)
	return nil
}

// collect adds rows (and a freeze) for a job; values of the current period also go to
// the cache.
func (a *Aggregator) collect(rs *runState, j aggregation.Job, current string,
	vals aggregation.Values,
) {
	for k, v := range vals {
		rs.write.Rows = append(rs.write.Rows, storage.PeriodRow{
			Level: j.Level, Key: k,
			Period: j.Key, Value: v,
		})
	}
	if j.Freeze {
		rs.write.Freezes = append(rs.write.Freezes, storage.Freeze{Level: j.Level, Period: j.Key})
	}
	if j.Key == current {
		for k, v := range vals {
			rs.current[k] = v
		}
	}
}

// values converts computed numbers to cache values.
func (a *Aggregator) values(vals aggregation.Values, at time.Time) map[string]*solis.Value {
	out := make(map[string]*solis.Value, len(vals))
	for k, v := range vals {
		reg, ok := a.d.Registry.ByKey(k)
		if !ok {
			continue
		}
		out[k] = &solis.Value{
			Key: k, Name: reg.Name, Unit: reg.Unit, Timestamp: at,
			RawValue: v / reg.Scale, DecodedValue: v,
		}
	}
	return out
}
