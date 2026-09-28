// Package modbus is the Modbus client (TCP or RTU) used only by the poller. It is an
// external layer: it depends on the standard library and simonvetter/modbus only (no
// config, health or logging globals). Construction never touches the network/serial
// line; Run keeps the single connection alive with exponential backoff and beats while
// it waits.
package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	sv "github.com/simonvetter/modbus"

	"github.com/dombyte/solis/internal/util"
)

// Reconnect backoff bounds; the delay doubles per failed attempt up to MaxBackoff.
const (
	InitialBackoff = time.Second
	MaxBackoff     = 30 * time.Second
	backoffFactor  = 2
)

// Settings bounds.
const (
	maxPort     = 65535
	minDataBits = 5
	maxDataBits = 8
	maxStopBits = 2
)

var (
	// ErrNotConnected is returned by reads while the connection is down.
	ErrNotConnected = errors.New("modbus: not connected")
	// ErrInvalidSettings is returned by Validate.
	ErrInvalidSettings = errors.New("modbus: invalid settings")
)

// ReadError describes a failed register read.
type ReadError struct {
	// Addr is the first register address.
	Addr uint16
	// Count is the number of registers.
	Count uint16
	// Err is the underlying error.
	Err error
}

func (e *ReadError) Error() string {
	return fmt.Sprintf("modbus: read %d registers at %d: %v", e.Count, e.Addr, e.Err)
}

// Unwrap returns the underlying error.
func (e *ReadError) Unwrap() error { return e.Err }

// Settings are the connection parameters. Address selects the transport via URL scheme:
// "tcp://host:port" or "rtu://<serial device path>" (e.g. "rtu:///dev/ttyUSB0"). Speed,
// DataBits, Parity and StopBits apply to rtu only; zero/empty values fall back to the
// library defaults (19200 8N2).
type Settings struct {
	// Address is the connection URL, e.g. "tcp://192.168.1.100:502" or "rtu:///dev/ttyUSB0".
	Address string
	// UnitID is the Modbus unit (slave) id.
	UnitID byte
	// Timeout bounds connect and each request.
	Timeout time.Duration
	// Speed is the serial link speed in bps (rtu only).
	Speed uint
	// DataBits is the number of bits per serial character (rtu only).
	DataBits uint
	// Parity is the serial link parity: "N", "E", or "O" (rtu only, default "N").
	Parity string
	// StopBits is the number of serial stop bits (rtu only).
	StopBits uint
}

// scheme returns the URL scheme of Address ("tcp" or "rtu"), and the remainder.
func (s Settings) scheme() (scheme, rest string, ok bool) {
	scheme, rest, ok = strings.Cut(s.Address, "://")
	return scheme, rest, ok && rest != ""
}

// Validate checks the settings.
func (s Settings) Validate() error {
	scheme, _, ok := s.scheme()
	if !ok || (scheme != "tcp" && scheme != "rtu") {
		return fmt.Errorf("%w: address %q must be tcp://host:port or rtu://<device>",
			ErrInvalidSettings, s.Address)
	}
	if s.Timeout <= 0 {
		return fmt.Errorf("%w: timeout %s must be positive", ErrInvalidSettings, s.Timeout)
	}
	if scheme == "tcp" {
		if err := validateHostPort(s.Address); err != nil {
			return err
		}
	}
	return s.validateSerial()
}

// validateHostPort checks the host:port part of a tcp:// address.
func validateHostPort(address string) error {
	_, hostport, _ := strings.Cut(address, "://")
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil || host == "" {
		return fmt.Errorf("%w: tcp address %q: host:port required", ErrInvalidSettings,
			address)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > maxPort {
		return fmt.Errorf("%w: tcp port %q must be 1-%d", ErrInvalidSettings, portStr,
			maxPort)
	}
	return nil
}

// validateSerial range-checks the rtu line settings; zero keeps the library default.
func (s Settings) validateSerial() error {
	if _, err := parity(s.Parity); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSettings, err)
	}
	if s.DataBits != 0 && (s.DataBits < minDataBits || s.DataBits > maxDataBits) {
		return fmt.Errorf("%w: data_bits %d must be %d-%d", ErrInvalidSettings, s.DataBits,
			minDataBits, maxDataBits)
	}
	if s.StopBits > maxStopBits {
		return fmt.Errorf("%w: stop_bits %d must be 1 or 2", ErrInvalidSettings, s.StopBits)
	}
	return nil
}

// parity maps a config parity letter to the simonvetter/modbus constant.
func parity(p string) (uint, error) {
	switch strings.ToUpper(p) {
	case "", "N":
		return sv.PARITY_NONE, nil
	case "E":
		return sv.PARITY_EVEN, nil
	case "O":
		return sv.PARITY_ODD, nil
	default:
		return 0, fmt.Errorf("invalid parity %q: must be N, E, or O", p)
	}
}

// Client is a single Modbus connection (TCP or RTU) with automatic reconnection.
type Client struct {
	set   Settings
	rtu   bool
	clock util.Clock
	log   zerolog.Logger

	mu        sync.Mutex // guards mc
	mc        *sv.ModbusClient
	connected atomic.Bool
	lost      chan struct{} // wakes Run when a read marks the connection lost
}

// New creates a disconnected client; it never fails because the device is unreachable.
func New(set Settings, clock util.Clock, log zerolog.Logger) (*Client, error) {
	if err := set.Validate(); err != nil {
		return nil, err
	}
	scheme, _, _ := set.scheme()
	return &Client{
		set: set, rtu: scheme == "rtu", clock: clock, log: log,
		lost: make(chan struct{}, 1),
	}, nil
}

// IsConnected reports whether the connection is up.
func (c *Client) IsConnected() bool { return c.connected.Load() }

// ReadRegisters reads count input registers starting at addr. Only a transport failure
// marks the connection lost (the Run loop reconnects); a Modbus exception reply or, on
// RTU, a garbled/missing frame keeps it open so the caller can retry the block.
func (c *Client) ReadRegisters(ctx context.Context, addr, count uint16) ([]uint16, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	mc := c.mc
	c.mu.Unlock()
	if mc == nil || !c.connected.Load() {
		return nil, ErrNotConnected
	}
	regs, err := mc.ReadRegisters(addr, count, sv.INPUT_REGISTER)
	if err != nil {
		if !c.keepsConnection(err) {
			c.markLost(err)
		}
		return nil, &ReadError{Addr: addr, Count: count, Err: err}
	}
	return regs, nil
}

// keepsConnection reports whether err leaves the link usable. Exception replies are
// complete protocol frames. On RTU the library already resyncs the line after a bad CRC,
// short frame or protocol error, and a timeout leaves nothing buffered; on TCP those mean
// the stream may be out of step, so the connection is dropped.
func (c *Client) keepsConnection(err error) bool {
	var me sv.Error
	if !errors.As(err, &me) {
		return false
	}
	switch me {
	case sv.ErrIllegalFunction, sv.ErrIllegalDataAddress, sv.ErrIllegalDataValue,
		sv.ErrServerDeviceFailure, sv.ErrAcknowledge, sv.ErrServerDeviceBusy,
		sv.ErrMemoryParityError, sv.ErrGWPathUnavailable, sv.ErrGWTargetFailedToRespond,
		sv.ErrBadUnitId, sv.ErrUnexpectedParameters:
		return true
	case sv.ErrBadCRC, sv.ErrShortFrame, sv.ErrProtocolError, sv.ErrRequestTimedOut:
		return c.rtu
	default:
		return false
	}
}

func (c *Client) markLost(cause error) {
	if c.connected.CompareAndSwap(true, false) {
		c.log.Warn().Err(cause).Msg("modbus connection lost")
		c.closeConn()
		select {
		case c.lost <- struct{}{}:
		default:
		}
	}
}

// connect opens a fresh connection.
func (c *Client) connect() error {
	par, err := parity(c.set.Parity)
	if err != nil {
		return fmt.Errorf("modbus: %w", err)
	}
	mc, err := sv.NewClient(&sv.ClientConfiguration{
		URL:      c.set.Address,
		Timeout:  c.set.Timeout,
		Speed:    c.set.Speed,
		DataBits: c.set.DataBits,
		Parity:   par,
		StopBits: c.set.StopBits,
	})
	if err != nil {
		return fmt.Errorf("modbus: create client: %w", err)
	}
	if err := mc.SetUnitId(c.set.UnitID); err != nil {
		return fmt.Errorf("modbus: set unit id: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("modbus: connect %s: %w", c.set.Address, err)
	}
	c.mu.Lock()
	c.mc = mc
	c.mu.Unlock()
	c.connected.Store(true)
	c.log.Info().Str("address", c.set.Address).Msg("modbus connected")
	return nil
}

func (c *Client) closeConn() {
	c.mu.Lock()
	mc := c.mc
	c.mc = nil
	c.mu.Unlock()
	if mc != nil {
		if err := mc.Close(); err != nil {
			c.log.Debug().Err(err).Msg("modbus close")
		}
	}
}

// Close drops the connection.
func (c *Client) Close() error {
	c.connected.Store(false)
	c.closeConn()
	return nil
}

// Run keeps the connection alive until ctx is done. beat is called on every connect
// attempt and at least every beatEvery while waiting (also inside a long backoff), so a
// reconnecting client never looks stale to the supervisor.
func (c *Client) Run(ctx context.Context, beatEvery time.Duration, beat func()) {
	c.log.Debug().Dur("beat_every", beatEvery).Msg("modbus run started")
	if beatEvery <= 0 {
		beatEvery = InitialBackoff
	}
	backoff := InitialBackoff
	for ctx.Err() == nil {
		beat()
		if c.connected.Load() {
			backoff = InitialBackoff
			c.wait(ctx, beatEvery, beatEvery, beat, true)
			continue
		}
		c.log.Debug().Dur("backoff", backoff).Msg("modbus reconnecting")
		if err := c.connect(); err != nil {
			c.log.Warn().Err(err).Dur("backoff", backoff).Msg("modbus reconnect failed")
			c.wait(ctx, backoff, beatEvery, beat, false)
			backoff = min(backoff*backoffFactor, MaxBackoff)
			continue
		}
		c.log.Debug().Msg("modbus reconnected")
		backoff = InitialBackoff
	}
}

// wait sleeps d, beating every beatEvery, and returns early when ctx is done or (with
// stopOnLost) when a read marked the connection lost.
func (c *Client) wait(ctx context.Context, d, beatEvery time.Duration, beat func(),
	stopOnLost bool,
) {
	lost := c.lost
	if !stopOnLost {
		lost = nil
	}
	deadline := c.clock.Now().Add(d)
	for {
		left := deadline.Sub(c.clock.Now())
		if left <= 0 {
			return
		}
		t := c.clock.NewTimer(min(left, beatEvery))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-lost:
			t.Stop()
			return
		case <-t.C():
			beat()
		}
	}
}
