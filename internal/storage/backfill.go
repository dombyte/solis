package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"

	"github.com/dombyte/solis/internal/period"
)

// BackfillTx is the maintenance view of one backfill transaction. Writes bypass the
// frozen watermarks ("unfreezing only inside the job's own transaction"); nothing is
// persisted unless the job function returns nil.
type BackfillTx interface {
	// SumDaily sums daily values of key for from <= date <= to.
	SumDaily(key, from, to string) (float64, error)
	// PeriodValue returns the stored value of a monthly/yearly row.
	PeriodValue(l period.Level, key, p string) (float64, bool, error)
	// PutPeriod writes a monthly/yearly row, ignoring freeze marks.
	PutPeriod(l period.Level, key, p string, v float64) error
	// Baseline returns the baseline year and values.
	Baseline() (string, map[string]float64)
	// PutBaseline replaces the baseline values (year unchanged).
	PutBaseline(values map[string]float64) error
	// PurgedBefore is the earliest day retention cleanup kept ("" = nothing deleted);
	// daily rows before it are gone, so periods starting earlier cannot be recomputed.
	PurgedBefore() string
	// FirstDailyDay is the oldest stored daily row ("" = none). Periods starting
	// before it have no complete daily history (v2 retention, or logging started later).
	FirstDailyDay() (string, error)
	// Cutover is the v3 cutover day ("" = not recorded yet).
	Cutover() string
}

// Backfill runs fn in one transaction and commits only when fn returns nil.
func (s *Storage) Backfill(ctx context.Context, fn func(BackfillTx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.meta.clone()
	err := s.withTx(ctx, func(tx *txn) error {
		return fn(&backfillTx{s: s, ctx: ctx, tx: tx, meta: &next})
	})
	if err != nil {
		return err
	}
	s.meta = next
	return nil
}

type backfillTx struct {
	s    *Storage
	ctx  context.Context
	tx   *txn
	meta *metaState
}

func (b *backfillTx) SumDaily(key, from, to string) (float64, error) {
	return sumDaily(b.ctx, b.tx, key, from, to)
}

func (b *backfillTx) PeriodValue(l period.Level, key, p string) (float64, bool, error) {
	return periodValue(b.ctx, b.tx, l, key, p)
}

// periodValue reads a stored monthly/yearly row (false if there is none).
func periodValue(ctx context.Context, q queryer, l period.Level, key, p string) (
	float64, bool, error,
) {
	var stmt string
	switch l {
	case period.Monthly:
		stmt = `SELECT value FROM monthly_values WHERE register_key = ? AND month = ?`
	case period.Yearly:
		stmt = `SELECT value FROM yearly_values WHERE register_key = ? AND year = ?`
	default:
		return 0, false, fmt.Errorf("storage: period level %s not supported", l)
	}
	var v float64
	err := q.QueryRowContext(ctx, stmt, key, p).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("storage: read %s %s: %w", key, p, err)
	}
	return v, true, nil
}

func (b *backfillTx) PutPeriod(l period.Level, key, p string, v float64) error {
	reg, err := b.s.lookup(key)
	if err != nil {
		return err
	}
	if l != period.Monthly && l != period.Yearly {
		return fmt.Errorf("storage: backfill level %s not supported", l)
	}
	if err := checkAggregatorDomain(reg, l); err != nil {
		return err
	}
	if err := b.clearOffset(OffsetKey(l, p, key)); err != nil {
		return err
	}
	return upsertPeriod(b.tx, reg, PeriodRow{Level: l, Key: key, Period: p, Value: v},
		b.s.clock.Now())
}

// clearOffset drops a cutover offset: a backfilled period is the pure daily sum, and the
// aggregator must not add the inverter offset on top again.
func (b *backfillTx) clearOffset(key string) error {
	if _, ok := b.meta.offsets[key]; !ok {
		return nil
	}
	if _, err := b.tx.ExecContext(b.ctx, `DELETE FROM meta WHERE key = ?`,
		metaOffset+key); err != nil {
		return fmt.Errorf("storage: clear offset %s: %w", key, err)
	}
	delete(b.meta.offsets, key)
	return nil
}

func (b *backfillTx) Baseline() (string, map[string]float64) {
	return b.meta.baselineYear, maps.Clone(b.meta.baseline)
}

func (b *backfillTx) PurgedBefore() string { return b.meta.purgedBefore }

func (b *backfillTx) Cutover() string { return b.meta.cutover }

func (b *backfillTx) FirstDailyDay() (string, error) {
	var first string
	err := b.tx.QueryRowContext(b.ctx,
		`SELECT COALESCE(MIN(date), '') FROM daily_values`).Scan(&first)
	if err != nil {
		return "", fmt.Errorf("storage: first daily day: %w", err)
	}
	return first, nil
}

func (b *backfillTx) PutBaseline(values map[string]float64) error {
	for k, v := range values {
		if err := putMeta(b.tx, metaBaseline+k, formatFloat(v)); err != nil {
			return err
		}
		b.meta.baseline[k] = v
	}
	return nil
}
