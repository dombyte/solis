package storage

import (
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDSN_BarePathWithoutPragmas(t *testing.T) {
	assert.Equal(t, "/data/solis.db", dsn(Settings{Path: "/data/solis.db"}))
}

func TestDSN_EncodesConfiguredPragmas(t *testing.T) {
	got := dsn(Settings{Path: "a.db", WalMode: true, Synchronous: "FULL", TempStore: "FILE"})
	assert.Equal(t, "a.db?_pragma=journal_mode%28WAL%29&_pragma=synchronous%28FULL%29"+
		"&_pragma=temp_store%28FILE%29", got)
}

func pragmaValues(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range []string{"journal_mode", "synchronous", "temp_store"} {
		var v string
		require.NoError(t, db.QueryRowContext(ctx, "PRAGMA "+p).Scan(&v))
		out[p] = v
	}
	return out
}

// discardConn makes the pool drop its only connection, so the next query opens a new one.
func discardConn(t *testing.T, db *sql.DB) {
	t.Helper()
	c, err := db.Conn(ctx)
	require.NoError(t, err)
	err = c.Raw(func(any) error { return driver.ErrBadConn })
	require.ErrorIs(t, err, driver.ErrBadConn) // the pool has closed and dropped it
}

func TestOpen_PragmasSurviveReconnect(t *testing.T) {
	cfg := Settings{
		Path:    filepath.Join(t.TempDir(), "p.db"),
		WalMode: true, Synchronous: "EXTRA", TempStore: "MEMORY",
	}
	db, err := open(ctx, cfg, zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	want := map[string]string{"journal_mode": "wal", "synchronous": "3", "temp_store": "2"}
	assert.Equal(t, want, pragmaValues(t, db))

	discardConn(t, db)
	assert.Equal(t, want, pragmaValues(t, db))
}

func TestOpen_InvalidPragmaFails(t *testing.T) {
	cfg := Settings{Path: filepath.Join(t.TempDir(), "p.db"), Synchronous: "BOGUS("}
	_, err := open(ctx, cfg, zerolog.Nop())
	require.Error(t, err)
}
