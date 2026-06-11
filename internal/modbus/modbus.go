// Package modbus provides Modbus client implementations (TCP, RTU, RTU over TCP)
// with reconnection handling for the Solis inverter monitoring system.
// It wraps the grid-x/modbus library to provide automatic reconnection and
// simplified register reading.
package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
	"github.com/grid-x/modbus"
)

// logger is the package-level logger for modbus operations.
var logger = logging.NewComponentLogger("modbus")

// Client is a Modbus client wrapper with reconnection support.
type Client struct {
	// config holds the Modbus connection configuration.
	config *config.ModbusSettings
	// client is the underlying grid-x modbus client.
	client modbus.Client
	// handler is the modbus client handler (TCP, RTU, or RTU over TCP).
	handler modbus.ClientHandler
	// isConnected tracks whether the client is currently connected.
	isConnected bool
	// mu protects the client and handler from concurrent access.
	mu sync.RWMutex
	// reconnectDelay is the delay between reconnection attempts.
	reconnectDelay time.Duration
	// maxReconnectAttempts is the maximum number of reconnection attempts.
	maxReconnectAttempts int
	// timeout is the connection/read timeout.
	timeout time.Duration
	// allowDisconnected indicates if the client can be created without initial connection.
	allowDisconnected bool
	// reconnectInProgress tracks if a reconnection is currently in progress.
	reconnectInProgress bool
	// reconnectCtxCancel is used to cancel the background reconnection loop.
	reconnectCtxCancel context.CancelFunc
}

// ClientOption is a function that configures a Client.
type ClientOption func(*Client)

// WithAllowDisconnected allows the client to be created without an initial connection.
// When set to true, client creation will succeed even if the initial connection fails,
// and a background reconnection loop will be started.
func WithAllowDisconnected(allow bool) ClientOption {
	return func(c *Client) {
		c.allowDisconnected = allow
	}
}

// Connect establishes a connection to the Modbus device.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		logger.Debug().Msg("Already connected")
		return nil
	}

	if err := c.handler.Connect(ctx); err != nil {
		logger.Error().Msgf("Connection failed: %v", err)
		return fmt.Errorf("connection failed: %w", err)
	}

	c.isConnected = true
	logger.Info().Msg("Modbus connection established")
	return nil
}

// Disconnect closes the connection to the Modbus device.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected {
		logger.Debug().Msg("Already disconnected")
		return nil
	}

	// Stop any background reconnection attempts
	if c.reconnectCtxCancel != nil {
		c.reconnectCtxCancel()
		c.reconnectCtxCancel = nil
	}

	if err := c.handler.Close(); err != nil {
		logger.Error().Msgf("Error closing connection: %v", err)
		return err
	}

	c.isConnected = false
	logger.Info().Msg("Modbus connection closed")
	return nil
}

// Close is an alias for Disconnect.
func (c *Client) Close() error {
	return c.Disconnect()
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isConnected
}

// Config returns the Modbus configuration.
func (c *Client) Config() *config.ModbusSettings {
	return c.config
}

// ReadRegisters reads a range of input registers from the device.
// It handles reconnection automatically if the connection fails.
// The address is the starting register address (0-based in grid-x/modbus).
// Note: Solis inverter uses 1-based addressing, so we may need to adjust.
func (c *Client) ReadRegisters(ctx context.Context, address uint16, count uint16) ([]byte, error) {
	// Check if context is already done (canceled or timed out)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	c.mu.RLock()
	client := c.client
	isConnected := c.isConnected
	c.mu.RUnlock()

	// Use background context for reconnection attempts to avoid timeout issues
	// but use provided context for the actual read
	if !isConnected {
		if err := c.Connect(ctx); err != nil {
			return nil, fmt.Errorf("not connected: %w", err)
		}
	}

	var rawBytes []byte
	var lastErr error
	attempts := 0
	maxAttempts := c.maxReconnectAttempts + 1 // +1 for initial attempt

	for attempt := range maxAttempts {
		attempts++

		// Use the provided context with timeout for the read
		ctxRead, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()

		rawBytes, lastErr = client.ReadInputRegisters(ctxRead, address, count)

		if lastErr == nil {
			// Success
			logger.Debug().Msgf("Read %d registers from address %d (attempt %d/%d)",
				count, address, attempt+1, maxAttempts)
			return rawBytes, nil
		}

		// Check if error is a network error that might be fixed by reconnection
		if c.shouldReconnect(lastErr) && attempt < maxAttempts-1 {
			c.mu.Lock()
			if reconnectErr := c.reconnect(ctx); reconnectErr != nil {
				c.mu.Unlock()
				lastErr = reconnectErr
				logger.Warn().Msgf("Reconnection failed on attempt %d/%d: %v",
					attempt+1, maxAttempts, lastErr)
				continue
			}
			// Reconnection succeeded, retry the read
			c.mu.Unlock()
			logger.Info().Msgf("Reconnected, retrying read (attempt %d/%d)",
				attempt+2, maxAttempts)
			continue
		}

		// Not a reconnectable error or max attempts reached
		logger.Warn().Msgf("Read failed after %d attempts: %v", attempts, lastErr)
		break
	}

	// Log the error with context
	logger.Error().Msgf("Failed to read %d registers from address %d after %d attempts: %v",
		count, address, attempts, lastErr)

	return nil, fmt.Errorf("modbus read failed after %d attempts: %w", attempts, lastErr)
}

// shouldReconnect determines if an error indicates the connection should be reconnected.
func (c *Client) shouldReconnect(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on context errors - the context is already done
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for common network/connection errors
	errStr := err.Error()

	// Connection refused, timeout, reset, etc.
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout() || netErr.Temporary()
	}

	// For RTU, CRC errors are common and worth retrying (device might be busy)
	if c.config.Type == "rtu" && isCRCEor(err) {
		return true
	}

	// Check for specific error strings
	switch {
	case strings.Contains(errStr, "connection reset"),
		strings.Contains(errStr, "connection refused"),
		strings.Contains(errStr, "connection timed out"),
		strings.Contains(errStr, "i/o timeout"),
		strings.Contains(errStr, "EOF"),
		strings.Contains(errStr, "broken pipe"),
		strings.Contains(errStr, "timeout"),
		strings.Contains(errStr, "no such host"):
		return true
	}

	return false
}

// isCRCEor checks if the error is a Modbus CRC error (common with RTU).
func isCRCEor(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "crc") || strings.Contains(errStr, "CRC")
}

// reconnect attempts to reconnect to the Modbus device.
// Must be called with c.mu held (Lock, not RLock).
func (c *Client) reconnect(ctx context.Context) error {
	logger.Info().Msg("Attempting to reconnect...")

	// First, close existing connection
	if c.isConnected {
		if closeErr := c.handler.Close(); closeErr != nil {
			logger.Warn().Msgf("Error closing existing connection: %v", closeErr)
		}
		c.isConnected = false
	}

	// Wait before reconnecting (context-aware)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(c.reconnectDelay):
	}

	// Recreate handler based on connection type
	switch c.config.Type {
	case "tcp":
		address := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
		c.handler = modbus.NewTCPClientHandler(address)
		c.handler.SetSlave(c.config.UnitID)
		c.client = modbus.NewClient(c.handler)
	case "rtu":
		handler := modbus.NewRTUClientHandler(c.config.SerialPort)
		handler.SetSlave(c.config.UnitID)
		// Set serial configuration directly on the handler
		parity := convertParity(c.config.Parity)
		handler.BaudRate = c.config.BaudRate
		handler.DataBits = c.config.DataBits
		handler.StopBits = c.config.StopBits
		handler.Parity = parity
		// RTU-specific serial port timeouts
		handler.LinkRecoveryTimeout = 10 * time.Second
		handler.IdleTimeout = 120 * time.Second
		handler.ConnectDelay = 100 * time.Millisecond
		c.handler = handler
		c.client = modbus.NewClient(handler)
	case "rtu_over_tcp":
		address := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
		c.handler = modbus.NewRTUOverTCPClientHandler(address)
		c.handler.SetSlave(c.config.UnitID)
		c.client = modbus.NewClient(c.handler)
	}

	// Test the new connection
	if err := c.handler.Connect(ctx); err != nil {
		logger.Error().Msgf("Reconnection failed: %v", err)
		return fmt.Errorf("reconnection failed: %w", err)
	}

	c.isConnected = true
	logger.Info().Msg("Reconnection successful")
	return nil
}

// StartReconnectionLoop starts a background goroutine that continuously attempts
// to reconnect to the Modbus device. It uses exponential backoff and stops when
// the connection is established or the context is cancelled.
func (c *Client) StartReconnectionLoop(ctx context.Context) {
	c.mu.Lock()
	if c.isConnected {
		c.mu.Unlock()
		logger.Debug().Msg("Already connected, not starting reconnection loop")
		return
	}
	if c.reconnectCtxCancel != nil {
		c.reconnectCtxCancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	c.reconnectCtxCancel = cancel
	c.mu.Unlock()

	go func() {
		backoff := 1 * time.Second
		maxBackoff := 30 * time.Second

		for {
			select {
			case <-ctx.Done():
				logger.Info().Msg("Reconnection loop stopped: context cancelled")
				return
			default:
				c.mu.Lock()
				if c.isConnected {
					c.mu.Unlock()
					logger.Info().Msg("Reconnection loop stopped: already connected")
					return
				}
				c.mu.Unlock()

				logger.Info().Msgf("Attempting to reconnect (backoff: %s)...", backoff)

				connCtx, connCancel := context.WithTimeout(context.Background(), c.timeout)
				if err := c.Connect(connCtx); err != nil {
					connCancel()
					logger.Warn().Msgf("Reconnection failed: %v", err)
					// Wait before next attempt
					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
					// Exponential backoff with cap
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				} else {
					connCancel()
					logger.Info().Msg("Reconnection successful!")
					return
				}
			}
		}
	}()
}

// StopReconnectionLoop stops the background reconnection loop.
func (c *Client) StopReconnectionLoop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reconnectCtxCancel != nil {
		c.reconnectCtxCancel()
		c.reconnectCtxCancel = nil
	}
}

// NewClient creates a new Modbus client based on the connection type in config.
func NewClient(cfg *config.ModbusSettings, opts ...ClientOption) (*Client, error) {
	switch cfg.Type {
	case "tcp":
		return NewTCPClient(cfg, opts...)
	case "rtu":
		return NewRTUClient(cfg, opts...)
	case "rtu_over_tcp":
		return NewRTUOverTCPClient(cfg, opts...)
	default:
		return nil, fmt.Errorf("unsupported modbus type: %s", cfg.Type)
	}
}

// convertParity converts parity from config format (none, even, odd, N, E, O) to
// grid-x/serial library format (N, E, O).
func convertParity(parity string) string {
	switch strings.ToUpper(parity) {
	case "N", "NONE", "":
		return "N"
	case "E", "EVEN":
		return "E"
	case "O", "ODD":
		return "O"
	default:
		logger.Warn().Msgf("Unknown parity '%s', defaulting to 'N' (none)", parity)
		return "N"
	}
}
