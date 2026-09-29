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

	"github.com/dombyte/solis/internal/util"
	"github.com/dombyte/solis/internal/util/clocktest"
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

// illegalAddr answers every read with an ILLEGAL DATA ADDRESS exception.
const illegalAddr = 9000

func (d *device) HandleInputRegisters(req *sv.InputRegistersRequest) ([]uint16, error) {
	d.reads.Add(1)
	if req.Addr == illegalAddr {
		return nil, sv.ErrIllegalDataAddress
	}
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
	return Settings{
		Address: "tcp://127.0.0.1:" + strconv.Itoa(port), UnitID: 1,
		Timeout: 500 * time.Millisecond,
	}
}

func TestSettingsValidate(t *testing.T) {
	assert.NoError(t, settings(502).Validate())
	assert.NoError(t, Settings{Address: "rtu:///dev/ttyUSB0", Timeout: time.Second}.Validate())
	assert.NoError(t, Settings{
		Address: "rtu:///dev/ttyUSB0", Timeout: time.Second,
		Parity: "e",
	}.Validate())
	for _, s := range []Settings{
		{},
		{Address: "tcp://h:502", Timeout: 0},
		{Address: "udp://h:502", Timeout: 1},
		{Address: "tcp://", Timeout: 1},
		{Address: "rtu://", Timeout: 1},
		{Address: "rtu:///dev/ttyUSB0", Timeout: 1, Parity: "X"},
	} {
		assert.ErrorIs(t, s.Validate(), ErrInvalidSettings)
	}
	_, err := New(Settings{}, util.NewRealClock(), zerolog.Nop())
	assert.ErrorIs(t, err, ErrInvalidSettings)
}

func TestParity(t *testing.T) {
	p, err := parity("")
	require.NoError(t, err)
	assert.Equal(t, sv.PARITY_NONE, p)
	p, err = parity("n")
	require.NoError(t, err)
	assert.Equal(t, sv.PARITY_NONE, p)
	p, err = parity("E")
	require.NoError(t, err)
	assert.Equal(t, sv.PARITY_EVEN, p)
	p, err = parity("O")
	require.NoError(t, err)
	assert.Equal(t, sv.PARITY_ODD, p)
	_, err = parity("X")
	assert.Error(t, err)
}

func TestConnect_RTUMissingDevice(t *testing.T) {
	c, err := New(Settings{Address: "rtu:///dev/nonexistent-solis-test", Timeout: time.Second},
		util.NewRealClock(), zerolog.Nop())
	require.NoError(t, err)
	err = c.connect()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rtu:///dev/nonexistent-solis-test")
}

func TestNew_DoesNotConnect(t *testing.T) {
	c, err := New(settings(freePort(t)), util.NewRealClock(), zerolog.Nop())
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

func TestKeepsConnection(t *testing.T) {
	tests := []struct {
		err      error
		tcp, rtu bool
	}{
		{sv.ErrIllegalDataAddress, true, true},
		{sv.ErrServerDeviceBusy, true, true},
		{sv.ErrAcknowledge, true, true},
		{sv.ErrGWTargetFailedToRespond, true, true},
		{sv.ErrBadCRC, false, true},
		{sv.ErrShortFrame, false, true},
		{sv.ErrProtocolError, false, true},
		{sv.ErrRequestTimedOut, false, true},
		{sv.ErrBadTransactionId, false, false},
		{errors.New("broken pipe"), false, false},
	}
	tcp := &Client{}
	rtu := &Client{rtu: true}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			assert.Equal(t, tt.tcp, tcp.keepsConnection(tt.err), "tcp")
			assert.Equal(t, tt.rtu, rtu.keepsConnection(tt.err), "rtu")
		})
	}
}

func TestReadRegisters_ExceptionKeepsConnection(t *testing.T) {
	port := freePort(t)
	srv := startDevice(t, port)
	defer func() { _ = srv.Stop() }()
	c, err := New(settings(port), util.NewRealClock(), zerolog.Nop())
	require.NoError(t, err)
	require.NoError(t, c.connect())
	defer func() { _ = c.Close() }()

	_, err = c.ReadRegisters(context.Background(), illegalAddr, 1)
	require.ErrorIs(t, err, sv.ErrIllegalDataAddress)
	assert.True(t, c.IsConnected(), "an exception reply is a valid frame")
	regs, err := c.ReadRegisters(context.Background(), 5, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{5, 6}, regs)
}

func TestSettingsValidate_Serial(t *testing.T) {
	base := Settings{Address: "rtu:///dev/ttyUSB0", Timeout: time.Second}
	ok := base
	ok.DataBits, ok.StopBits = 7, 1
	assert.NoError(t, ok.Validate())
	for _, s := range []Settings{
		{Address: base.Address, Timeout: 1, DataBits: 4},
		{Address: base.Address, Timeout: 1, DataBits: 9},
		{Address: base.Address, Timeout: 1, StopBits: 3},
		{Address: "tcp://h:0", Timeout: 1},
		{Address: "tcp://h:x", Timeout: 1},
	} {
		assert.ErrorIs(t, s.Validate(), ErrInvalidSettings, s)
	}
}

// On RTU an open port says nothing about the inverter: 5 line errors in a row mark the
// connection lost; an exception reply (the device answered) resets the count (ACQ-M3).
func TestTooManyLineFailures(t *testing.T) {
	c := &Client{rtu: true}
	for range maxLineFailures - 1 {
		assert.False(t, c.tooManyLineFailures(sv.ErrRequestTimedOut))
	}
	assert.False(t, c.tooManyLineFailures(sv.ErrIllegalDataAddress), "a reply resets")
	for range maxLineFailures - 1 {
		assert.False(t, c.tooManyLineFailures(sv.ErrBadCRC))
	}
	assert.True(t, c.tooManyLineFailures(sv.ErrShortFrame))

	tcp := &Client{}
	for range 2 * maxLineFailures {
		assert.False(t, tcp.tooManyLineFailures(sv.ErrRequestTimedOut), "tcp drops at once")
	}
}
