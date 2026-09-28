package migrations

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// StatusTimestampLayout is the fixed-width UTC layout of error_data.timestamp since V4.
// Fixed width keeps string comparison (range filters, ORDER BY, retention) in time
// order; storage writes and binds every error_data timestamp with it.
const StatusTimestampLayout = period.TimestampLayout

// legacyTimestampLayouts are the formats found in error_data before V4: Go's
// time.Time.String() (bound directly by v3 before the fix), RFC 3339 and SQLite's
// CURRENT_TIMESTAMP.
func legacyTimestampLayouts() []string {
	return []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
	}
}

// V4Migration rewrites error_data timestamps into StatusTimestampLayout (UTC).
type V4Migration struct{}

// Version returns the migration version (4).
func (m *V4Migration) Version() int { return 4 }

// Description returns a description of what this migration does.
func (m *V4Migration) Description() string {
	return "normalize error_data timestamps to fixed-width UTC"
}

// Up rewrites every parseable error_data timestamp; unparseable values stay untouched.
func (m *V4Migration) Up(tx *sql.Tx) error {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'error_data'`).Scan(&n); err != nil {
		return fmt.Errorf("check error_data table: %w", err)
	}
	if n == 0 {
		return nil
	}
	rows, err := readStatusTimestamps(tx)
	if err != nil {
		return err
	}
	for id, ts := range rows {
		if _, err := tx.Exec(`UPDATE OR IGNORE error_data SET timestamp = ? WHERE id = ?`,
			ts, id); err != nil {
			return fmt.Errorf("rewrite error_data %d: %w", id, err)
		}
	}
	return nil
}

// readStatusTimestamps returns the normalized timestamp of every row that changes.
// CAST drops the DATETIME decltype so the driver returns the stored text unparsed.
func readStatusTimestamps(tx *sql.Tx) (map[int64]string, error) {
	rows, err := tx.Query(`SELECT id, CAST(timestamp AS TEXT) FROM error_data`)
	if err != nil {
		return nil, fmt.Errorf("read error_data: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64]string)
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, fmt.Errorf("scan error_data: %w", err)
		}
		if ts, ok := NormalizeStatusTimestamp(raw); ok && ts != raw {
			out[id] = ts
		}
	}
	return out, rows.Err()
}

// NormalizeStatusTimestamp parses a legacy error_data timestamp and formats it with
// StatusTimestampLayout.
func NormalizeStatusTimestamp(raw string) (string, bool) {
	s, _, _ := strings.Cut(strings.TrimSpace(raw), " m=") // drop the monotonic reading
	for _, layout := range legacyTimestampLayouts() {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(StatusTimestampLayout), true
		}
	}
	return "", false
}

// Down is not supported: the original layouts carried no information worth restoring.
func (m *V4Migration) Down(_ *sql.Tx) error {
	return ErrNotImplemented
}

// GetV4Migration returns a new V4Migration instance.
func GetV4Migration() Migration {
	return &V4Migration{}
}
