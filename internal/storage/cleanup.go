package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// vacuumInterval is the minimum time between two VACUUMs.
const vacuumInterval = 72 * time.Hour

type retentionRule struct {
	table     string
	query     string
	retention time.Duration
	layout    string // "" = compare the timestamp itself
}

func (s *Storage) retentionRules() []retentionRule {
	return []retentionRule{
		{"daily_values", "DELETE FROM daily_values WHERE date < ?", s.cfg.DailyRetention,
			period.DayLayout},
		{"monthly_values", "DELETE FROM monthly_values WHERE month < ?", s.cfg.MonthlyRetention,
			period.MonthLayout},
		{"yearly_values", "DELETE FROM yearly_values WHERE year < ?", s.cfg.YearlyRetention,
			period.YearLayout},
		{"error_data", "DELETE FROM error_data WHERE timestamp < ?", s.cfg.ErrorRetention, ""},
	}
}

// CleanupAll deletes rows older than the configured retention and VACUUMs at most every
// 72 h when something was deleted.
func (s *Storage) CleanupAll(ctx context.Context) error {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	now := s.clock.Now()
	var deleted int64
	var errs []error
	for _, r := range s.retentionRules() {
		n, err := s.cleanupTable(ctx, r, now.Add(-r.retention))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		deleted += n
	}
	s.vacuumIfNeeded(ctx, deleted, now)
	s.log.Info().Int64("rows_deleted", deleted).Int("errors", len(errs)).
		Msg("retention cleanup complete")
	return errors.Join(errs...)
}

func (s *Storage) cleanupTable(ctx context.Context, r retentionRule, cutoff time.Time) (int64,
	error) {
	var arg any = cutoff
	if r.layout != "" {
		arg = cutoff.Format(r.layout)
	}
	s.mu.Lock()
	res, err := s.db.ExecContext(ctx, r.query, arg)
	s.mu.Unlock()
	if err != nil {
		return 0, fmt.Errorf("storage: cleanup %s: %w", r.table, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("storage: cleanup %s rows affected: %w", r.table, err)
	}
	return n, nil
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
