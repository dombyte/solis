package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestNormalizeStatusTimestamp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
		ok       bool
	}{
		{
			"2026-09-27 20:58:18.612840635 +0200 CEST m=+0.000546397",
			"2026-09-27T18:58:18.612Z", true,
		},
		{"2026-10-25 02:30:00 +0100 CET", "2026-10-25T01:30:00.000Z", true},
		{"2026-10-25 02:30:00 +0200 CEST", "2026-10-25T00:30:00.000Z", true},
		{"2024-01-15T10:30:00Z", "2024-01-15T10:30:00.000Z", true},
		{"2024-01-15 10:30:00", "2024-01-15T10:30:00.000Z", true},
		{"2024-01-15T10:30:00.000Z", "2024-01-15T10:30:00.000Z", true},
		{"garbage", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, ok := NormalizeStatusTimestamp(tt.in)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestV3Migration_Up(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	m := GetV3Migration()
	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, m.Up(context.Background(), tx), "no error_data table only creates meta")
	require.NoError(t, tx.Commit())

	_, err = db.Exec(`CREATE TABLE error_data (id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL, register_key TEXT NOT NULL, raw_value REAL NOT NULL,
		string_value TEXT, UNIQUE(register_key, timestamp))`)
	require.NoError(t, err)
	// Row 3 is row 1's status change stored again in another legacy format: after
	// normalization both collide on UNIQUE(register_key, timestamp) (review DB-M3).
	_, err = db.Exec(`INSERT INTO error_data (timestamp, register_key, raw_value) VALUES
		('2026-09-27 20:58:18.612840635 +0200 CEST m=+0.000546397', 'a', 1),
		('garbage', 'a', 2),
		('2026-09-27T18:58:18.612840635Z', 'a', 1)`)
	require.NoError(t, err)

	tx, err = db.Begin()
	require.NoError(t, err)
	require.NoError(t, m.Up(context.Background(), tx))
	require.NoError(t, tx.Commit())

	rows, err := db.Query(`SELECT CAST(timestamp AS TEXT) FROM error_data ORDER BY id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		got = append(got, s)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"2026-09-27T18:58:18.612Z", "garbage"}, got)
}

func TestV3Migration_MetadataAndDown(t *testing.T) {
	m := GetV3Migration()
	assert.Equal(t, 3, m.Version())
	assert.Contains(t, m.Description(), "meta table")
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, m.Up(context.Background(), tx))
	require.NoError(t, m.Down(context.Background(), tx))
	require.NoError(t, tx.Commit())
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'meta'`).Scan(&n))
	assert.Zero(t, n, "Down drops the meta table")
}
