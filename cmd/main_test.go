package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, configPath),
		[]byte("storage:\n  path: ./data/solis.db\n  enable_backup: true\n"), 0o600))

	errOut.Reset()
	assert.Equal(t, 1, runBackfill([]string{"--years", "-1"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "--years must be >= 0")

	// Empty database: backup fails because the file does not exist yet -> exit 1.
	errOut.Reset()
	assert.Equal(t, 1, runBackfill(nil, &out, &errOut))
	assert.Contains(t, errOut.String(), "nothing was written")
}
