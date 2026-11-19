package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTx opens a fresh database, runs setup and begins a transaction.
func openTx(t *testing.T, setup ...string) *sql.Tx {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	for _, s := range setup {
		_, err := db.Exec(s)
		require.NoError(t, err)
	}
	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

const errorDataSQL = `CREATE TABLE error_data (id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp DATETIME NOT NULL, register_key TEXT NOT NULL, raw_value REAL NOT NULL,
	UNIQUE(register_key, timestamp))`

func TestV3Migration_UpErrors(t *testing.T) {
	t.Parallel()
	legacyRow := `INSERT INTO error_data (timestamp, register_key, raw_value)
		VALUES ('2024-01-15 10:30:00', 'a', 1)`
	tests := []struct {
		name  string
		setup []string
		want  string
	}{
		{
			name:  "read fails",
			setup: []string{`CREATE TABLE error_data (id INTEGER PRIMARY KEY)`},
			want:  "read error_data",
		},
		{
			name: "scan fails",
			setup: []string{
				`CREATE TABLE error_data (id TEXT, timestamp TEXT)`,
				`INSERT INTO error_data VALUES ('x', '2024-01-15 10:30:00')`,
			},
			want: "scan error_data",
		},
		{
			name: "update fails",
			setup: []string{errorDataSQL, legacyRow, `CREATE TRIGGER no_update
				BEFORE UPDATE ON error_data BEGIN SELECT RAISE(ABORT, 'blocked'); END`},
			want: "rewrite error_data 1",
		},
		{
			name: "duplicate delete fails",
			setup: []string{
				errorDataSQL, legacyRow,
				`INSERT INTO error_data (timestamp, register_key, raw_value)
					VALUES ('2024-01-15T10:30:00.000Z', 'a', 1)`,
				`CREATE TRIGGER no_delete BEFORE DELETE ON error_data
					BEGIN SELECT RAISE(ABORT, 'blocked'); END`,
			},
			want: "drop duplicate error_data 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tx := openTx(t, tt.setup...)
			assert.ErrorContains(t, GetV3Migration().Up(context.Background(), tx), tt.want)
		})
	}
}

func TestV3Migration_UpFailsOnFinishedTx(t *testing.T) {
	t.Parallel()
	tx := openTx(t)
	require.NoError(t, tx.Rollback())
	assert.ErrorContains(t, GetV3Migration().Up(context.Background(), tx), "create meta table")

	tx = openTx(t)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := tx.ExecContext(ctx, V3MetaTableSQL)
	require.NoError(t, err)
	cancel()
	assert.Error(t, normalizeStatusTimestamps(ctx, tx), "check error_data table")
}
