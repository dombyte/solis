package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dombyte/solis/internal/database/migrations"
	"github.com/dombyte/solis/internal/period"
)

// Row limits keep history queries bounded.
const (
	maxErrorRows  = 50000
	maxPeriodRows = 10000
	maxYearRows   = 1000
)

// query is a named SQL statement with its arguments.
type query struct {
	what string
	sql  string
	args []any
}

// bounds is a key plus an inclusive period range formatted with layout.
type bounds struct {
	key        string
	start, end time.Time
	layout     string
	limit      int
}

// rangeQuery binds b to a `key, from, to, limit` history statement.
func rangeQuery(what, stmt string, b bounds) query {
	return query{what: what, sql: stmt,
		args: []any{b.key, b.start.Format(b.layout), b.end.Format(b.layout), b.limit}}
}

// scanRows runs q and scans every row with scan.
func scanRows[T any](ctx context.Context, db *sql.DB, q query,
	scan func(*sql.Rows) (T, error)) ([]T, error) {
	what := q.what
	rows, err := db.QueryContext(ctx, q.sql, q.args...)
	if err != nil {
		return nil, fmt.Errorf("storage: query %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]T, 0)
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan %s: %w", what, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate %s: %w", what, err)
	}
	return out, nil
}

// statusTimestamp formats t for error_data (fixed-width UTC, sortable as text).
func statusTimestamp(t time.Time) string {
	return t.UTC().Format(migrations.StatusTimestampLayout)
}

// GetErrorHistory returns the status/fault changes of key in [start, end], oldest first.
// Over the row cap the newest maxErrorRows changes are kept. CAST drops the DATETIME
// decltype so the driver returns the stored text instead of reformatting it.
func (s *Storage) GetErrorHistory(ctx context.Context, key string, start, end time.Time) (
	[]*ErrorDataPoint, error) {
	out, err := scanRows(ctx, s.db, query{what: "error history",
		sql: `SELECT CAST(timestamp AS TEXT), raw_value, string_value FROM error_data
		WHERE register_key = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC LIMIT ?`,
		args: []any{key, statusTimestamp(start), statusTimestamp(end), maxErrorRows}},
		func(r *sql.Rows) (*ErrorDataPoint, error) {
			var dp ErrorDataPoint
			var str sql.NullString
			err := r.Scan(&dp.Timestamp, &dp.RawValue, &str)
			dp.StringValue = str.String
			return &dp, err
		})
	slices.Reverse(out)
	return out, err
}

// GetDailyHistory returns the daily rows of key between the start and end days.
// substr() drops the DATE decltype so the driver returns the raw YYYY-MM-DD key.
func (s *Storage) GetDailyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*DailyDataPoint, error) {
	return scanRows(ctx, s.db, rangeQuery("daily history",
		`SELECT substr(date, 1, 10), value, raw_value FROM daily_values
		WHERE register_key = ? AND date >= ? AND date <= ? ORDER BY date LIMIT ?`,
		bounds{key, start, end, period.DayLayout, maxPeriodRows}),
		func(r *sql.Rows) (*DailyDataPoint, error) {
			var dp DailyDataPoint
			return &dp, r.Scan(&dp.Date, &dp.Value, &dp.RawValue)
		})
}

// GetMonthlyHistory returns the monthly rows of key between the start and end months.
func (s *Storage) GetMonthlyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*MonthlyDataPoint, error) {
	return scanRows(ctx, s.db, rangeQuery("monthly history",
		`SELECT month, value, raw_value FROM monthly_values
		WHERE register_key = ? AND month >= ? AND month <= ? ORDER BY month LIMIT ?`,
		bounds{key, start, end, period.MonthLayout, maxPeriodRows}),
		func(r *sql.Rows) (*MonthlyDataPoint, error) {
			var dp MonthlyDataPoint
			return &dp, r.Scan(&dp.Month, &dp.Value, &dp.RawValue)
		})
}

// GetYearlyHistory returns the yearly rows of key between the start and end years.
func (s *Storage) GetYearlyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*YearlyDataPoint, error) {
	return scanRows(ctx, s.db, rangeQuery("yearly history",
		`SELECT year, value, raw_value FROM yearly_values
		WHERE register_key = ? AND year >= ? AND year <= ? ORDER BY year LIMIT ?`,
		bounds{key, start, end, period.YearLayout, maxYearRows}),
		func(r *sql.Rows) (*YearlyDataPoint, error) {
			var dp YearlyDataPoint
			return &dp, r.Scan(&dp.Year, &dp.Value, &dp.RawValue)
		})
}

// GetTotalHistory returns the stored total of key, or nil when none exists. The
// timestamp is returned as stored (CAST drops the DATETIME decltype).
func (s *Storage) GetTotalHistory(ctx context.Context, key string) (*TotalDataPoint, error) {
	var dp TotalDataPoint
	err := s.db.QueryRowContext(ctx, `SELECT value, raw_value, CAST(timestamp AS TEXT)
		FROM total_values
		WHERE register_key = ?`, key).Scan(&dp.Value, &dp.RawValue, &dp.Timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: query total %s: %w", key, err)
	}
	return &dp, nil
}
