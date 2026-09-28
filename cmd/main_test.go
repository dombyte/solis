package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/maintenance"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/util/clocktest"
)

// inConfigDir switches to a temp dir holding config.yaml and returns the DB path.
func inConfigDir(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, configPath), []byte(yaml), 0o600))
	return filepath.Join(dir, "data", "solis.db")
}

func TestDispatch_HelpAndUnknown(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, dispatch([]string{"help"}, &out, &errOut))
	assert.Contains(t, out.String(), "solis backfill --years N")
	assert.Equal(t, 1, dispatch([]string{"frobnicate"}, &out, &errOut))
	assert.Contains(t, errOut.String(), `unknown command "frobnicate"`)
}

func TestRestartLoop(t *testing.T) {
	var logs bytes.Buffer
	assert.Equal(t, 0, restartLoop(&logs, func() error { return nil }, 3, 0))

	calls := 0
	code := restartLoop(&logs, func() error {
		calls++
		if calls == 1 {
			panic("boom")
		}
		return errors.New("health fatal: poller")
	}, 3, 0)
	assert.Equal(t, 1, code, "shared restart ceiling reached")
	assert.Equal(t, 3, calls)
	assert.Contains(t, logs.String(), "panic: boom")
}

func TestBackfill_FlagsAndConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 1, runBackfill([]string{"--years", "x"}, &out, &errOut))

	inConfigDir(t, "storage:\n  path: ./data/solis.db\n  enable_backup: true\n")

	errOut.Reset()
	assert.Equal(t, 1, runBackfill([]string{"--years", "-1"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "--years must be >= 0")

	// Empty database: backup fails because the file does not exist yet -> exit 1.
	errOut.Reset()
	assert.Equal(t, 1, runBackfill(nil, &out, &errOut))
	assert.Contains(t, errOut.String(), "nothing was written")
}

func TestBackfill_Success(t *testing.T) {
	dbPath := inConfigDir(t, "storage:\n  path: ./data/solis.db\n")
	ctx, clk := context.Background(), clocktest.New(time.Now())
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o750))
	mgr := database.NewManager(database.Settings{Path: dbPath}, &database.BackupConfig{}, clk,
		zerolog.Nop())
	require.NoError(t, mgr.Prepare(ctx))
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	st, err := storage.New(ctx, storage.Settings{Path: dbPath}, reg, clk, zerolog.Nop())
	require.NoError(t, err)
	require.NoError(t, st.Close())

	var out, errOut bytes.Buffer
	require.Equal(t, 0, runBackfill([]string{"--years", "0"}, &out, &errOut), errOut.String())
	assert.Contains(t, out.String(), "backup: ")
}

func TestRunServer_ConfigErrorAndLockedDatabase(t *testing.T) {
	inConfigDir(t, "app:\n  port: 0\n")
	require.Error(t, runServer(), "invalid config")

	dbPath := inConfigDir(t, "app:\n  serve_only: true\nstorage:\n  path: ./data/solis.db\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o750))
	job, err := maintenance.AcquireExclusive(dbPath)
	require.NoError(t, err)
	defer func() { _ = job.Release() }()
	assert.ErrorIs(t, runServer(), maintenance.ErrLocked)
}
