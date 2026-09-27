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

	"github.com/rs/zerolog"

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
)

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
	Backup    func() (string, error)
	OpenStore func() (Store, func() error, error)
	Registry  Registry
	Now       time.Time
	Out       io.Writer
	Log       zerolog.Logger
}

// RunBackfill locks the database, takes a verified backup, recomputes monthly and yearly
// values for the current year plus `years` closed years (refreshing the total baseline
// when a closed year is touched) and prints the report. No backup, no write.
func RunBackfill(ctx context.Context, env Env, years int) (err error) {
	if years < 0 {
		return fmt.Errorf("%w: --years must be >= 0, got %d", ErrInvalidArgs, years)
	}
	lock, err := AcquireExclusive(env.DBPath)
	if err != nil {
		return fmt.Errorf("refusing to run: %w (stop the app first)", err)
	}
	defer func() { err = errors.Join(err, lock.Release()) }()

	backup, err := env.Backup()
	if err != nil {
		return fmt.Errorf("backup failed, nothing was written: %w", err)
	}
	if _, err := fmt.Fprintf(env.Out, "backup: %s\n", backup); err != nil {
		return err
	}
	st, closeStore, err := env.OpenStore()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { err = errors.Join(err, closeStore()) }()

	var rep Report
	err = st.Backfill(ctx, func(tx storage.BackfillTx) error {
		var rErr error
		rep, rErr = Recompute(tx, env.Registry, period.Of(env.Now), years)
		return rErr
	})
	if err != nil {
		return fmt.Errorf("backfill failed, rolled back: %w", err)
	}
	return rep.Write(env.Out)
}

// Recompute rewrites monthly and yearly rows (always both) of the affected years and, if a
// closed year is included, the total baseline. It returns the report lines.
func Recompute(tx storage.BackfillTx, reg Registry, now period.Period, years int) (
	Report, error) {
	var rep Report
	for back := years; back >= 0; back-- {
		year, err := period.AddYears(now.Year, -back)
		if err != nil {
			return rep, err
		}
		if err := recomputeYear(tx, reg, now, year, &rep); err != nil {
			return rep, err
		}
	}
	if years > 0 {
		if err := refreshBaseline(tx, reg, &rep); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func recomputeYear(tx storage.BackfillTx, reg Registry, now period.Period, year string,
	rep *Report) error {
	months, err := monthsOf(year, now)
	if err != nil {
		return err
	}
	for _, m := range months {
		first, last, err := period.MonthBounds(m)
		if err != nil {
			return err
		}
		j := job{level: period.Monthly, key: m, from: first, to: min(last, now.Day)}
		if err := recomputePeriod(tx, reg, j, rep); err != nil {
			return err
		}
	}
	first, last, err := period.YearBounds(year)
	if err != nil {
		return err
	}
	return recomputePeriod(tx, reg, job{level: period.Yearly, key: year, from: first,
		to: min(last, now.Day)}, rep)
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

// recomputePeriod computes edges + net from daily sums and writes every row.
func recomputePeriod(tx storage.BackfillTx, reg Registry, j job, rep *Report) error {
	l, p := j.level, j.key
	edges := reg.Edges(l)
	sums := aggregation.Values{}
	for _, k := range aggregation.SourceKeys(edges) {
		v, err := tx.SumDaily(k, j.from, j.to)
		if err != nil {
			return err
		}
		sums[k] = v
	}
	vals := aggregation.ApplyEdges(edges, sums)
	vals = aggregation.Merge(vals, aggregation.ApplyNet(reg.NetPairs(l), vals))
	for _, key := range sortedKeys(vals) {
		old, had, err := tx.PeriodValue(l, key, p)
		if err != nil {
			return err
		}
		if err := tx.PutPeriod(l, key, p, vals[key]); err != nil {
			return err
		}
		rep.add(Line{Level: l.String(), Key: key, Period: p, Old: old, HadOld: had,
			New: vals[key], Unit: unitOf(reg, key)})
	}
	return nil
}

// refreshBaseline recomputes baseline = sum of all daily rows up to the baseline year.
func refreshBaseline(tx storage.BackfillTx, reg Registry, rep *Report) error {
	year, old := tx.Baseline()
	if year == "" {
		return nil
	}
	edges := reg.Edges(period.Total)
	sums := aggregation.Values{}
	for _, k := range aggregation.SourceKeys(edges) {
		v, err := tx.SumDaily(k, "", year+"-12-31")
		if err != nil {
			return err
		}
		sums[k] = v
	}
	next := aggregation.ApplyEdges(edges, sums)
	for _, key := range sortedKeys(next) {
		o, had := old[key]
		rep.add(Line{Level: "total", Key: key, Period: "baseline", Old: o, HadOld: had,
			New: next[key], Unit: unitOf(reg, key)})
	}
	return tx.PutBaseline(next)
}

func unitOf(reg Registry, key string) string {
	r, _ := reg.ByKey(key)
	return r.Unit
}
