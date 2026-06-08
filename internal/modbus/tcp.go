package modbus

import (
	"context"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/grid-x/modbus"
)

// NewTCPClient creates a new Modbus TCP client wrapper.
// It initializes the connection and sets up reconnection handling.
func NewTCPClient(cfg *config.ModbusSettings, opts ...ClientOption) (*Client, error) {
	logger.Info().Msgf("Creating new Modbus TCP client for %s:%d", cfg.Host, cfg.Port)

	c := &Client{
		config:               cfg,
		isConnected:          false,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              cfg.Timeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Create TCP handler
	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	handler := modbus.NewTCPClientHandler(address)
	handler.SetSlave(cfg.UnitID)

	// Create client
	client := modbus.NewClient(handler)

	c.mu.Lock()
	c.client = client
	c.handler = handler
	c.mu.Unlock()

	// Test connection with timeout
	connCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	if err := c.Connect(connCtx); err != nil {
		logger.Error().Msgf("Initial connection failed: %v", err)
		if c.allowDisconnected {
			logger.Warn().Msg("AllowDisconnected is true, continuing with disconnected client")
			return c, nil
		}
		return nil, fmt.Errorf("initial connection failed: %w", err)
	}

	logger.Info().Msg("Modbus TCP client created successfully")
	return c, nil
}
