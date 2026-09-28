package migrations

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// V3MetaTableSQL creates the key/value meta table used by v3 for the cutover date,
// closed-period watermarks and the total baseline. Existing tables stay byte-identical.
const V3MetaTableSQL = `CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY, value TEXT NOT NULL)`

// V3Migration adds the meta table (Plan.md D1) and normalizes error_data timestamps.
type V3Migration struct{}

// Version returns the migration version (3).
func (m *V3Migration) Version() int { return 3 }

// Description returns a description of what this migration does.
func (m *V3Migration) Description() string {
	return "v3 meta table (cutover date, closed-period watermarks, total baseline), " +
		"error_data timestamps normalized to fixed-width UTC"
}

// Up creates the meta table and rewrites error_data timestamps into StatusTimestampLayout.
func (m *V3Migration) Up(tx *sql.Tx) error {
	if _, err := tx.Exec(V3MetaTableSQL); err != nil {
		return fmt.Errorf("create meta table: %w", err)
	}
	return normalizeStatusTimestamps(tx)
}

// Down drops the meta table; the original error_data timestamp layouts are not restored.
func (m *V3Migration) Down(tx *sql.Tx) error {
	_, err := tx.Exec(`DROP TABLE IF EXISTS meta`)
	return err
}

// GetV3Migration returns a new V3Migration instance.
func GetV3Migration() Migration {
	return &V3Migration{}
}

// StatusTimestampLayout is the fixed-width UTC layout of error_data.timestamp since V3.
// Fixed width keeps string comparison (range filters, ORDER BY, retention) in time
// order; storage writes and binds every error_data timestamp with it.
const StatusTimestampLayout = period.TimestampLayout

// legacyTimestampLayouts are the formats found in error_data before V3: Go's
// time.Time.String() (bound directly by early v3 builds), RFC 3339 and SQLite's
// CURRENT_TIMESTAMP.
func legacyTimestampLayouts() []string {
	return []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
	}
}

// normalizeStatusTimestamps rewrites every parseable error_data timestamp into
// StatusTimestampLayout (UTC); unparseable values stay untouched.
func normalizeStatusTimestamps(tx *sql.Tx) error {
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
