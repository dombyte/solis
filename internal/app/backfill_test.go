package app

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/maintenance"
)

func TestCreateBackfillEnv_RunsAJob(t *testing.T) {
	cfg := &config.AppConfig{Storage: config.StorageSettings{
		Path: filepath.Join(t.TempDir(), "solis.db"), MaxBackups: 1,
		Synchronous: "NORMAL", TempStore: "MEMORY",
	}}
	var out bytes.Buffer
	env, err := CreateBackfillEnv(cfg, &out, zerolog.Nop())
	require.NoError(t, err)

	// Create the database (migrations + schema) the way the job opens it.
	_, closeStore, err := env.OpenStore(context.Background())
	require.NoError(t, err)
	require.NoError(t, closeStore())

	for range 2 {
		out.Reset()
		require.NoError(t, maintenance.RunBackfill(context.Background(), env, 0))
		assert.Contains(t, out.String(), "backup: ")
	}
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(cfg.Storage.Path), "backups", "*"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "backups rotated to max_backups")
}
