package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/util/clocktest"
)

func TestSchemaTooNewError(t *testing.T) {
	t.Parallel()
	err := &SchemaTooNewError{Version: 9}
	assert.Contains(t, err.Error(), "schema version 9")
	assert.ErrorIs(t, err, ErrSchemaTooNew)
}

// A database named like another one's prefix never has its backups rotated by it
// (solis.db must not delete solis.db.old's backups).
func TestListBackups_OnlyOwnBackups(t *testing.T) {
	t.Parallel()
	dbPath := writeBackups(t,
		"test.db.20260627_100000Z.backup", "test.db.online.20260627_110000.backup",
		"test.db.v2.20260627_120000.backup", "test.db.old.20260627_130000Z.backup",
		"test.db.vx.20260627_140000.backup", "test.db.a.b.20260627_150000.backup",
		"test.db.20260627_1600.backup")
	backups, err := ListBackups(dbPath)
	require.NoError(t, err)
	var names []string
	for _, b := range backups {
		names = append(names, filepath.Base(b.Filename))
	}
	assert.ElementsMatch(t, []string{
		"test.db.v2.20260627_120000.backup", "test.db.online.20260627_110000.backup",
		"test.db.20260627_100000Z.backup",
	}, names)
}

func TestListBackups_ReadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "backups"), nil, 0o600))
	_, err := ListBackups(filepath.Join(dir, "x.db"))
	require.ErrorContains(t, err, "failed to read backups directory")
	assert.ErrorContains(t, CleanupBackups(filepath.Join(dir, "x.db"), 1, zerolog.Nop()),
		"failed to list backups")
}

func TestExtractBackupInfo_Invalid(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"solis.db.20260627_143022.txt",
		"solis.db.20260627_143022.5.backup",
		"solis.db.20260627_143022_.backup",
		"solis.db.20260627_143022x123.backup",
		"solis.db.20260627_143022_1000.backup",
		"solis.db.20260627_143022_-1.backup",
	} {
		_, err := ExtractBackupInfo(name)
		assert.Error(t, err, name)
	}
	info, err := ExtractBackupInfo("solis.db.20260627_143022_007Z.backup")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 6, 27, 14, 30, 22, 7_000_000, time.UTC), info.Timestamp)
}

func TestWriteBackup_StatError(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	_, err := WriteBackup(context.Background(), filepath.Join(file, "x.db"), time.Now(),
		zerolog.Nop())
	assert.ErrorContains(t, err, "stat database file")
}

func TestPrepare_StatError(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	m := newManager(t, filepath.Join(file, "x.db"), clocktest.New(time.Now()))
	assert.ErrorContains(t, m.Prepare(context.Background()), "database: stat")
}

func TestPrepare_CanceledContext(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "solis.db")
	require.NoError(t, createTestDB(path))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := newManager(t, path, clocktest.New(time.Now()))
	assert.ErrorContains(t, m.Prepare(ctx), "database: ping")
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestUpgrade_Errors(t *testing.T) {
	t.Parallel()
	m := newManager(t, "unused", clocktest.New(time.Now()))
	db := openDB(t)
	require.NoError(t, db.Close())
	assert.ErrorContains(t, m.upgrade(context.Background(), db, false),
		"current schema version")
	assert.ErrorContains(t, checkCompatible(context.Background(), db, 0), "check data tables")

	db = openDB(t)
	_, err := db.Exec(`CREATE TABLE schema_version (id INTEGER)`)
	require.NoError(t, err)
	_, err = NewMigrationExecutor(NewMigrationRegistry(), zerolog.Nop()).
		GetCurrentVersion(context.Background(), db)
	assert.ErrorContains(t, err, "failed to query schema version")
}

type failingMigration struct{ v1Stub }

func (failingMigration) Up(context.Context, *sql.Tx) error { return errors.New("broken") }

func TestApplyPendingMigrations(t *testing.T) {
	t.Parallel()
	reg := NewMigrationRegistry()
	exec := NewMigrationExecutor(reg, zerolog.Nop())
	db := openDB(t)
	n, err := exec.ApplyPendingMigrations(context.Background(), db, 0)
	require.NoError(t, err)
	assert.Zero(t, n, "nothing registered")

	reg.Register(failingMigration{})
	n, err = exec.ApplyPendingMigrations(context.Background(), db, 0)
	require.ErrorContains(t, err, "failed to apply migration 1")
	assert.ErrorContains(t, err, "broken")
	assert.Zero(t, n)

	require.NoError(t, db.Close())
	err = exec.ApplyMigration(context.Background(), db, v1Stub{})
	assert.ErrorContains(t, err, "begin migration transaction")
}

func TestApplyInTx_RecordErrors(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	// A schema_version table without the recorded columns fails the record step.
	_, err := db.Exec(`CREATE TABLE schema_version (id INTEGER)`)
	require.NoError(t, err)
	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	assert.ErrorContains(t, applyInTx(context.Background(), tx, v1Stub{}),
		"failed to ensure schema_version table")

	db = openDB(t)
	_, err = db.Exec(`CREATE TABLE schema_version (version INTEGER, description TEXT,
		applied_at TEXT, success INTEGER)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER no_insert BEFORE INSERT ON schema_version
		BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	require.NoError(t, err)
	tx2, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx2.Rollback() }()
	assert.ErrorContains(t, applyInTx(context.Background(), tx2, v1Stub{}),
		"failed to record migration")
}

// A failed periodic backup is logged and retried on the next tick.
func TestRunPeriodicBackups_FailureKeepsRunning(t *testing.T) {
	t.Parallel()
	clk := clocktest.New(time.Now())
	m := newManager(t, filepath.Join(t.TempDir(), "missing.db"), clk)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.RunPeriodicBackups(ctx); close(done) }()
	require.True(t, clk.BlockUntil(1))
	cancel()
	<-done

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	m.RunPeriodicBackups(ctx) // ctx already done before the first backup
}

func TestCreateSQLiteBackup_RemovesUnfinishedCopy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	require.NoError(t, createTestDB(src))
	// The destination path is a directory: the copy fails and nothing is left behind.
	dest := filepath.Join(dir, "dest.backup")
	require.NoError(t, os.Mkdir(dest, 0o750))
	err := createSQLiteBackup(context.Background(), src, dest, zerolog.Nop())
	assert.ErrorContains(t, err, "SQLite backup failed")
}
