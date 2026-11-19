package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/util"
	"github.com/dombyte/solis/internal/util/clocktest"
	"github.com/dombyte/solis/internal/websocket"
)

func TestBusProbe(t *testing.T) {
	t.Parallel()
	a := &App{bus: eventbus.New()}
	st, _ := a.busProbe()
	assert.Equal(t, health.Healthy, st)
	a.bus.Close()
	st, reason := a.busProbe()
	assert.Equal(t, health.Failed, st)
	assert.Equal(t, "event bus closed", reason)
}

func TestWebAssets(t *testing.T) {
	t.Parallel()
	fe, apiDocs, err := webAssets(zerolog.Nop())
	require.NoError(t, err)
	assert.NotNil(t, fe)
	assert.NotNil(t, apiDocs)
}

func TestFactories_InvalidDependencies(t *testing.T) {
	t.Parallel()
	_, err := CreateModbus(modbus.Settings{Address: "udp://x"}, time.Second,
		&util.Slot[poller.Reader]{}, clocktest.New(time.Now()), zerolog.Nop())(nopReporter{})
	assert.Error(t, err, "unsupported address scheme")
	_, err = CreateHub(websocket.HubDeps{}, &util.Slot[*websocket.Hub]{})(nopReporter{})
	assert.Error(t, err, "missing hub dependencies")
}

func TestCreateBackfillEnv_OpenStoreErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A directory where the database should be fails the migrations.
	cfg := &config.AppConfig{Storage: config.StorageSettings{Path: dir}}
	env, err := CreateBackfillEnv(cfg, io.Discard, zerolog.Nop())
	require.NoError(t, err)
	_, _, err = env.OpenStore(context.Background())
	assert.Error(t, err)

	// Migrations pass, the storage rejects its pragma.
	path := filepath.Join(t.TempDir(), "solis.db")
	cfg = &config.AppConfig{Storage: config.StorageSettings{Path: path, Synchronous: "BOGUS("}}
	env, err = CreateBackfillEnv(cfg, io.Discard, zerolog.Nop())
	require.NoError(t, err)
	// The backup of a database that does not exist yet fails before any rotation.
	_, err = env.Backup(context.Background())
	assert.Error(t, err)
	_, statErr := os.Stat(filepath.Join(filepath.Dir(path), "backups"))
	assert.True(t, os.IsNotExist(statErr), "nothing to rotate")
	_, _, err = env.OpenStore(context.Background())
	assert.Error(t, err)
}
