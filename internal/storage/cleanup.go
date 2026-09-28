package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// vacuumInterval is the minimum time between two VACUUMs.
const vacuumInterval = 72 * time.Hour

// CleanupAll deletes rows older than the configured retention and VACUUMs at most every
// 72 h when something was deleted.
//
// Daily, monthly and yearly rows share daily_retention, but only periods that can never
// be recomputed again are deleted: everything before the first open day/month/year of
// the freeze watermarks and the total baseline. Computed values therefore never shrink.
// The earliest kept day is recorded as the purge watermark so `solis backfill` refuses
// to recompute periods whose daily rows are gone.
func (s *Storage) CleanupAll(ctx context.Context) error {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	now := s.clock.Now()
	var errs []error
	deleted, err := s.cleanupPeriods(ctx, now.Add(-s.cfg.DailyRetention).Format(period.DayLayout))
	if err != nil {
		errs = append(errs, err)
	}
	n, err := s.cleanupStatus(ctx, now.Add(-s.cfg.ErrorRetention))
	if err != nil {
		errs = append(errs, err)
	}
	deleted += n
	s.vacuumIfNeeded(ctx, deleted, now)
	s.log.Info().Int64("rows_deleted", deleted).Int("errors", len(errs)).
		Msg("retention cleanup complete")
	return errors.Join(errs...)
}

// cleanupPeriods deletes daily/monthly/yearly rows before min(wanted, first open day) and
// advances the purge watermark in the same transaction.
func (s *Storage) cleanupPeriods(ctx context.Context, wanted string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit, err := s.firstOpenDay()
	if err != nil || limit == "" {
		return 0, err
	}
	cutoff := min(wanted, limit)
	if cutoff <= s.meta.purgedBefore {
		return 0, nil
	}
	month, year := cutoff[:len(period.MonthLayout)], cutoff[:len(period.YearLayout)]
	stmts := []struct{ table, sql, arg string }{
		{"daily_values", `DELETE FROM daily_values WHERE date < ?`, cutoff},
		{"monthly_values", `DELETE FROM monthly_values WHERE month < ?`, month},
		{"yearly_values", `DELETE FROM yearly_values WHERE year < ?`, year},
	}
	var deleted int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		for _, st := range stmts {
			n, err := execCount(ctx, tx, st.sql, st.arg)
			if err != nil {
				return fmt.Errorf("storage: cleanup %s: %w", st.table, err)
			}
			deleted += n
		}
		return putMeta(tx, metaPurgedBefore, cutoff)
	})
	if err != nil {
		return 0, err
	}
	s.meta.purgedBefore = cutoff
	return deleted, nil
}

// firstOpenDay is the earliest day any computed value may still be recomputed from:
// the day after the last frozen net day, month and year and after the baseline year.
// "" (nothing frozen yet) means nothing may be deleted. Caller holds mu.
func (s *Storage) firstOpenDay() (string, error) {
	type mark struct {
		value string
		next  func(string) (string, error)
	}
	marks := []mark{
		{s.meta.frozen[period.Daily], func(d string) (string, error) {
			return period.AddDays(d, 1)
		}},
		{s.meta.frozen[period.Monthly], func(m string) (string, error) {
			next, err := period.AddMonths(m, 1)
			return next + "-01", err
		}},
		{s.meta.frozen[period.Yearly], nextYearStart},
		{s.meta.baselineYear, nextYearStart},
	}
	first := ""
	for i, m := range marks {
		if m.value == "" {
			return "", nil
		}
		day, err := m.next(m.value)
		if err != nil {
			return "", err
		}
		if i == 0 || day < first {
			first = day
		}
	}
	return first, nil
}

func nextYearStart(y string) (string, error) {
	next, err := period.AddYears(y, 1)
	return next + "-01-01", err
}

// cleanupStatus deletes error_data rows older than cutoff.
func (s *Storage) cleanupStatus(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := execCount(ctx, s.db, `DELETE FROM error_data WHERE timestamp < ?`,
		statusTimestamp(cutoff))
	if err != nil {
		return 0, fmt.Errorf("storage: cleanup error_data: %w", err)
	}
	return n, nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// execCount runs a statement and returns the number of affected rows.
func execCount(ctx context.Context, db execer, stmt string, args ...any) (int64, error) {
	res, err := db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Storage) vacuumIfNeeded(ctx context.Context, deleted int64, now time.Time) {
	if deleted == 0 || (!s.lastVacuum.IsZero() && now.Sub(s.lastVacuum) <= vacuumInterval) {
		return
	}
	s.mu.Lock()
	_, err := s.db.ExecContext(ctx, "VACUUM;")
	s.mu.Unlock()
	if err != nil {
		s.log.Warn().Err(err).Msg("VACUUM failed")
		return
	}
	s.lastVacuum = now
}
