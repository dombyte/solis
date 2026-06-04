package modbus

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/grid-x/modbus"
)

// withReconnectDelay sets the delay between reconnection attempts.
func withReconnectDelay(delay time.Duration) ClientOption {
	return func(c *Client) {
		c.reconnectDelay = delay
	}
}

// withMaxReconnectAttempts sets the maximum number of reconnection attempts.
func withMaxReconnectAttempts(attempts int) ClientOption {
	return func(c *Client) {
		c.maxReconnectAttempts = attempts
	}
}

// withTimeout sets the connection/read timeout.
func withTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = timeout
	}
}

// MockModbusClient implements modbus.Client interface for testing
type MockModbusClient struct {
	readInputRegistersFunc func(ctx context.Context, address, quantity uint16) ([]byte, error)
	readInputRegistersData []byte
	readInputRegistersErr  error
}

func (m *MockModbusClient) ReadInputRegisters(ctx context.Context, address, quantity uint16) ([]byte, error) {
	if m.readInputRegistersFunc != nil {
		return m.readInputRegistersFunc(ctx, address, quantity)
	}
	return m.readInputRegistersData, m.readInputRegistersErr
}

// Implement all other required Client interface methods as no-ops
func (m *MockModbusClient) ReadCoils(ctx context.Context, address, quantity uint16) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) ReadDiscreteInputs(ctx context.Context, address, quantity uint16) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) WriteSingleCoil(ctx context.Context, address, value uint16) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) WriteMultipleCoils(ctx context.Context, address, quantity uint16, value []byte) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) ReadHoldingRegisters(ctx context.Context, address, quantity uint16) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) WriteSingleRegister(ctx context.Context, address, value uint16) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) WriteMultipleRegisters(ctx context.Context, address, quantity uint16, value []byte) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) ReadWriteMultipleRegisters(ctx context.Context, readAddress, readQuantity, writeAddress, writeQuantity uint16, value []byte) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) MaskWriteRegister(ctx context.Context, address, andMask, orMask uint16) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) ReadFIFOQueue(ctx context.Context, address uint16) ([]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) ReadDeviceIdentification(ctx context.Context, readDeviceIDCode modbus.ReadDeviceIDCode) (map[byte][]byte, error) {
	return nil, nil
}
func (m *MockModbusClient) ReadDeviceIdentificationSpecificObject(ctx context.Context, objectID byte) (map[byte][]byte, error) {
	return nil, nil
}

// MockHandler implements modbus.ClientHandler interface for testing
type MockHandler struct {
	connectFunc  func(ctx context.Context) error
	connectErr   error
	closeFunc    func() error
	closeErr     error
	closed       bool
	setSlaveFunc func(id byte)
}

func (h *MockHandler) Connect(ctx context.Context) error {
	if h.connectFunc != nil {
		return h.connectFunc(ctx)
	}
	return h.connectErr
}

func (h *MockHandler) Close() error {
	if h.closeFunc != nil {
		return h.closeFunc()
	}
	h.closed = true
	return h.closeErr
}

func (h *MockHandler) SetSlave(id byte) {
	if h.setSlaveFunc != nil {
		h.setSlaveFunc(id)
	}
}

// Implement all other required ClientHandler interface methods
func (h *MockHandler) SetTransporter(transporter modbus.Transporter) {}
func (h *MockHandler) Transport() modbus.Transporter                 { return nil }
func (h *MockHandler) Encode(pdu *modbus.ProtocolDataUnit) ([]byte, error) {
	return nil, nil
}
func (h *MockHandler) Decode(adu []byte) (*modbus.ProtocolDataUnit, error) {
	return nil, nil
}
func (h *MockHandler) Verify(aduRequest []byte, aduResponse []byte) error {
	return nil
}
func (h *MockHandler) Send(ctx context.Context, aduRequest []byte) (aduResponse []byte, err error) {
	return nil, nil
}

// ========== Basic Tests ==========

func TestNewClient_InvalidType(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type: "invalid",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() expected error for invalid type, got nil")
	}

	expectedMsg := "unsupported modbus type: invalid"
	if err.Error() != expectedMsg {
		t.Errorf("NewClient() error = %v, expected %v", err.Error(), expectedMsg)
	}
}

func TestClient_Options(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	t.Run("withReconnectDelay", func(t *testing.T) {
		client, err := NewTCPClient(cfg, withReconnectDelay(5*time.Second))
		if err != nil {
			t.Skipf("Skipping withReconnectDelay test: %v", err)
			return
		}
		defer client.Close()
		if client.reconnectDelay != 5*time.Second {
			t.Errorf("withReconnectDelay: got %v, want %v", client.reconnectDelay, 5*time.Second)
		}
	})

	t.Run("withMaxReconnectAttempts", func(t *testing.T) {
		client, err := NewTCPClient(cfg, withMaxReconnectAttempts(10))
		if err != nil {
			t.Skipf("Skipping withMaxReconnectAttempts test: %v", err)
			return
		}
		defer client.Close()
		if client.maxReconnectAttempts != 10 {
			t.Errorf("withMaxReconnectAttempts: got %d, want %d", client.maxReconnectAttempts, 10)
		}
	})

	t.Run("withTimeout", func(t *testing.T) {
		client, err := NewTCPClient(cfg, withTimeout(15*time.Second))
		if err != nil {
			t.Skipf("Skipping withTimeout test: %v", err)
			return
		}
		defer client.Close()
		if client.timeout != 15*time.Second {
			t.Errorf("withTimeout: got %v, want %v", client.timeout, 15*time.Second)
		}
	})
}

// ========== Client Core Function Tests with Mocks ==========

func TestClient_Connect_Success(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		connectErr: nil,
	}

	c := &Client{
		config:               cfg,
		isConnected:          false,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               &MockModbusClient{},
	}

	ctx := context.Background()
	err := c.Connect(ctx)
	if err != nil {
		t.Errorf("Connect() should succeed with mock handler, got: %v", err)
	}

	if !c.IsConnected() {
		t.Error("Client should be connected after successful Connect()")
	}
}

func TestClient_Connect_AlreadyConnected(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
	}

	ctx := context.Background()
	err := c.Connect(ctx)
	if err != nil {
		t.Errorf("Connect() should not return error when already connected, got: %v", err)
	}

	if !c.IsConnected() {
		t.Error("Client should still be connected after Connect() when already connected")
	}
}

func TestClient_Connect_Failure(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		connectErr: errors.New("connection failed"),
	}

	c := &Client{
		config:               cfg,
		isConnected:          false,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
	}

	ctx := context.Background()
	err := c.Connect(ctx)
	if err == nil {
		t.Error("Connect() should return error when handler fails")
	}

	if c.IsConnected() {
		t.Error("Client should not be connected when Connect() fails")
	}
}

func TestClient_Disconnect_Success(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		closeErr: nil,
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
	}

	err := c.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() should succeed, got: %v", err)
	}

	if c.IsConnected() {
		t.Error("Client should not be connected after Disconnect()")
	}

	if !mockHandler.closed {
		t.Error("Handler should have been closed")
	}
}

func TestClient_Disconnect_AlreadyDisconnected(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{}

	c := &Client{
		config:               cfg,
		isConnected:          false,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
	}

	err := c.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() should not return error when already disconnected, got: %v", err)
	}

	if c.IsConnected() {
		t.Error("Client should still not be connected after Disconnect() when already disconnected")
	}

	if mockHandler.closed {
		t.Error("Handler should NOT have been closed when already disconnected")
	}
}

func TestClient_Disconnect_HandlerCloseError(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		closeErr: errors.New("close failed"),
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
	}

	err := c.Disconnect()
	if err == nil {
		t.Error("Disconnect() should return error when handler.Close() fails")
	}

	// When handler.Close() fails, the client remains connected (isConnected is not set to false)
	if !c.IsConnected() {
		t.Error("Client should still be connected when handler.Close() fails (isConnected not updated)")
	}
}

func TestClient_Close_Success(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		closeErr: nil,
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
	}

	err := c.Close()
	if err != nil {
		t.Errorf("Close() should succeed, got: %v", err)
	}

	if c.IsConnected() {
		t.Error("Client should not be connected after Close()")
	}
}

func TestClient_Close_AlreadyClosed(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	c := &Client{
		config:               cfg,
		isConnected:          false,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
	}

	err := c.Close()
	if err != nil {
		t.Errorf("Close() should not return error when already closed, got: %v", err)
	}

	if c.IsConnected() {
		t.Error("Client should still not be connected after Close() when already closed")
	}
}

func TestClient_ReadRegisters_Success(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{}
	mockClient := &MockModbusClient{
		readInputRegistersData: []byte{0x01, 0x02, 0x03, 0x04},
		readInputRegistersErr:  nil,
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               mockClient,
	}

	ctx := context.Background()
	data, err := c.ReadRegisters(ctx, 0, 2)
	if err != nil {
		t.Errorf("ReadRegisters() should succeed, got: %v", err)
	}

	if len(data) != 4 {
		t.Errorf("ReadRegisters() should return 4 bytes, got %d", len(data))
	}

	if data[0] != 0x01 || data[1] != 0x02 || data[2] != 0x03 || data[3] != 0x04 {
		t.Errorf("ReadRegisters() returned wrong data: %v", data)
	}
}

func TestClient_ReadRegisters_ModbusError(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{}
	mockClient := &MockModbusClient{
		readInputRegistersErr: errors.New("modbus read error"),
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 0,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               mockClient,
	}

	ctx := context.Background()
	_, err := c.ReadRegisters(ctx, 0, 2)
	if err == nil {
		t.Error("ReadRegisters() should return error when modbus client fails")
	}
}

func TestClient_ReadRegisters_WithReconnection(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	// First read fails, second read succeeds
	callCount := 0
	mockClient := &MockModbusClient{
		readInputRegistersFunc: func(ctx context.Context, address, quantity uint16) ([]byte, error) {
			callCount++
			if callCount == 1 {
				// First call fails with a reconnectable error
				return nil, errors.New("connection reset by peer")
			}
			// Second call succeeds
			return []byte{0x01, 0x02}, nil
		},
	}

	// Mock handler for reconnection
	mockHandler := &MockHandler{
		connectFunc: func(ctx context.Context) error {
			return nil
		},
		closeFunc: func() error {
			return nil
		},
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       100 * time.Millisecond,
		maxReconnectAttempts: 2,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               mockClient,
	}

	ctx := context.Background()
	data, err := c.ReadRegisters(ctx, 0, 2)
	if err != nil {
		t.Errorf("ReadRegisters() with reconnection should succeed, got: %v", err)
	}

	if len(data) != 2 {
		t.Errorf("ReadRegisters() should return 2 bytes, got %d", len(data))
	}

	if callCount != 2 {
		t.Errorf("ReadInputRegisters should be called 2 times (1 fail + 1 success), got %d", callCount)
	}
}

func TestClient_ReadRegisters_ReconnectionFailure(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	// All reads fail
	mockClient := &MockModbusClient{
		readInputRegistersErr: errors.New("connection reset by peer"),
	}

	// Mock handler that fails on reconnect
	mockHandler := &MockHandler{
		connectFunc: func(ctx context.Context) error {
			return errors.New("reconnection failed")
		},
		closeFunc: func() error {
			return nil
		},
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       100 * time.Millisecond,
		maxReconnectAttempts: 2,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               mockClient,
	}

	ctx := context.Background()
	_, err := c.ReadRegisters(ctx, 0, 2)
	if err == nil {
		t.Error("ReadRegisters() should return error when reconnection fails")
	}
}

func TestClient_ReadRegisters_Reconnection_AllTypes(t *testing.T) {
	types := []string{"tcp", "rtu", "rtu_over_tcp"}

	for _, modbusType := range types {
		t.Run(modbusType, func(t *testing.T) {
			cfg := &config.ModbusSettings{
				Type:    modbusType,
				Timeout: 1 * time.Second,
				UnitID:  1,
			}

			switch modbusType {
			case "tcp":
				cfg.Host = "127.0.0.1"
				cfg.Port = 502
			case "rtu":
				cfg.SerialPort = "/dev/ttyUSB0"
				cfg.BaudRate = 9600
				cfg.DataBits = 8
				cfg.StopBits = 1
				cfg.Parity = "none"
			case "rtu_over_tcp":
				cfg.Host = "127.0.0.1"
				cfg.Port = 502
			}

			// First read fails, second read succeeds
			callCount := 0
			mockClient := &MockModbusClient{
				readInputRegistersFunc: func(ctx context.Context, address, quantity uint16) ([]byte, error) {
					callCount++
					if callCount == 1 {
						// First call fails with a reconnectable error
						return nil, errors.New("connection reset by peer")
					}
					// Second call succeeds
					return []byte{0x01, 0x02}, nil
				},
			}

			// Mock handler for reconnection
			mockHandler := &MockHandler{
				connectFunc: func(ctx context.Context) error {
					return nil
				},
				closeFunc: func() error {
					return nil
				},
			}

			c := &Client{
				config:               cfg,
				isConnected:          true,
				reconnectDelay:       100 * time.Millisecond,
				maxReconnectAttempts: 2,
				timeout:              1 * time.Second,
				handler:              mockHandler,
				client:               mockClient,
			}

			ctx := context.Background()
			data, err := c.ReadRegisters(ctx, 0, 2)
			if err != nil {
				t.Errorf("ReadRegisters() with reconnection should succeed for %s, got: %v", modbusType, err)
			}

			if len(data) != 2 {
				t.Errorf("ReadRegisters() should return 2 bytes for %s, got %d", modbusType, len(data))
			}

			if callCount != 2 {
				t.Errorf("ReadInputRegisters should be called 2 times for %s, got %d", modbusType, callCount)
			}
		})
	}
}

func TestClient_ReadRegisters_NonReconnectableError(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	// Non-reconnectable error
	mockClient := &MockModbusClient{
		readInputRegistersErr: errors.New("invalid function code"),
	}

	mockHandler := &MockHandler{}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       100 * time.Millisecond,
		maxReconnectAttempts: 2,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               mockClient,
	}

	ctx := context.Background()
	_, err := c.ReadRegisters(ctx, 0, 2)
	if err == nil {
		t.Error("ReadRegisters() should return error for non-reconnectable error")
	}

	// Should not attempt reconnection for non-reconnectable error
	// Only 1 attempt should be made
}

func TestClient_ReadRegisters_NotConnected(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Nanosecond,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		connectErr: errors.New("connection failed"),
	}

	mockClient := &MockModbusClient{
		readInputRegistersErr: errors.New("read failed"),
	}

	c := &Client{
		config:               cfg,
		isConnected:          false,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 0,
		timeout:              1 * time.Nanosecond,
		handler:              mockHandler,
		client:               mockClient,
	}

	ctx := context.Background()
	_, err := c.ReadRegisters(ctx, 0, 2)
	if err == nil {
		t.Error("ReadRegisters() should return error when not connected and connection fails")
	}
}

func TestClient_IsConnected_Unit(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	t.Run("connected", func(t *testing.T) {
		c := &Client{
			config:               cfg,
			isConnected:          true,
			reconnectDelay:       1 * time.Second,
			maxReconnectAttempts: 3,
			timeout:              1 * time.Second,
		}

		if !c.IsConnected() {
			t.Error("IsConnected() should return true for connected client")
		}
	})

	t.Run("disconnected", func(t *testing.T) {
		c := &Client{
			config:               cfg,
			isConnected:          false,
			reconnectDelay:       1 * time.Second,
			maxReconnectAttempts: 3,
			timeout:              1 * time.Second,
		}

		if c.IsConnected() {
			t.Error("IsConnected() should return false for disconnected client")
		}
	})
}

func TestShouldReconnect_AllCases(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	c := &Client{
		config:               cfg,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
	}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset by peer", errors.New("connection reset by peer"), true},
		{"EOF", errors.New("EOF"), true},
		{"i/o timeout", errors.New("i/o timeout"), true},
		{"connection timed out", errors.New("connection timed out"), true},
		{"broken pipe", errors.New("broken pipe"), true},
		{"no such host", errors.New("no such host"), true},
		{"unknown error", errors.New("some other error"), false},
		{"timeout", errors.New("timeout"), true},
		{"timeout in message", errors.New("request timeout"), true},
		{"network is down", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("network is down")}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.shouldReconnect(tt.err)
			if result != tt.expected {
				t.Errorf("shouldReconnect(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// mockNetError is a custom net.Error implementation for testing
type mockNetError struct {
	timeoutValue   bool
	temporaryValue bool
	msg            string
}

func (e *mockNetError) Error() string {
	return e.msg
}
func (e *mockNetError) Timeout() bool {
	return e.timeoutValue
}
func (e *mockNetError) Temporary() bool {
	return e.temporaryValue
}

// TestShouldReconnect_NetError tests the net.Error handling in shouldReconnect
func TestShouldReconnect_NetError(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	c := &Client{
		config:               cfg,
		reconnectDelay:       1 * time.Second,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
	}

	t.Run("net.Error with Timeout true", func(t *testing.T) {
		err := &mockNetError{
			timeoutValue:   true,
			temporaryValue: false,
			msg:            "custom timeout error",
		}
		result := c.shouldReconnect(err)
		if !result {
			t.Errorf("shouldReconnect() should return true for net.Error with Timeout()=true, got false")
		}
	})

	t.Run("net.Error with Temporary true", func(t *testing.T) {
		err := &mockNetError{
			timeoutValue:   false,
			temporaryValue: true,
			msg:            "custom temporary error",
		}
		result := c.shouldReconnect(err)
		if !result {
			t.Errorf("shouldReconnect() should return true for net.Error with Temporary()=true, got false")
		}
	})

	t.Run("net.Error with both Timeout and Temporary true", func(t *testing.T) {
		err := &mockNetError{
			timeoutValue:   true,
			temporaryValue: true,
			msg:            "custom error",
		}
		result := c.shouldReconnect(err)
		if !result {
			t.Errorf("shouldReconnect() should return true for net.Error with both true, got false")
		}
	})

	t.Run("net.Error with both false", func(t *testing.T) {
		err := &mockNetError{
			timeoutValue:   false,
			temporaryValue: false,
			msg:            "custom error",
		}
		result := c.shouldReconnect(err)
		if result {
			t.Errorf("shouldReconnect() should return false for net.Error with both false, got true")
		}
	})
}

// ========== Integration Tests (with real client creation) ==========

func TestNewClient_AllTypes(t *testing.T) {
	testCases := []struct {
		name string
		cfg  *config.ModbusSettings
	}{
		{
			name: "tcp",
			cfg: &config.ModbusSettings{
				Type:    "tcp",
				Host:    "192.168.1.100",
				Port:    502,
				Timeout: 1 * time.Millisecond,
				UnitID:  1,
			},
		},
		{
			name: "rtu",
			cfg: &config.ModbusSettings{
				Type:       "rtu",
				SerialPort: "/dev/ttyUSB0",
				BaudRate:   9600,
				DataBits:   8,
				StopBits:   1,
				Parity:     "none",
				Timeout:    1 * time.Millisecond,
				UnitID:     1,
			},
		},
		{
			name: "rtu_over_tcp",
			cfg: &config.ModbusSettings{
				Type:    "rtu_over_tcp",
				Host:    "192.168.1.100",
				Port:    502,
				Timeout: 1 * time.Millisecond,
				UnitID:  1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClient(tc.cfg)
			if err == nil {
				t.Logf("NewClient(%s) succeeded unexpectedly", tc.name)
			} else {
				t.Logf("NewClient(%s) failed as expected: %v", tc.name, err)
				// For RTU over TCP, verify the address format bug is fixed
				if tc.name == "rtu_over_tcp" {
					errStr := err.Error()
					if errStr == "initial connection failed: connection failed: dial tcp: address tcp://192.168.1.100:502: too many colons in address" {
						t.Error("RTU over TCP should not have 'too many colons in address' error - this indicates the tcp:// prefix bug is back")
					}
				}
			}
		})
	}
}

func TestNewClient_InvalidTypeError(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type: "invalid_type",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient should return error for invalid type")
	}

	expectedErr := "unsupported modbus type: invalid_type"
	if err.Error() != expectedErr {
		t.Errorf("Expected 'unsupported modbus type: invalid_type', got: %v", err.Error())
	}
}

func TestClient_Close_AllTypes(t *testing.T) {
	types := []string{"tcp", "rtu", "rtu_over_tcp"}

	for _, modbusType := range types {
		t.Run(modbusType, func(t *testing.T) {
			cfg := &config.ModbusSettings{
				Type:    modbusType,
				Timeout: 1 * time.Millisecond,
				UnitID:  1,
			}

			switch modbusType {
			case "tcp":
				cfg.Host = "192.168.1.100"
				cfg.Port = 502
			case "rtu":
				cfg.SerialPort = "/dev/ttyUSB0"
				cfg.BaudRate = 9600
				cfg.DataBits = 8
				cfg.StopBits = 1
				cfg.Parity = "none"
			case "rtu_over_tcp":
				cfg.Host = "192.168.1.100"
				cfg.Port = 502
			}

			client, err := NewClient(cfg)
			if err != nil {
				t.Skipf("Skipping Close test for %s: %v", modbusType, err)
			}

			err = client.Close()
			if err != nil {
				t.Logf("Client.Close() error for %s: %v", modbusType, err)
			}

			// Test double close
			err = client.Close()
			if err != nil {
				t.Logf("Client.Close() second call error for %s: %v", modbusType, err)
			}
		})
	}
}

func TestClientOptions_MultipleOptions(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewTCPClient(
		cfg,
		withReconnectDelay(3*time.Second),
		withMaxReconnectAttempts(5),
		withTimeout(10*time.Second),
	)
	if err != nil {
		t.Skipf("Skipping multiple options test: %v", err)
		return
	}
	defer client.Close()

	if client.reconnectDelay != 3*time.Second {
		t.Errorf("reconnectDelay: got %v, want %v", client.reconnectDelay, 3*time.Second)
	}
	if client.maxReconnectAttempts != 5 {
		t.Errorf("maxReconnectAttempts: got %d, want %d", client.maxReconnectAttempts, 5)
	}
	if client.timeout != 10*time.Second {
		t.Errorf("timeout: got %v, want %v", client.timeout, 10*time.Second)
	}
}

// TestReconnect_AllConnectionTypes tests the reconnect function for all connection types
// This tests that the reconnect function creates the correct handler type based on config
func TestReconnect_AllConnectionTypes(t *testing.T) {
	testCases := []struct {
		name        string
		cfg         *config.ModbusSettings
		expectError bool // Will error because we can't connect to real devices
	}{
		{
			name: "tcp",
			cfg: &config.ModbusSettings{
				Type:    "tcp",
				Host:    "192.168.1.100",
				Port:    502,
				Timeout: 1 * time.Millisecond,
				UnitID:  1,
			},
			expectError: true,
		},
		{
			name: "rtu",
			cfg: &config.ModbusSettings{
				Type:       "rtu",
				SerialPort: "/dev/ttyUSB0",
				BaudRate:   9600,
				DataBits:   8,
				StopBits:   1,
				Parity:     "none",
				Timeout:    1 * time.Millisecond,
				UnitID:     1,
			},
			expectError: true,
		},
		{
			name: "rtu_over_tcp",
			cfg: &config.ModbusSettings{
				Type:    "rtu_over_tcp",
				Host:    "192.168.1.100",
				Port:    502,
				Timeout: 1 * time.Millisecond,
				UnitID:  1,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a client with a dummy handler so handler.Close() doesn't panic
			mockHandler := &MockHandler{
				closeFunc: func() error {
					return nil
				},
			}

			c := &Client{
				config:               tc.cfg,
				isConnected:          true,
				reconnectDelay:       10 * time.Millisecond,
				maxReconnectAttempts: 3,
				timeout:              1 * time.Second,
				handler:              mockHandler,
				client:               &MockModbusClient{},
			}

			// Lock the mutex as required by reconnect
			c.mu.Lock()

			ctx := context.Background()
			err := c.reconnect(ctx)

			c.mu.Unlock()

			// These should all fail because we're trying to connect to non-existent devices
			// But we're testing that the code path works correctly
			if tc.expectError && err == nil {
				t.Errorf("reconnect() should fail for %s (no real device), got nil", tc.name)
			}
		})
	}
}

// TestReconnect_ContextCancellation tests reconnect with context cancellation
func TestReconnect_ContextCancellation(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		closeFunc: func() error {
			return nil
		},
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       1 * time.Second, // Long delay
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               &MockModbusClient{},
	}

	c.mu.Lock()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel context immediately
	cancel()

	err := c.reconnect(ctx)

	c.mu.Unlock()

	if err != context.Canceled {
		t.Errorf("reconnect() should return context.Canceled, got: %v", err)
	}
}

// TestReconnect_AlreadyConnected tests reconnect when already connected
func TestReconnect_AlreadyConnected(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "192.168.1.100",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		closeFunc: func() error {
			return nil
		},
	}

	c := &Client{
		config:               cfg,
		isConnected:          true,
		reconnectDelay:       10 * time.Millisecond,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               &MockModbusClient{},
	}

	c.mu.Lock()

	ctx := context.Background()
	_ = c.reconnect(ctx)

	c.mu.Unlock()

	// The test is that it doesn't panic and the code path is executed
	// We can't easily verify Close() was called because it creates a new handler
}

// TestReconnect_NotConnected tests reconnect when not already connected
func TestReconnect_NotConnected(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "192.168.1.100",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	mockHandler := &MockHandler{
		closeFunc: func() error {
			return nil
		},
	}

	c := &Client{
		config:               cfg,
		isConnected:          false,
		reconnectDelay:       10 * time.Millisecond,
		maxReconnectAttempts: 3,
		timeout:              1 * time.Second,
		handler:              mockHandler,
		client:               &MockModbusClient{},
	}

	c.mu.Lock()

	ctx := context.Background()
	_ = c.reconnect(ctx)

	c.mu.Unlock()

	// The test is that it doesn't panic and the code path is executed
	// When not connected, it skips closing the handler
}

// TestReconnect_Success tests successful reconnection - skipped as it requires real device
func TestReconnect_Success(t *testing.T) {
	t.Skip("Skipping - requires real Modbus device")
}
