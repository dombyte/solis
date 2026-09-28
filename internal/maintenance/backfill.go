// Package maintenance holds out-of-band CLI jobs that run against the database with the
// app stopped (exclusive flock): `solis backfill --years N` recomputes monthly and yearly
// values with the same aggregation functions as the live aggregator, after a verified
// backup, and prints a per-period report.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/dombyte/solis/internal/aggregation"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

var (
	// ErrLocked is returned when the database is in use by the server or another job.
	ErrLocked = errors.New("database is locked by a running instance")
	// ErrInvalidArgs is returned for invalid job arguments.
	ErrInvalidArgs = errors.New("invalid arguments")
	// ErrPurgedHistory is returned when the daily rows a job needs were deleted.
	ErrPurgedHistory = errors.New("daily history incomplete")
)

// LockedError reports the lock file held by a running server or another job.
type LockedError struct {
	// Path is the lock file.
	Path string
}

func (e *LockedError) Error() string { return fmt.Sprintf("%v: %s", ErrLocked, e.Path) }

// Unwrap returns ErrLocked.
func (e *LockedError) Unwrap() error { return ErrLocked }

// ArgError reports an invalid job argument.
type ArgError struct {
	// Arg is the flag name, e.g. "--years".
	Arg string
	// Reason explains the constraint that failed.
	Reason string
}

func (e *ArgError) Error() string {
	return fmt.Sprintf("%v: %s %s", ErrInvalidArgs, e.Arg, e.Reason)
}

// Unwrap returns ErrInvalidArgs.
func (e *ArgError) Unwrap() error { return ErrInvalidArgs }

// PurgedHistoryError reports that retention deleted daily rows a job would need.
type PurgedHistoryError struct {
	// PurgedBefore is the first day that still has daily rows.
	PurgedBefore string
}

func (e *PurgedHistoryError) Error() string {
	return fmt.Sprintf("%v: daily rows before %s were removed by retention "+
		"(storage.daily_retention); closed years can no longer be recomputed",
		ErrPurgedHistory, e.PurgedBefore)
}

// Unwrap returns ErrPurgedHistory.
func (e *PurgedHistoryError) Unwrap() error { return ErrPurgedHistory }

// Store runs a backfill transaction.
type Store interface {
	Backfill(ctx context.Context, fn func(storage.BackfillTx) error) error
}

// Registry provides the computation definitions.
type Registry interface {
	ByKey(key string) (solis.Register, bool)
	Edges(l period.Level) []solis.Edge
	NetPairs(l period.Level) []solis.NetPair
}

// Env wires a backfill run. OpenStore is called only after the lock and the backup.
type Env struct {
	DBPath    string
	Backup    func(ctx context.Context) (string, error)
	OpenStore func(ctx context.Context) (Store, func() error, error)
	Registry  Registry
	Now       time.Time
	Out       io.Writer
}

// Options select what a backfill recomputes.
type Options struct {
	// Years is the number of closed years recomputed in addition to the current year.
	Years int
	// Force recomputes every period, even one that starts before the oldest daily row
	// (writing partial or zero sums over the stored value), and refreshes the total
	// baseline from all daily rows, pre-cutover years included. It is the pre-guard
	// behaviour for a deliberate override; periods removed by retention (purged_before)
	// are still refused.
	Force bool
}

// RunBackfill locks the database, takes a verified backup, recomputes monthly and yearly
// values for the current year plus opts.Years closed years (refreshing the total baseline
// when a closed year is touched) and prints the report. No backup, no write.
func RunBackfill(ctx context.Context, env Env, opts Options) (err error) {
	if opts.Years < 0 {
		return &ArgError{Arg: "--years", Reason: fmt.Sprintf("must be >= 0, got %d", opts.Years)}
	}
	lock, err := AcquireExclusive(env.DBPath)
	if err != nil {
		return fmt.Errorf("refusing to run: %w (stop the app first)", err)
	}
	defer func() { err = errors.Join(err, lock.Release()) }()

	backup, err := env.Backup(ctx)
	if err != nil {
		return fmt.Errorf("backup failed, nothing was written: %w", err)
	}
	if err := writeHeader(env.Out, backup, opts.Force); err != nil {
		return err
	}
	st, closeStore, err := env.OpenStore(ctx)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { err = errors.Join(err, closeStore()) }()

	var rep Report
	err = st.Backfill(ctx, func(tx storage.BackfillTx) error {
		var rErr error
		rep, rErr = Recompute(tx, env.Registry, period.Of(env.Now), opts)
		return rErr
	})
	if err != nil {
		return fmt.Errorf("backfill failed, rolled back: %w", err)
	}
	return rep.Write(env.Out)
}

// writeHeader prints the backup path and, with --force, what force changes.
func writeHeader(out io.Writer, backup string, force bool) error {
	if _, err := fmt.Fprintf(out, "backup: %s\n", backup); err != nil {
		return err
	}
	if !force {
		return nil
	}
	_, err := fmt.Fprintln(out, "force: periods without complete daily history are "+
		"overwritten; the baseline includes all daily history")
	return err
}

// Recompute rewrites monthly and yearly rows (always both) of the affected years and, if a
// closed year is included, the total baseline. Periods that start before the oldest
// daily row have no complete daily history: they are skipped and keep their stored
// (e.g. inverter-reported) value instead of being overwritten with a partial or zero
// sum, unless opts.Force. It returns the report lines.
func Recompute(tx storage.BackfillTx, reg Registry, now period.Period, opts Options) (
	Report, error,
) {
	if err := checkPurged(tx, now, opts.Years); err != nil {
		return Report{}, err
	}
	first, err := tx.FirstDailyDay()
	if err != nil {
		return Report{}, err
	}
	r := &recomputer{tx: tx, reg: reg, now: now, first: first, force: opts.Force}
	err = r.run(opts.Years)
	return r.rep, err
}

// run recomputes the current year plus `years` closed years, then the baseline.
func (r *recomputer) run(years int) error {
	for back := years; back >= 0; back-- {
		year, err := period.AddYears(r.now.Year, -back)
		if err != nil {
			return err
		}
		if err := r.year(year); err != nil {
			return err
		}
	}
	if years > 0 {
		return r.refreshBaseline()
	}
	return nil
}

// checkPurged refuses a job that would recompute from daily rows retention already
// deleted: the oldest touched year must start on or after the purge watermark, and the
// baseline (refreshed when a closed year is touched) needs the complete history.
func checkPurged(tx storage.BackfillTx, now period.Period, years int) error {
	purged := tx.PurgedBefore()
	if purged == "" {
		return nil
	}
	oldest, err := period.AddYears(now.Year, -years)
	if err != nil {
		return err
	}
	if years > 0 || oldest+"-01-01" < purged {
		return &PurgedHistoryError{PurgedBefore: purged}
	}
	return nil
}

// recomputer carries one Recompute run.
type recomputer struct {
	tx    storage.BackfillTx
	reg   Registry
	now   period.Period
	first string // oldest daily row ("" = none)
	force bool   // Options.Force
	rep   Report
}

func (r *recomputer) year(year string) error {
	months, err := monthsOf(year, r.now)
	if err != nil {
		return err
	}
	for _, m := range months {
		first, last, err := period.MonthBounds(m)
		if err != nil {
			return err
		}
		j := job{level: period.Monthly, key: m, from: first, to: min(last, r.now.Day)}
		if err := r.period(j); err != nil {
			return err
		}
	}
	first, last, err := period.YearBounds(year)
	if err != nil {
		return err
	}
	return r.period(job{
		level: period.Yearly, key: year, from: first,
		to: min(last, r.now.Day),
	})
}

// monthsOf lists the months of year up to the current month.
func monthsOf(year string, now period.Period) ([]string, error) {
	prev, err := period.AddMonths(year+"-01", -1)
	if err != nil {
		return nil, err
	}
	through := year + "-12"
	if now.Month < through {
		through = now.Month
	}
	return period.MonthsBetween(prev, through)
}

// job is one period to recompute, summing daily rows from..to.
type job struct {
	level         period.Level
	key, from, to string
}

// period computes edges + net from daily sums and writes every row, unless the daily
// history does not reach back to the period's start (review DB-H1).
func (r *recomputer) period(j job) error {
	if r.incomplete(j) {
		r.rep.skip(Skip{Level: j.level.String(), Period: j.key, FirstDaily: r.first})
		return nil
	}
	l, p := j.level, j.key
	edges := r.reg.Edges(l)
	sums, err := r.sums(aggregation.SourceKeys(edges), j.from, j.to)
	if err != nil {
		return err
	}
	vals := aggregation.ApplyEdges(edges, sums)
	vals = aggregation.Merge(vals, aggregation.ApplyNet(r.reg.NetPairs(l), vals))
	for _, key := range sortedKeys(vals) {
		old, had, err := r.tx.PeriodValue(l, key, p)
		if err != nil {
			return err
		}
		if err := r.tx.PutPeriod(l, key, p, vals[key]); err != nil {
			return err
		}
		r.rep.add(Line{
			Level: l.String(), Key: key, Period: p, Old: old, HadOld: had,
			New: vals[key], Unit: unitOf(r.reg, key),
		})
	}
	return nil
}

// incomplete reports a period whose start lies before the oldest daily row, which is
// skipped unless Force (review DB-H1).
func (r *recomputer) incomplete(j job) bool {
	return !r.force && (r.first == "" || j.from < r.first)
}

// sums runs one bounded daily sum per source key.
func (r *recomputer) sums(keys []string, from, to string) (aggregation.Values, error) {
	sums := aggregation.Values{}
	for _, k := range keys {
		v, err := r.tx.SumDaily(k, from, to)
		if err != nil {
			return nil, err
		}
		sums[k] = v
	}
	return sums, nil
}

// refreshBaseline recomputes the total baseline the way the live aggregator builds it:
// the daily rows of every folded year since the cutover year (totals are app-lifetime,
// review DB-H2). Pre-cutover years are never added, and while no year since the cutover
// has been folded there is nothing to refresh.
func (r *recomputer) refreshBaseline() error {
	year, old := r.tx.Baseline()
	from, ok := r.baselineFrom(year)
	if !ok {
		return nil
	}
	edges := r.reg.Edges(period.Total)
	sums, err := r.sums(aggregation.SourceKeys(edges), from, year+"-12-31")
	if err != nil {
		return err
	}
	next := aggregation.ApplyEdges(edges, sums)
	for _, key := range sortedKeys(next) {
		o, had := old[key]
		r.rep.add(Line{
			Level: "total", Key: key, Period: "baseline", Old: o, HadOld: had,
			New: next[key], Unit: unitOf(r.reg, key),
		})
	}
	return r.tx.PutBaseline(next)
}

// baselineFrom is the first day summed into the baseline: the cutover year's 1 January, or
// the start of all history with Force (then pre-cutover years are included). false means
// there is nothing to refresh.
func (r *recomputer) baselineFrom(year string) (string, bool) {
	if year == "" {
		return "", false
	}
	if r.force {
		return "", true
	}
	cutover := r.tx.Cutover()
	if len(cutover) < len("2006") || year < cutover[:4] {
		return "", false
	}
	return cutover[:4] + "-01-01", true
}

func unitOf(reg Registry, key string) string {
	r, _ := reg.ByKey(key)
	return r.Unit
}
