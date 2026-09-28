package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modernc.org/sqlite"
)

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func TestCloseInto(t *testing.T) {
	errClose := errors.New("close failed")
	var err error
	closeInto(&err, closerFunc(func() error { return errClose }), "thing")
	require.ErrorIs(t, err, errClose)
	assert.Contains(t, err.Error(), "failed to close thing")

	earlier := errors.New("earlier")
	err = earlier
	closeInto(&err, closerFunc(func() error { return errClose }), "thing")
	assert.Equal(t, earlier, err, "an earlier error is kept")
}

type failingBackuper struct{}

func (failingBackuper) NewBackup(string) (*sqlite.Backup, error) {
	return nil, errors.New("no backup")
}

func TestRunBackup_Errors(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "x.backup")
	assert.ErrorContains(t, runBackup(struct{}{}, dest), "does not support backups")
	assert.ErrorContains(t, runBackup(failingBackuper{}, dest), "failed to create backup object")
}

func TestCreateSQLiteBackup_Errors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	require.NoError(t, createTestDB(src))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := createSQLiteBackup(ctx, src, filepath.Join(dir, "a.backup"), zerolog.Nop())
	assert.ErrorContains(t, err, "ping source database")

	// The destination directory cannot be created below a regular file.
	blocker := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	err = createSQLiteBackup(context.Background(), src, filepath.Join(blocker, "sub", "b.backup"),
		zerolog.Nop())
	assert.ErrorContains(t, err, "create destination directory")
}
