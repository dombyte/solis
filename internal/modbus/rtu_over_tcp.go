package modbus

import (
	"context"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/grid-x/modbus"
)

// NewRTUOverTCPClient creates a new Modbus RTU over TCP client wrapper.
func NewRTUOverTCPClient(cfg *config.ModbusSettings, opts ...ClientOption) (*Client, error) {
	logger.Info().Msgf("Creating new Modbus RTU over TCP client for %s:%d", cfg.Host, cfg.Port)

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

	// Create RTU over TCP handler
	// Note: grid-x/modbus expects just "host:port" format, NOT "tcp://host:port"
	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	handler := modbus.NewRTUOverTCPClientHandler(address)
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
		return nil, fmt.Errorf("initial connection failed: %w", err)
	}

	logger.Info().Msg("Modbus RTU over TCP client created successfully")
	return c, nil
}
