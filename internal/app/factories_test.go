package app

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/util"
	"github.com/dombyte/solis/internal/util/clocktest"
)

type nopReporter struct{}

func (nopReporter) Report(health.State, string) {}

func newModbusComponent(t *testing.T, slot *util.Slot[poller.Reader]) health.Component {
	t.Helper()
	settings := modbus.Settings{Address: "tcp://127.0.0.1:1", Timeout: time.Second}
	c, err := CreateModbus(settings, time.Hour, slot, clocktest.New(time.Now()), zerolog.Nop())(
		nopReporter{})
	require.NoError(t, err)
	return c
}

func TestModbusComponent_StaleStopKeepsSuccessor(t *testing.T) {
	var slot util.Slot[poller.Reader]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldC, newC := newModbusComponent(t, &slot), newModbusComponent(t, &slot)
	require.NoError(t, oldC.Start(ctx))
	require.NoError(t, newC.Start(ctx))
	current, _ := slot.Load()
	require.NoError(t, oldC.Stop())
	after, ok := slot.Load()
	assert.True(t, ok, "the old instance must not withdraw its successor")
	assert.Same(t, current, after)
	require.NoError(t, newC.Stop())
	_, ok = slot.Load()
	assert.False(t, ok)
}

func TestModbusComponent_StartAfterStopIsNoop(t *testing.T) {
	var slot util.Slot[poller.Reader]
	c := newModbusComponent(t, &slot)
	require.NoError(t, c.Stop())
	require.NoError(t, c.Start(context.Background()))
	_, ok := slot.Load()
	assert.False(t, ok, "a stopped instance never publishes its client")
	require.NoError(t, c.Stop())
}
