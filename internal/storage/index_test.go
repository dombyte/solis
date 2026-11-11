package storage

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util/clocktest"
)

// A database created by an earlier version carries indexes that repeat the UNIQUE
// constraints; opening it drops them, and key lookups keep using the constraint's index.
func TestNew_DropsDuplicateIndexes(t *testing.T) {
	s, _, path := newStore(t, day("2026-08-05"))
	_, err := s.db.ExecContext(ctx,
		`CREATE INDEX idx_daily_key_date ON daily_values(register_key, date)`)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `CREATE INDEX idx_total_key ON total_values(register_key)`)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	s, err = New(ctx, testConfig(path), reg, clocktest.New(day("2026-08-05")), zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	var n int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'index' AND name IN ('idx_daily_key_date', 'idx_monthly_key_month',
		'idx_yearly_key_year', 'idx_total_key', 'idx_error_key_timestamp')`).Scan(&n))
	assert.Zero(t, n)

	var id, parent, unused int
	var detail string
	require.NoError(t, s.db.QueryRowContext(ctx, `EXPLAIN QUERY PLAN SELECT SUM(value)
		FROM daily_values WHERE register_key = 'pv_energy_daily' AND date >= '2026-08-01'
		AND date <= '2026-08-05'`).Scan(&id, &parent, &unused, &detail))
	assert.Contains(t, detail, "USING INDEX sqlite_autoindex_daily_values_1")
}
