package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/util/clocktest"
)

func newManager(t *testing.T, path string, clk *clocktest.Clock) *Manager {
	t.Helper()
	cfg := Settings{Path: path, CleanupInterval: time.Hour}
	backup := &BackupConfig{Enabled: true, MaxBackups: 2, BackupInterval: time.Hour}
	return NewManager(cfg, backup, clk, zerolog.Nop())
}

func schemaVersion(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	v, err := NewMigrationExecutor(NewMigrationRegistry(), zerolog.Nop()).GetCurrentVersion(context.Background(), db)
	require.NoError(t, err)
	return v
}

func hasTable(t *testing.T, path, table string) bool {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'
		AND name=?`, table).Scan(&n))
	return n == 1
}

func TestPrepare_FreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "solis.db")
	m := newManager(t, path, clocktest.New(time.Now()))
	require.NoError(t, m.Prepare(context.Background()))
	assert.Equal(t, CurrentSchemaVersion, schemaVersion(t, path))
	assert.True(t, hasTable(t, path, "meta"))

	backups, err := ListBackups(path)
	require.NoError(t, err)
	assert.Empty(t, backups, "no backup for a database that did not exist")

	// Idempotent.
	require.NoError(t, m.Prepare(context.Background()))
}

func TestPrepare_LegacyDatabaseIsBackedUpAndMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "solis.db")
	require.NoError(t, createTestDB(path))
	m := newManager(t, path, clocktest.New(time.Now()))
	require.NoError(t, m.Prepare(context.Background()))
	assert.Equal(t, CurrentSchemaVersion, schemaVersion(t, path))
	backups, err := ListBackups(path)
	require.NoError(t, err)
	assert.Len(t, backups, 1)
}

func TestPrepare_OpenError(t *testing.T) {
	m := newManager(t, t.TempDir(), clocktest.New(time.Now()))
	assert.Error(t, m.Prepare(context.Background()))
}

type countingCleaner struct {
	n   atomic.Int32
	err error
}

func (c *countingCleaner) CleanupAll(context.Context) error {
	c.n.Add(1)
	return c.err
}

func TestRunPeriodicCleanup(t *testing.T) {
	clk := clocktest.New(time.Now())
	m := newManager(t, filepath.Join(t.TempDir(), "x.db"), clk)
	c := &countingCleaner{err: errors.New("boom")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.RunPeriodicCleanup(ctx, c); close(done) }()

	require.True(t, clk.BlockUntil(1))
	assert.Equal(t, int32(1), c.n.Load(), "runs immediately")
	clk.Advance(time.Hour)
	assert.Eventually(t, func() bool { return c.n.Load() == 2 }, time.Second, time.Millisecond)
	cancel()
	<-done
}

func TestRunPeriodicBackups(t *testing.T) {
	clk := clocktest.New(time.Now())
	path := filepath.Join(t.TempDir(), "solis.db")
	require.NoError(t, createTestDB(path))
	m := newManager(t, path, clk)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.RunPeriodicBackups(ctx); close(done) }()

	require.True(t, clk.BlockUntil(1))
	clk.Advance(time.Hour)
	assert.Eventually(t, func() bool {
		b, _ := ListBackups(path)
		return len(b) == 1
	}, 2*time.Second, 5*time.Millisecond)
	cancel()
	<-done
}

func TestRunPeriodic_Disabled(t *testing.T) {
	cfg := Settings{Path: "x"}
	m := NewManager(cfg, &BackupConfig{}, clocktest.New(time.Now()), zerolog.Nop())
	m.RunPeriodicBackups(context.Background()) // returns immediately
	m.RunPeriodicCleanup(context.Background(), &countingCleaner{})
}

func TestRegistry_DuplicateIgnored(t *testing.T) {
	r := NewMigrationRegistry()
	assert.True(t, r.Register(&v1Stub{}))
	assert.False(t, r.Register(&v1Stub{}))
	assert.Len(t, r.GetMigrationsFrom(0), 1)
}

type v1Stub struct{}

func (v1Stub) Version() int                        { return 1 }
func (v1Stub) Description() string                 { return "stub" }
func (v1Stub) Up(context.Context, *sql.Tx) error   { return nil }
func (v1Stub) Down(context.Context, *sql.Tx) error { return nil }

func execSQL(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	for _, s := range stmts {
		_, err := db.Exec(s)
		require.NoError(t, err)
	}
}

func TestPrepare_V2DatabaseIsMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "solis.db")
	execSQL(t, path, SchemaVersionTableSQL,
		`INSERT INTO schema_version (version, success) VALUES (1, 1), (2, 1)`,
		`CREATE TABLE daily_values (id INTEGER PRIMARY KEY, date DATE)`)
	m := newManager(t, path, clocktest.New(time.Now()))
	require.NoError(t, m.Prepare(context.Background()))
	assert.Equal(t, CurrentSchemaVersion, schemaVersion(t, path))
	assert.True(t, hasTable(t, path, "meta"))
}

// Migrations rewrite data in place: without a verified backup nothing is migrated, and
// the backup is taken even when periodic backups are disabled (review DB-M2).
func TestPrepare_NoBackupNoMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "solis.db")
	require.NoError(t, createTestDB(path))
	// A file where the backups directory should be makes every backup fail.
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "backups"), nil, 0o600))
	err := newManager(t, path, clocktest.New(time.Now())).Prepare(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-migration backup failed, not migrating")
	assert.False(t, hasTable(t, path, "meta"), "nothing migrated")
}

func TestPrepare_BackupEvenWhenBackupsDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "solis.db")
	require.NoError(t, createTestDB(path))
	m := NewManager(Settings{Path: path}, &BackupConfig{Enabled: false}, clocktest.New(time.Now()),
		zerolog.Nop())
	require.NoError(t, m.Prepare(context.Background()))
	backups, err := ListBackups(path)
	require.NoError(t, err)
	assert.Len(t, backups, 1)
}

// A schema written by a newer release is refused instead of treated as up to date
// (review DB-M4).
func TestPrepare_NewerSchemaIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "solis.db")
	execSQL(t, path, SchemaVersionTableSQL,
		`INSERT INTO schema_version (version, success) VALUES (2, 1), (3, 1), (4, 1)`)
	err := newManager(t, path, clocktest.New(time.Now())).Prepare(context.Background())
	require.ErrorIs(t, err, ErrSchemaTooNew)
	var tooNew *SchemaTooNewError
	require.ErrorAs(t, err, &tooNew)
	assert.Equal(t, 4, tooNew.Version)
}

func TestPrepare_TooOldDatabaseIsRejected(t *testing.T) {
	tests := map[string][]string{
		"schema v1": {
			SchemaVersionTableSQL,
			`INSERT INTO schema_version (version, success) VALUES (1, 1)`,
		},
		"pre-migration data": {`CREATE TABLE daily_values (id INTEGER PRIMARY KEY)`},
	}
	for name, stmts := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "solis.db")
			execSQL(t, path, stmts...)
			err := newManager(t, path, clocktest.New(time.Now())).Prepare(context.Background())
			require.ErrorIs(t, err, ErrSchemaTooOld)
			var tooOld *SchemaTooOldError
			require.ErrorAs(t, err, &tooOld)
			assert.Contains(t, err.Error(), "v2 release")
			assert.False(t, hasTable(t, path, "meta"), "nothing migrated")
			backups, err := ListBackups(path)
			require.NoError(t, err)
			assert.Empty(t, backups, "no backup for a rejected database")
		})
	}
}
