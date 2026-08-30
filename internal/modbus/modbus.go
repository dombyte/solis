// Package modbus provides Modbus TCP client functionality for Solis inverter monitoring.
// It uses simonvetter/modbus for low-level operations and provides automatic reconnection
// with exponential backoff for reliable operation.
package modbus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
	"github.com/simonvetter/modbus"
)

// logger is the package-level logger for modbus operations.
var logger = logging.NewComponentLogger("modbus")

// Use simonvetter's built-in RegType constants:
// HOLDING_REGISTER = 0 (Modbus function code 0x03)
// INPUT_REGISTER = 1 (Modbus function code 0x04)

// ErrorType classifies errors for appropriate handling.
type ErrorType int

const (
	// ErrTypeUnknown is for errors that don't fit other categories.
	ErrTypeUnknown ErrorType = iota
	// ErrTypeConnection is for connection-related errors (should trigger reconnection).
	ErrTypeConnection
	// ErrTypeTimeout is for timeout errors (should trigger reconnection).
	ErrTypeTimeout
)

// ModbusError is a structured error with type information.
type ModbusError struct {
	Type    ErrorType
	Message string
	Cause   error
}

// IsReconnectable returns true if this error should trigger a reconnection attempt.
func (e *ModbusError) IsReconnectable() bool {
	return e.Type == ErrTypeConnection || e.Type == ErrTypeTimeout
}

// State represents the connection state.
type State int

const (
	Disconnected State = iota
	Connecting
	Connected
	Error
)

func (s State) String() string {
	switch s {
	case Disconnected:
		return "disconnected"
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// Client is a Modbus TCP client with automatic reconnection support.
type Client struct {
	config *config.ModbusSettings

	// Connection state
	state   State
	stateMu sync.RWMutex

	// simonvetter client (handles actual Modbus communication)
	modbusClient *modbus.ModbusClient

	// Reconnection settings
	reconnectDelay        time.Duration
	initialReconnectDelay time.Duration
	maxReconnectDelay     time.Duration
	maxReconnectAttempts  int

	// Timeout for individual reads (simonvetter uses its own timeout)
	readTimeout time.Duration

	// For managing background reconnection loop
	reconnectCtx    context.Context
	reconnectCancel context.CancelFunc
}

// NewClient creates a new Modbus TCP client.
// It initializes the connection to the device specified in the config.
func NewClient(cfg *config.ModbusSettings) (*Client, error) {
	if cfg.Type != "tcp" {
		return nil, fmt.Errorf("unsupported modbus type: %s (only tcp is supported)", cfg.Type)
	}

	logger.Info().Msgf("Creating Modbus TCP client for %s:%d", cfg.Host, cfg.Port)

	c := &Client{
		config:                cfg,
		state:                 Disconnected,
		reconnectDelay:        1 * time.Second,
		initialReconnectDelay: 1 * time.Second,
		maxReconnectDelay:     30 * time.Second,
		maxReconnectAttempts:  3,
		readTimeout:           cfg.Timeout,
	}

	// Create simonvetter client configuration
	url := fmt.Sprintf("tcp://%s:%d", cfg.Host, cfg.Port)
	clientConfig := &modbus.ClientConfiguration{
		URL:     url,
		Timeout: c.readTimeout,
	}

	modbusClient, err := modbus.NewClient(clientConfig)
	if err != nil {
		logger.Error().Msgf("Failed to create simonvetter modbus client: %v", err)
		return nil, fmt.Errorf("failed to create modbus client: %w", err)
	}

	// Set the unit ID
	if err := modbusClient.SetUnitId(cfg.UnitID); err != nil {
		logger.Error().Msgf("Failed to set unit ID: %v", err)
		return nil, fmt.Errorf("failed to set unit ID: %w", err)
	}

	c.modbusClient = modbusClient

	// Try initial connection
	ctx, cancel := context.WithTimeout(context.Background(), c.readTimeout)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		logger.Warn().Msgf("Initial connection failed (will retry in background): %v", err)
		// Return client anyway - caller can use StartReconnectionLoop
		return c, nil
	}

	logger.Info().Msg("Modbus TCP client created and connected")
	return c, nil
}

// Connect establishes a connection to the Modbus device.
func (c *Client) Connect(ctx context.Context) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.state == Connected {
		logger.Debug().Msg("Already connected")
		return nil
	}

	if c.state == Connecting {
		logger.Debug().Msg("Connection already in progress")
		// Wait for state change
		c.stateMu.Unlock()
		return c.waitForState(ctx, Connected, Error, Disconnected)
	}

	c.setState(Connecting)

	// Close existing connection if any
	if c.modbusClient != nil {
		if err := c.modbusClient.Close(); err != nil {
			logger.Warn().Msgf("Error closing existing connection: %v", err)
		}
	}

	// Create fresh client
	url := fmt.Sprintf("tcp://%s:%d", c.config.Host, c.config.Port)
	clientConfig := &modbus.ClientConfiguration{
		URL:     url,
		Timeout: c.readTimeout,
	}

	modbusClient, err := modbus.NewClient(clientConfig)
	if err != nil {
		c.setState(Error)
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Set unit ID
	if err := modbusClient.SetUnitId(c.config.UnitID); err != nil {
		c.setState(Error)
		return fmt.Errorf("failed to set unit ID: %w", err)
	}

	c.modbusClient = modbusClient

	// Open connection
	if err := modbusClient.Open(); err != nil {
		c.setState(Error)
		return fmt.Errorf("connection failed: %w", err)
	}

	c.setState(Connected)
	logger.Info().Msg("Modbus connection established")
	return nil
}

// Disconnect closes the connection to the Modbus device.
func (c *Client) Disconnect() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.state == Disconnected {
		logger.Debug().Msg("Already disconnected")
		return nil
	}

	if c.modbusClient != nil {
		if err := c.modbusClient.Close(); err != nil {
			logger.Error().Msgf("Error closing connection: %v", err)
			c.setState(Error)
			return err
		}
	}

	c.setState(Disconnected)
	logger.Info().Msg("Modbus connection closed")
	return nil
}

// Close is an alias for Disconnect.
func (c *Client) Close() error {
	return c.Disconnect()
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state == Connected
}

// GetState returns the current connection state.
func (c *Client) GetState() State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// Config returns the Modbus configuration.
func (c *Client) Config() *config.ModbusSettings {
	return c.config
}

// setState changes the state with logging.
func (c *Client) setState(newState State) {
	oldState := c.state
	c.state = newState

	if oldState != newState {
		logger.Info().Msgf("State: %s -> %s", oldState.String(), newState.String())
	}
}

// waitForState blocks until the state changes to one of the desired states or context is canceled.
func (c *Client) waitForState(ctx context.Context, desired ...State) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			c.stateMu.RLock()
			currentState := c.state
			c.stateMu.RUnlock()

			for _, d := range desired {
				if currentState == d {
					return nil
				}
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

// WaitForConnection blocks until the client is connected or context is canceled.
func (c *Client) WaitForConnection(ctx context.Context) error {
	return c.waitForState(ctx, Connected)
}

// classifyError classifies an error for reconnection decisions.
// Uses a BLACKLIST approach: ALL errors are reconnectable EXCEPT context.Canceled.
// This is more robust than whitelisting - network errors, timeouts, broken pipes,
// connection resets, etc. should ALL trigger reconnection attempts.
// Only explicit user cancellation (context.Canceled) should NOT trigger reconnection.
// Note: context.DeadlineExceeded IS reconnectable as it may indicate device timeout.
func classifyError(err error) *ModbusError {
	if err == nil {
		return nil
	}

	// BLACKLIST: Only context.Canceled is NOT reconnectable
	// All other errors (including context.DeadlineExceeded from simonvetter timeouts)
	// are reconnectable
	if errors.Is(err, context.Canceled) {
		return &ModbusError{
			Type:    ErrTypeUnknown,
			Message: "context canceled",
			Cause:   err,
		}
	}

	// All other errors are reconnectable
	// Use ErrTypeConnection as the default (ErrTypeTimeout is also reconnectable)
	return &ModbusError{
		Type:    ErrTypeConnection,
		Message: err.Error(),
		Cause:   err,
	}
}

// ReadRegisters reads a range of input registers from the device.
// It automatically handles reconnection if the connection is lost.
// Uses a single TCP connection for the read operation.
func (c *Client) ReadRegisters(ctx context.Context, address uint16, count uint16) ([]uint16, error) {
	// Check context
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Ensure connected
	if err := c.WaitForConnection(ctx); err != nil {
		return nil, fmt.Errorf("not connected: %w", err)
	}

	return c.readWithRetry(ctx, address, count)
}

// readWithRetry reads registers and handles disconnection errors.
// For reconnectable errors, it marks the client as disconnected and returns an error.
// The background reconnection loop (started separately) handles automatic reconnection.
// The poller handles retry logic with its own configuration.
func (c *Client) readWithRetry(ctx context.Context, address uint16, count uint16) ([]uint16, error) {
	// Get current client
	c.stateMu.RLock()
	client := c.modbusClient
	c.stateMu.RUnlock()

	// Read registers - simonvetter uses its own internal timeout
	regs, err := client.ReadRegisters(address, count, modbus.INPUT_REGISTER)
	if err == nil {
		return regs, nil
	}

	// Classify the error
	classifiedErr := classifyError(err)

	logger.Debug().Msgf("Read error: %v [classified=%d]", err, classifiedErr.Type)

	// Non-reconnectable error? Return immediately
	if !classifiedErr.IsReconnectable() {
		logger.Warn().Msgf("Non-reconnectable error: %v", err)
		return nil, fmt.Errorf("modbus read failed: %w", err)
	}

	// For reconnectable errors, mark as disconnected
	// The background reconnection loop will handle the reconnection
	c.stateMu.Lock()
	if c.state != Disconnected {
		c.setState(Disconnected)
		if closeErr := c.modbusClient.Close(); closeErr != nil {
			logger.Warn().Msgf("Error closing: %v", closeErr)
		}
	}
	c.stateMu.Unlock()

	// Return error immediately - the poller will handle retries
	// and the background loop will handle reconnection
	return nil, fmt.Errorf("modbus read failed: %w", err)
}

// StartReconnectionLoop starts a background goroutine that monitors and reconnects.
func (c *Client) StartReconnectionLoop(ctx context.Context) {
	c.stateMu.Lock()
	// Stop existing
	if c.reconnectCancel != nil {
		c.reconnectCancel()
	}
	c.reconnectCtx, c.reconnectCancel = context.WithCancel(ctx)
	c.stateMu.Unlock()

	go c.reconnectionLoop()
}

// StopReconnectionLoop stops the background reconnection loop.
func (c *Client) StopReconnectionLoop() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.reconnectCancel != nil {
		c.reconnectCancel()
		c.reconnectCancel = nil
	}
}

// reconnectionLoop runs in background to maintain connection.
func (c *Client) reconnectionLoop() {
	backoff := c.initialReconnectDelay

	for {
		select {
		case <-c.reconnectCtx.Done():
			logger.Info().Msg("Reconnection loop stopped")
			return
		default:
			c.stateMu.RLock()
			connected := c.state == Connected
			c.stateMu.RUnlock()

			if connected {
				time.Sleep(5 * time.Second)
				continue
			}

			logger.Info().Msgf("Background reconnect attempt (backoff: %s)...", backoff)

			connCtx, connCancel := context.WithTimeout(c.reconnectCtx, c.readTimeout)
			if err := c.Connect(connCtx); err != nil {
				connCancel()
				logger.Warn().Msgf("Background reconnect failed: %v", err)

				timer := time.NewTimer(backoff)
				select {
				case <-c.reconnectCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}

				backoff *= 2
				if backoff > c.maxReconnectDelay {
					backoff = c.maxReconnectDelay
				}
			} else {
				connCancel()
				logger.Info().Msg("Background reconnection successful")
				backoff = c.initialReconnectDelay
			}
		}
	}
}
