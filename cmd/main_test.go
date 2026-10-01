package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, defaultConfigPath), []byte(yaml), 0o600))
	return filepath.Join(dir, "data", "solis.db")
}

func TestDispatch_HelpAndUnknown(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, dispatch([]string{"help"}, &out, &errOut))
	assert.Contains(t, out.String(), "solis [-config PATH] backfill --years N")
	out.Reset()
	assert.Equal(t, 0, dispatch([]string{"version"}, &out, &errOut))
	assert.Equal(t, buildInfo().String()+"\n", out.String())
	assert.Contains(t, out.String(), "solis dev (commit unknown")
	out.Reset()
	assert.Equal(t, 0, dispatch([]string{"--version"}, &out, &errOut))
	assert.Equal(t, buildInfo().String()+"\n", out.String())
	assert.Equal(t, 0, dispatch([]string{"-h"}, &out, &errOut), "help is not an error")
	assert.Contains(t, errOut.String(), "-config PATH")
	assert.Equal(t, 1, dispatch([]string{"-nope"}, &out, &errOut))
	assert.Equal(t, 1, dispatch([]string{"frobnicate"}, &out, &errOut))
	assert.Contains(t, errOut.String(), `unknown command "frobnicate"`)
}

// -config selects the configuration file for every subcommand; without it
// ./config.yaml is read.
func TestDispatch_ConfigFlag(t *testing.T) {
	inConfigDir(t, "storage:\n  path: ./data/solis.db\n")
	other := filepath.Join(t.TempDir(), "other.yaml")
	require.NoError(t, os.WriteFile(other, []byte("app:\n  port: 0\n"), 0o600))

	var out, errOut bytes.Buffer
	assert.Equal(t, 1, dispatch([]string{"-config", other, "backfill"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "load configuration", "the -config file is read")

	errOut.Reset()
	assert.Equal(t, 1, dispatch([]string{"backfill"}, &out, &errOut))
	assert.NotContains(t, errOut.String(), "load configuration",
		"./config.yaml is valid, the job fails later (no database)")

	errOut.Reset()
	assert.Equal(t, 1, dispatch([]string{"-config", other}, &out, &errOut), "serve reads it too")
	assert.Contains(t, errOut.String(), "exiting with code 1")
}

// There is no in-process restart loop: the server runs once, a clean shutdown is exit
// 0 and every error (health fatal, startup, shutdown timeout) is exit 1 so the container
// runtime restarts the process.
func TestServe_ExitCodes(t *testing.T) {
	var logs bytes.Buffer
	calls := 0
	assert.Equal(t, 0, serve(&logs, func(context.Context) error { calls++; return nil }))
	assert.Contains(t, logs.String(), "clean shutdown")

	logs.Reset()
	code := serve(&logs, func(context.Context) error {
		calls++
		return errors.New("health fatal: poller: restart budget exhausted")
	})
	assert.Equal(t, 1, code)
	assert.Equal(t, 2, calls, "run exactly once per process, no retry")
	assert.Contains(t, logs.String(), "exiting with code 1")
}

func TestServe_SignalCancelsContext(t *testing.T) {
	var logs bytes.Buffer
	code := serve(&logs, func(ctx context.Context) error {
		p, err := os.FindProcess(os.Getpid())
		require.NoError(t, err)
		require.NoError(t, p.Signal(syscall.SIGTERM))
		select {
		case <-ctx.Done():
			return nil // the app treats a cancelled parent context as a clean shutdown
		case <-time.After(5 * time.Second):
			return errors.New("SIGTERM did not cancel the context")
		}
	})
	assert.Equal(t, 0, code, logs.String())
}

func TestBackfill_FlagsAndConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 1, runBackfill([]string{"--years", "x"}, defaultConfigPath, &out, &errOut))
	assert.Equal(t, 0, runBackfill([]string{"-h"}, defaultConfigPath, &out, &errOut), "help is not an error")
	assert.Contains(t, errOut.String(), "-years")
	assert.Contains(t, errOut.String(), "-force")

	inConfigDir(t, "storage:\n  path: ./data/solis.db\n  enable_backup: true\n")

	errOut.Reset()
	assert.Equal(t, 1, runBackfill([]string{"--years", "-1"}, defaultConfigPath, &out, &errOut))
	assert.Contains(t, errOut.String(), "--years must be >= 0")

	// Empty database: backup fails because the file does not exist yet -> exit 1.
	errOut.Reset()
	assert.Equal(t, 1, runBackfill(nil, defaultConfigPath, &out, &errOut))
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
	require.Equal(t, 0, runBackfill([]string{"--years", "0"}, defaultConfigPath, &out, &errOut), errOut.String())
	assert.Contains(t, out.String(), "backup: ")
}

func TestRunServer_ConfigErrorAndLockedDatabase(t *testing.T) {
	inConfigDir(t, "app:\n  port: 0\n")
	require.Error(t, runServer(context.Background(), defaultConfigPath), "invalid config")

	dbPath := inConfigDir(t, "app:\n  serve_only: true\nstorage:\n  path: ./data/solis.db\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o750))
	job, err := maintenance.AcquireExclusive(dbPath)
	require.NoError(t, err)
	defer func() { _ = job.Release() }()
	assert.ErrorIs(t, runServer(context.Background(), defaultConfigPath), maintenance.ErrLocked)
}
