package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
)

const writerAggregator = "aggregator"

// HasDailyData reports whether any daily row exists.
func (s *Storage) HasDailyData(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM daily_values)`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("storage: has daily data: %w", err)
	}
	return n == 1, nil
}

// SumDaily sums the daily values of key with from <= date <= to. The upper bound is
// explicit (the open day) and also guards against stale future-dated rows.
func (s *Storage) SumDaily(ctx context.Context, key, from, to string) (float64, error) {
	return sumDaily(ctx, s.db, key, from, to)
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func sumDaily(ctx context.Context, q queryer, key, from, to string) (float64, error) {
	var sum float64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(value), 0) FROM daily_values
		WHERE register_key = ? AND date >= ? AND date <= ?`, key, from, to).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("storage: sum daily %s: %w", key, err)
	}
	return sum, nil
}

// WriteComputed applies one aggregator run in a single transaction: REPLACE upserts for
// open periods, then freezes, then the baseline fold. Rows targeting closed periods or
// keys outside the aggregator domain are skipped and reported in the joined error.
func (s *Storage) WriteComputed(ctx context.Context, w ComputedWrite) error {
	s.log.Debug().Int("rows", len(w.Rows)).Int("freezes", len(w.Freezes)).
		Int("folds", len(w.Folds)).Msg("write computed starting")
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.meta.clone()
	var rejected []error
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		rejected, err = s.applyComputed(tx, &next, w)
		return err
	})
	if err != nil {
		return err
	}
	s.meta = next
	s.log.Debug().Int("rejected", len(rejected)).Msg("write computed completed")
	return errors.Join(rejected...)
}

// applyComputed writes rows, freezes and folds inside tx and returns the rejected rows.
func (s *Storage) applyComputed(tx *sql.Tx, next *metaState, w ComputedWrite) (
	[]error, error) {
	var rejected []error
	for _, r := range w.Rows {
		if err := s.writeComputedRow(tx, *next, r, w.At); err != nil {
			if !IsRejection(err) {
				return nil, err
			}
			rejected = append(rejected, err)
		}
	}
	if err := applyFreezes(tx, next, w.Freezes); err != nil {
		return nil, err
	}
	for _, f := range w.Folds {
		if err := applyFold(tx, next, f); err != nil {
			return nil, err
		}
	}
	return rejected, nil
}

func (s *Storage) writeComputedRow(tx *sql.Tx, m metaState, r PeriodRow, at time.Time) error {
	reg, err := s.lookup(r.Key)
	if err != nil {
		return err
	}
	if err := checkAggregatorDomain(reg, r.Level); err != nil {
		return err
	}
	if r.Level != period.Total {
		if closed := m.frozen[r.Level]; r.Period <= closed {
			return &PeriodClosedError{Level: r.Level.String(), Key: r.Key, Period: r.Period,
				ClosedThrough: closed}
		}
	}
	return upsertPeriod(tx, reg, r, at)
}

// checkAggregatorDomain allows computed monthly/yearly/total keys and net daily keys.
func checkAggregatorDomain(reg solis.Register, l period.Level) error {
	if reg.Store != solis.StoreForLevel(l) {
		return &WriteDomainError{Writer: writerAggregator, Key: reg.Key,
			Reason: "store does not match level " + l.String()}
	}
	if !reg.Computed() || (l == period.Daily && !reg.Net) {
		return &WriteDomainError{Writer: writerAggregator, Key: reg.Key,
			Reason: "not a computed register"}
	}
	return nil
}

// upsertPeriod writes one row with REPLACE semantics (latest computation wins).
func upsertPeriod(tx *sql.Tx, reg solis.Register, r PeriodRow, at time.Time) error {
	raw := rawOf(reg, r.Value)
	var err error
	switch r.Level {
	case period.Daily:
		_, err = tx.Exec(`INSERT INTO daily_values (date, register_key, value, raw_value)
			VALUES (?, ?, ?, ?) ON CONFLICT(register_key, date) DO UPDATE
			SET value = excluded.value, raw_value = excluded.raw_value`,
			r.Period, r.Key, r.Value, raw)
	case period.Monthly:
		_, err = tx.Exec(`INSERT INTO monthly_values (month, register_key, value, raw_value)
			VALUES (?, ?, ?, ?) ON CONFLICT(register_key, month) DO UPDATE
			SET value = excluded.value, raw_value = excluded.raw_value`,
			r.Period, r.Key, r.Value, raw)
	case period.Yearly:
		_, err = tx.Exec(`INSERT INTO yearly_values (year, register_key, value, raw_value)
			VALUES (?, ?, ?, ?) ON CONFLICT(register_key, year) DO UPDATE
			SET value = excluded.value, raw_value = excluded.raw_value`,
			r.Period, r.Key, r.Value, raw)
	default:
		_, err = tx.Exec(`INSERT INTO total_values (register_key, value, raw_value, timestamp)
			VALUES (?, ?, ?, ?) ON CONFLICT(register_key) DO UPDATE
			SET value = excluded.value, raw_value = excluded.raw_value,
			timestamp = excluded.timestamp`, r.Key, r.Value, raw, at.Format(time.RFC3339))
	}
	if err != nil {
		return fmt.Errorf("storage: upsert %s %s %s: %w", r.Level, r.Key, r.Period, err)
	}
	return nil
}

func applyFreezes(tx *sql.Tx, m *metaState, freezes []Freeze) error {
	for _, f := range freezes {
		k, err := frozenKey(f.Level)
		if err != nil {
			return err
		}
		if m.frozen[f.Level], err = advance(tx, k, m.frozen[f.Level], f.Period); err != nil {
			return err
		}
	}
	return nil
}

// applyFold adds a closed year to the baseline once (years <= baseline_year are no-ops).
func applyFold(tx *sql.Tx, m *metaState, f BaselineFold) error {
	if f.Year <= m.baselineYear {
		return nil
	}
	for k, add := range f.Add {
		m.baseline[k] += add
		if err := putMeta(tx, metaBaseline+k, formatFloat(m.baseline[k])); err != nil {
			return err
		}
	}
	m.baselineYear = f.Year
	return putMeta(tx, metaBaselineYear, f.Year)
}
