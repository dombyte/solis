package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// V3MetaTableSQL creates the key/value meta table used by v3 for the cutover date,
// closed-period watermarks and the total baseline. Existing tables stay byte-identical.
const V3MetaTableSQL = `CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY, value TEXT NOT NULL)`

// v3Version is the schema version V3Migration migrates to.
const v3Version = 3

// V3Migration adds the meta table (cutover, watermarks, baselines) and normalizes error_data
// timestamps.
type V3Migration struct{}

// Version returns the migration version (3).
func (m *V3Migration) Version() int { return v3Version }

// Description returns a description of what this migration does.
func (m *V3Migration) Description() string {
	return "v3 meta table (cutover date, closed-period watermarks, total baseline), " +
		"error_data timestamps normalized to fixed-width UTC"
}

// Up creates the meta table and rewrites error_data timestamps into StatusTimestampLayout.
func (m *V3Migration) Up(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, V3MetaTableSQL); err != nil {
		return fmt.Errorf("create meta table: %w", err)
	}
	return normalizeStatusTimestamps(ctx, tx)
}

// Down drops the meta table; the original error_data timestamp layouts are not restored.
func (m *V3Migration) Down(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS meta`)
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
// StatusTimestampLayout (UTC); unparseable values stay untouched. A row whose normalized
// timestamp collides with an existing row of the same key (the same status change stored
// twice in two formats) is deleted, so no legacy-format row is left behind to break the
// fixed-width ordering.
func normalizeStatusTimestamps(ctx context.Context, tx *sql.Tx) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'error_data'`).Scan(&n); err != nil {
		return fmt.Errorf("check error_data table: %w", err)
	}
	if n == 0 {
		return nil
	}
	rows, err := readStatusTimestamps(ctx, tx)
	if err != nil {
		return err
	}
	// In id order, so of two rows that normalize to the same instant the older one is
	// kept and the newer duplicate dropped, every time (map order is random).
	for _, id := range slices.Sorted(maps.Keys(rows)) {
		if err := rewriteStatusTimestamp(ctx, tx, id, rows[id]); err != nil {
			return err
		}
	}
	return nil
}

// rewriteStatusTimestamp normalizes one row, or deletes it when its normalized twin
// already exists.
func rewriteStatusTimestamp(ctx context.Context, tx *sql.Tx, id int64, ts string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE OR IGNORE error_data SET timestamp = ? WHERE id = ?`, ts, id)
	if err != nil {
		return fmt.Errorf("rewrite error_data %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM error_data WHERE id = ?`, id); err != nil {
		return fmt.Errorf("drop duplicate error_data %d: %w", id, err)
	}
	return nil
}

// readStatusTimestamps returns the normalized timestamp of every row that changes.
// CAST drops the DATETIME decltype so the driver returns the stored text unparsed.
func readStatusTimestamps(ctx context.Context, tx *sql.Tx) (map[int64]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, CAST(timestamp AS TEXT) FROM error_data`)
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
