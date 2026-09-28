package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestGenerateBackupFilename(t *testing.T) {
	got := GenerateBackupFilename("/path/to/db/solis.db",
		time.Date(2026, 6, 27, 14, 30, 22, 7_000_000, time.UTC))
	assert.True(t, filepath.Ext(got) == ".backup", got)
	assert.Contains(t, got, filepath.Join("/path/to/db", "backups"))
	assert.Contains(t, got, "solis.db.")
	assert.NotContains(t, got, ".v", "no version marker")
	assert.NotContains(t, got, "online", "no online marker")
}

func TestExtractBackupInfo(t *testing.T) {
	// Legacy stamps (no "Z") were written in local time.
	info, err := ExtractBackupInfo("/path/to/backups/solis.db.20260627_143022.backup")
	require.NoError(t, err)
	assert.True(t, info.Timestamp.Equal(time.Date(2026, 6, 27, 14, 30, 22, 0, time.Local)))

	for _, name := range []string{
		"/path/to/backups/solis.db.backup",               // no timestamp
		"/path/to/backups/solisdb20260627_143022.backup", // no dot separator
	} {
		_, err := ExtractBackupInfo(name)
		assert.Error(t, err, name)
	}
}

// newSQLiteDB creates a database at path with one table and one row.
func newSQLiteDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO test (name) VALUES (?)", "test data")
	require.NoError(t, err)
}

// writeBackups creates a db placeholder plus the named files in its backups directory.
func writeBackups(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("db"), 0o600))
	backupsDir := filepath.Join(dir, "backups")
	require.NoError(t, os.MkdirAll(backupsDir, 0o750))
	for _, n := range names {
		require.NoError(t, os.WriteFile(filepath.Join(backupsDir, n), []byte("backup"), 0o600))
	}
	return dbPath
}

func TestCreateBackup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	newSQLiteDB(t, dbPath)
	cfg := &BackupConfig{Enabled: true, MaxBackups: 3, BackupInterval: 24 * time.Hour}

	backupPath, err := CreateBackup(context.Background(), dbPath, cfg, time.Now(), zerolog.Nop())
	require.NoError(t, err)
	st, err := os.Stat(backupPath)
	require.NoError(t, err)
	assert.Positive(t, st.Size())

	// The backup is a complete SQLite database with the source row.
	db, err := sql.Open("sqlite", backupPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var name string
	require.NoError(t, db.QueryRow("SELECT name FROM test").Scan(&name))
	assert.Equal(t, "test data", name)

	cfg.Enabled = false
	backupPath, err = CreateBackup(context.Background(), dbPath, cfg, time.Now(), zerolog.Nop())
	require.NoError(t, err)
	assert.Empty(t, backupPath, "disabled backup is a no-op")
}

func TestCreateBackup_MissingSource(t *testing.T) {
	cfg := &BackupConfig{Enabled: true}
	_, err := CreateBackup(context.Background(), filepath.Join(t.TempDir(), "missing.db"), cfg, time.Now(),
		zerolog.Nop())
	assert.Error(t, err)
}

func TestVerifyBackupFile_RejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.backup")
	require.NoError(t, os.WriteFile(path, []byte("definitely not a sqlite database file"), 0o600))
	require.Error(t, verifyBackupFile(context.Background(), path, zerolog.Nop()))
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "an unverifiable backup is removed")

	require.NoError(t, os.WriteFile(path, nil, 0o600))
	assert.ErrorContains(t, verifyBackupFile(context.Background(), path, zerolog.Nop()), "empty")
}

func TestBackupFilename_MillisecondsRoundTrip(t *testing.T) {
	at := time.Date(2026, 6, 27, 14, 30, 22, 7_000_000, time.UTC)
	name := GenerateBackupFilename("/db/solis.db", at)
	assert.Equal(t, filepath.Join("/db", "backups", "solis.db.20260627_143022_007Z.backup"), name)
	info, err := ExtractBackupInfo(name)
	require.NoError(t, err)
	assert.True(t, info.Timestamp.Equal(at))
	assert.NotEqual(t, name, GenerateBackupFilename("/db/solis.db", at.Add(time.Millisecond)))
	_, err = ExtractBackupInfo("/db/backups/solis.db.20260627_143022_x.backup")
	assert.Error(t, err)
}

// New stamps are UTC ("Z"), so the DST fall-back hour can no longer make a newer backup
// sort as older (review DB-L2); legacy local stamps still order by their real instant.
func TestBackupStamps_UTCAndLegacyLocalOrder(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	// 2026-10-25 02:30 happens twice in Berlin; one hour apart in UTC.
	first := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	assert.Equal(t, first.In(loc).Format(backupStampLayout),
		second.In(loc).Format(backupStampLayout), "local stamps collide")
	a, err := ExtractBackupInfo(GenerateBackupFilename("/db/s.db", first.In(loc)))
	require.NoError(t, err)
	b, err := ExtractBackupInfo(GenerateBackupFilename("/db/s.db", second.In(loc)))
	require.NoError(t, err)
	assert.True(t, b.Timestamp.After(a.Timestamp))
	assert.True(t, a.Timestamp.Equal(first))
}

// A backup of a WAL-mode database leaves no -wal/-shm files behind (review DB-M1): the
// copy is switched to a rollback journal before it is verified.
func TestCreateBackup_WALSourceLeavesNoSidecars(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wal.db")
	src, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer func() { _ = src.Close() }()
	for _, q := range []string{
		"PRAGMA journal_mode=WAL", "CREATE TABLE t (v TEXT)",
		"INSERT INTO t VALUES ('x')",
	} {
		_, err := src.Exec(q)
		require.NoError(t, err)
	}
	path, err := WriteBackup(context.Background(), dbPath, time.Now(), zerolog.Nop())
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the backup file itself")
	assert.Equal(t, filepath.Base(path), entries[0].Name())
	st, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm(), "owner-only like the database")

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var mode, v string
	require.NoError(t, db.QueryRow("PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "delete", mode)
	require.NoError(t, db.QueryRow("SELECT v FROM t").Scan(&v))
	assert.Equal(t, "x", v)
}

// Rotation removes the sidecars older versions left and never touches the backups of
// another database in the same directory (review DB-L1).
func TestCleanupBackups_SidecarsAndOtherDatabases(t *testing.T) {
	dbPath := writeBackups(t,
		"test.db.20260627_100000.backup", "test.db.20260627_100000.backup-wal",
		"test.db.20260627_100000.backup-shm", "test.db.20260627_110000.backup",
		"other.db.20260101_000000.backup", "other.db.20260102_000000.backup")
	require.NoError(t, CleanupBackups(dbPath, 1, zerolog.Nop()))
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(dbPath), "backups"))
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{
		"test.db.20260627_110000.backup",
		"other.db.20260101_000000.backup", "other.db.20260102_000000.backup",
	}, names)
}

func TestCleanupBackups(t *testing.T) {
	dbPath := writeBackups(t,
		"test.db.v1.20260627_100000.backup", "test.db.v1.20260627_110000.backup",
		"test.db.v1.20260627_120000.backup", "test.db.v1.20260627_130000.backup")

	require.NoError(t, CleanupBackups(dbPath, 0, zerolog.Nop()), "0 keeps everything")
	require.NoError(t, CleanupBackups(dbPath, 2, zerolog.Nop()))

	remaining, err := ListBackups(dbPath)
	require.NoError(t, err)
	require.Len(t, remaining, 2)
	assert.Contains(t, remaining[0].Filename, "130000", "newest first")
	assert.Contains(t, remaining[1].Filename, "120000")
}

func TestListBackups(t *testing.T) {
	dbPath := writeBackups(t,
		"test.db.v1.20260627_143022.backup", "test.db.online.20260627_153022.backup",
		"other_file.backup", "test.db.v2.20260627_163022.backup")
	require.NoError(t, os.Mkdir(filepath.Join(filepath.Dir(dbPath), "backups", "dir.backup"),
		0o750))

	backups, err := ListBackups(dbPath)
	require.NoError(t, err)
	require.Len(t, backups, 3, "other_file.backup and directories are ignored")
	for i := 1; i < len(backups); i++ {
		assert.False(t, backups[i-1].Timestamp.Before(backups[i].Timestamp), "newest first")
	}

	empty, err := ListBackups(filepath.Join(t.TempDir(), "none.db"))
	require.NoError(t, err)
	assert.Empty(t, empty, "missing backups directory is an empty list")
}
