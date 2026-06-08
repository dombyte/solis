package modbus

import (
	"context"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/grid-x/modbus"
)

// NewRTUClient creates a new Modbus RTU client wrapper.
func NewRTUClient(cfg *config.ModbusSettings, opts ...ClientOption) (*Client, error) {
	logger.Info().Msgf("Creating new Modbus RTU client for %s", cfg.SerialPort)

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

	// Create RTU handler
	address := fmt.Sprintf("%s?baudrate=%d&databits=%d&stopbits=%d&parity=%s",
		cfg.SerialPort, cfg.BaudRate, cfg.DataBits, cfg.StopBits, cfg.Parity)
	handler := modbus.NewRTUClientHandler(address)
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

	logger.Info().Msg("Modbus RTU client created successfully")
	return c, nil
}
