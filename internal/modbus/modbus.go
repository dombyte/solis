// Package modbus is the Modbus TCP client used only by the poller. It is an external
// layer: it depends on the standard library and simonvetter/modbus only (no config,
// health or logging globals). Construction never touches the network; Run keeps the
// single connection alive with exponential backoff and beats while it waits.
package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	sv "github.com/simonvetter/modbus"

	"github.com/dombyte/solis/internal/utils"
)

// Reconnect backoff bounds.
const (
	InitialBackoff = time.Second
	MaxBackoff     = 30 * time.Second
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

// Settings are the connection parameters.
type Settings struct {
	// Host is the device host name or IP.
	Host string
	// Port is the TCP port.
	Port int
	// UnitID is the Modbus unit (slave) id.
	UnitID byte
	// Timeout bounds connect and each request.
	Timeout time.Duration
}

// Validate checks the settings.
func (s Settings) Validate() error {
	const maxPort = 65535
	if s.Host == "" || s.Port <= 0 || s.Port > maxPort || s.Timeout <= 0 {
		return fmt.Errorf("%w: host %q port %d timeout %s", ErrInvalidSettings, s.Host, s.Port,
			s.Timeout)
	}
	return nil
}

func (s Settings) url() string {
	return "tcp://" + net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// Client is a single Modbus TCP connection with automatic reconnection.
type Client struct {
	set   Settings
	clock utils.Clock
	log   zerolog.Logger

	mu        sync.Mutex // guards mc
	mc        *sv.ModbusClient
	connected atomic.Bool
	lost      chan struct{} // wakes Run when a read marks the connection lost
}

// New creates a disconnected client; it never fails because the device is unreachable.
func New(set Settings, clock utils.Clock, log zerolog.Logger) (*Client, error) {
	if err := set.Validate(); err != nil {
		return nil, err
	}
	return &Client{set: set, clock: clock, log: log, lost: make(chan struct{}, 1)}, nil
}

// IsConnected reports whether the connection is up.
func (c *Client) IsConnected() bool { return c.connected.Load() }

// ReadRegisters reads count input registers starting at addr. A failed read marks the
// connection lost; the Run loop reconnects.
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
		c.markLost(err)
		return nil, &ReadError{Addr: addr, Count: count, Err: err}
	}
	return regs, nil
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
	mc, err := sv.NewClient(&sv.ClientConfiguration{URL: c.set.url(), Timeout: c.set.Timeout})
	if err != nil {
		return fmt.Errorf("modbus: create client: %w", err)
	}
	if err := mc.SetUnitId(c.set.UnitID); err != nil {
		return fmt.Errorf("modbus: set unit id: %w", err)
	}
	if err := mc.Open(); err != nil {
		return fmt.Errorf("modbus: connect %s: %w", c.set.url(), err)
	}
	c.mu.Lock()
	c.mc = mc
	c.mu.Unlock()
	c.connected.Store(true)
	c.log.Info().Str("url", c.set.url()).Msg("modbus connected")
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
			backoff = min(backoff*2, MaxBackoff)
			continue
		}
		c.log.Debug().Msg("modbus reconnected")
		backoff = InitialBackoff
	}
}

// wait sleeps d, beating every beatEvery, and returns early when ctx is done or (with
// stopOnLost) when a read marked the connection lost.
func (c *Client) wait(ctx context.Context, d, beatEvery time.Duration, beat func(),
	stopOnLost bool) {
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
