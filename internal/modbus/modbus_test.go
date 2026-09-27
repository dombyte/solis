package modbus

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	sv "github.com/simonvetter/modbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/utils"
	"github.com/dombyte/solis/internal/utils/clocktest"
)

// device serves input register i as value i.
type device struct{ reads atomic.Int32 }

func (d *device) HandleCoils(*sv.CoilsRequest) ([]bool, error) { return nil, sv.ErrIllegalFunction }
func (d *device) HandleDiscreteInputs(*sv.DiscreteInputsRequest) ([]bool, error) {
	return nil, sv.ErrIllegalFunction
}
func (d *device) HandleHoldingRegisters(*sv.HoldingRegistersRequest) ([]uint16, error) {
	return nil, sv.ErrIllegalFunction
}
func (d *device) HandleInputRegisters(req *sv.InputRegistersRequest) ([]uint16, error) {
	d.reads.Add(1)
	out := make([]uint16, req.Quantity)
	for i := range out {
		out[i] = req.Addr + uint16(i)
	}
	return out, nil
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

func startDevice(t *testing.T, port int) *sv.ModbusServer {
	t.Helper()
	srv, err := sv.NewServer(&sv.ServerConfiguration{
		URL: "tcp://127.0.0.1:" + strconv.Itoa(port), Timeout: time.Second, MaxClients: 2,
	}, &device{})
	require.NoError(t, err)
	require.NoError(t, srv.Start())
	return srv
}

func settings(port int) Settings {
	return Settings{Host: "127.0.0.1", Port: port, UnitID: 1, Timeout: 500 * time.Millisecond}
}

func TestSettingsValidate(t *testing.T) {
	assert.NoError(t, settings(502).Validate())
	for _, s := range []Settings{{}, {Host: "h", Port: 0, Timeout: 1}, {Host: "h", Port: 70000,
		Timeout: 1}, {Host: "h", Port: 1}} {
		assert.ErrorIs(t, s.Validate(), ErrInvalidSettings)
	}
	_, err := New(Settings{}, utils.NewRealClock(), zerolog.Nop())
	assert.ErrorIs(t, err, ErrInvalidSettings)
}

func TestNew_DoesNotConnect(t *testing.T) {
	c, err := New(settings(freePort(t)), utils.NewRealClock(), zerolog.Nop())
	require.NoError(t, err)
	assert.False(t, c.IsConnected())
	_, err = c.ReadRegisters(context.Background(), 1, 1)
	assert.ErrorIs(t, err, ErrNotConnected)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.ReadRegisters(ctx, 1, 1)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NoError(t, c.Close())
}

func TestRun_ConnectReadLoseReconnect(t *testing.T) {
	port := freePort(t)
	srv := startDevice(t, port)
	clk := clocktest.New(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))
	c, err := New(settings(port), clk, zerolog.Nop())
	require.NoError(t, err)

	var beats atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, 5*time.Second, func() { beats.Add(1) }); close(done) }()
	defer func() { cancel(); <-done }()

	require.Eventually(t, c.IsConnected, 2*time.Second, time.Millisecond)
	regs, err := c.ReadRegisters(context.Background(), 33035, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{33035, 33036, 33037}, regs)

	// Device goes away: the read fails and marks the connection lost.
	require.NoError(t, srv.Stop())
	require.Eventually(t, func() bool {
		_, err := c.ReadRegisters(context.Background(), 1, 1)
		var re *ReadError
		return errors.As(err, &re) || errors.Is(err, ErrNotConnected)
	}, 2*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return !c.IsConnected() }, time.Second, time.Millisecond)

	// While reconnecting it keeps beating inside the backoff.
	before := beats.Load()
	for range 4 {
		require.True(t, clk.BlockUntil(1))
		clk.Advance(5 * time.Second)
	}
	require.Eventually(t, func() bool { return beats.Load() >= before+3 }, 2*time.Second,
		time.Millisecond)

	// Device returns: the loop reconnects.
	srv = startDevice(t, port)
	defer func() { _ = srv.Stop() }()
	require.Eventually(t, func() bool {
		clk.Advance(MaxBackoff)
		return c.IsConnected()
	}, 3*time.Second, 5*time.Millisecond)
	_, err = c.ReadRegisters(context.Background(), 10, 1)
	assert.NoError(t, err)
}

func TestReadError(t *testing.T) {
	e := &ReadError{Addr: 1, Count: 2, Err: errors.New("x")}
	assert.Contains(t, e.Error(), "read 2 registers at 1")
	assert.EqualError(t, errors.Unwrap(e), "x")
}
