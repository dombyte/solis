package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/dombyte/solis/internal/solis"
)

const writerPoller = "poller"

// WritePoll persists one poll cycle atomically: daily max-writes, status changes and
// day closes. Any row (daily, status or close) violating the write domain, a closed day
// or naming an unknown key is skipped and reported in the returned (joined) error; the
// rest of the poll is still committed, so a rejection never rolls back the other rows
// (review AGG-M1). Only a storage failure returns a non-rejection error and commits
// nothing.
func (s *Storage) WritePoll(ctx context.Context, w PollWrite) error {
	s.log.Debug().Int("daily", len(w.Daily)).Int("status", len(w.Status)).
		Int("closes", len(w.Close)).Msg("write poll starting")
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.meta.clone()
	var rejected []error
	err := s.withTx(ctx, func(tx *txn) error {
		var err error
		rejected, err = s.applyPoll(tx, &next, w)
		return err
	})
	if err != nil {
		return err
	}
	s.meta = next
	if len(rejected) > 0 {
		s.log.Debug().Int("rejected", len(rejected)).Msg("write poll completed with rejections")
	} else {
		s.log.Debug().Msg("write poll completed")
	}
	return errors.Join(rejected...)
}

// applyPoll writes every row of w inside tx and returns the rejected rows.
func (s *Storage) applyPoll(tx *txn, next *metaState, w PollWrite) ([]error, error) {
	var rejected []error
	for _, r := range w.Daily {
		if err := collect(&rejected, s.writeDaily(tx, next, r)); err != nil {
			return nil, err
		}
	}
	for _, r := range w.Status {
		if err := collect(&rejected, s.writeStatus(tx, r)); err != nil {
			return nil, err
		}
	}
	for _, c := range w.Close {
		if err := collect(&rejected, s.closeDay(tx, next, c)); err != nil {
			return nil, err
		}
	}
	return rejected, nil
}

// collect records a rejected row and returns any other (storage) error.
func collect(rejected *[]error, err error) error {
	if err != nil && IsRejection(err) {
		*rejected = append(*rejected, err)
		return nil
	}
	return err
}

// IsRejection reports whether err only carries rejected rows (closed period, foreign
// write domain, unknown key) rather than a storage failure.
func IsRejection(err error) bool {
	return errors.Is(err, ErrPeriodClosed) || errors.Is(err, ErrWriteDomain) ||
		errors.Is(err, ErrUnknownKey)
}

func (s *Storage) writeDaily(tx *txn, m *metaState, r DailyRow) error {
	reg, err := s.lookup(r.Key)
	if err != nil {
		return err
	}
	if reg.Store != solis.StoreDaily || reg.Net {
		return &WriteDomainError{
			Writer: writerPoller, Key: r.Key,
			Reason: "only non-net daily registers belong to the poller",
		}
	}
	if closed := m.closedDaily[r.Key]; r.Day <= closed {
		return &PeriodClosedError{Level: "daily", Key: r.Key, Period: r.Day, ClosedThrough: closed}
	}
	_, err = tx.ExecContext(tx.ctx, `INSERT INTO daily_values (date, register_key, value, raw_value)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(register_key, date) DO UPDATE
		SET value = excluded.value, raw_value = excluded.raw_value
		WHERE excluded.value > daily_values.value`, r.Day, r.Key, r.Value, r.Raw)
	if err != nil {
		return fmt.Errorf("storage: write daily %s %s: %w", r.Key, r.Day, err)
	}
	return nil
}

func (s *Storage) writeStatus(tx *txn, r StatusRow) error {
	reg, err := s.lookup(r.Key)
	if err != nil {
		return err
	}
	if reg.Store != solis.StoreStatus {
		return &WriteDomainError{Writer: writerPoller, Key: r.Key, Reason: "not a status register"}
	}
	// Two changes of one key within a millisecond keep the later value instead of
	// failing the whole poll transaction on UNIQUE(register_key, timestamp).
	if _, err := tx.ExecContext(tx.ctx, `INSERT INTO error_data (timestamp, register_key, raw_value,
		string_value) VALUES (?, ?, ?, '')
		ON CONFLICT(register_key, timestamp) DO UPDATE SET raw_value = excluded.raw_value`,
		statusTimestamp(r.At), r.Key, r.Raw); err != nil {
		return fmt.Errorf("storage: write status %s: %w", r.Key, err)
	}
	return nil
}

// closeDay advances the closed-day watermark of one poller-owned (non-net daily) key.
func (s *Storage) closeDay(tx *txn, m *metaState, c DayClose) error {
	reg, err := s.lookup(c.Key)
	if err != nil {
		return err
	}
	if reg.Store != solis.StoreDaily || reg.Net {
		return &WriteDomainError{
			Writer: writerPoller, Key: c.Key,
			Reason: "only non-net daily registers have poller day closes",
		}
	}
	v, err := advance(tx, metaClosedDaily+c.Key, m.closedDaily[c.Key], c.Day)
	if err != nil {
		return err
	}
	m.closedDaily[c.Key] = v
	return nil
}

// Seed loads the stored rows of the given days, the last status values and the
// closed-day watermarks.
func (s *Storage) Seed(ctx context.Context, days []string) (Seed, error) {
	daily, err := s.dailyRows(ctx, days)
	if err != nil {
		return Seed{}, err
	}
	status, err := s.lastStatus(ctx)
	if err != nil {
		return Seed{}, err
	}
	s.mu.Lock()
	closed := make(map[string]string, len(s.meta.closedDaily))
	for k, v := range s.meta.closedDaily {
		closed[k] = v
	}
	s.mu.Unlock()
	return Seed{Daily: daily, Status: status, Closed: closed}, nil
}

// dailyRows loads the rows of the requested (consecutive) days in one range query.
func (s *Storage) dailyRows(ctx context.Context, days []string) (map[string]map[string]float64,
	error,
) {
	out := make(map[string]map[string]float64, len(days))
	if len(days) == 0 {
		return out, nil
	}
	lo, hi := days[0], days[0]
	for _, d := range days {
		out[d] = make(map[string]float64)
		lo, hi = min(lo, d), max(hi, d)
	}
	// substr() drops the DATE decltype so the driver returns the raw YYYY-MM-DD key.
	rows, err := s.db.QueryContext(ctx, `SELECT substr(date, 1, 10), register_key, value
		FROM daily_values WHERE date >= ? AND date <= ?`, lo, hi)
	if err != nil {
		return nil, fmt.Errorf("storage: seed daily: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var day, key string
		var v float64
		if err := rows.Scan(&day, &key, &v); err != nil {
			return nil, fmt.Errorf("storage: seed daily: %w", err)
		}
		if m, ok := out[day]; ok {
			m[key] = v
		}
	}
	return out, rows.Err()
}

func (s *Storage) lastStatus(ctx context.Context) (map[string]float64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.register_key, e.raw_value FROM error_data e
		JOIN (SELECT register_key, MAX(id) AS id FROM error_data GROUP BY register_key) l
		ON e.id = l.id`)
	if err != nil {
		return nil, fmt.Errorf("storage: seed status: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]float64)
	for rows.Next() {
		var key string
		var v float64
		if err := rows.Scan(&key, &v); err != nil {
			return nil, fmt.Errorf("storage: seed status: %w", err)
		}
		out[key] = v
	}
	return out, rows.Err()
}
