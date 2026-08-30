package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
)

func TestNewClient_InvalidType(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type: "invalid",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() expected error for invalid type, got nil")
	}

	expectedMsg := "unsupported modbus type: invalid (only tcp is supported)"
	if err.Error() != expectedMsg {
		t.Errorf("NewClient() error = %v, expected %v", err.Error(), expectedMsg)
	}
}

func TestNewClient_ValidTCP(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	// This will likely fail to connect but should not panic
	client, err := NewClient(cfg)
	if err == nil && client != nil {
		defer client.Close()
	}
	// Test passes as long as it doesn't panic
}

func TestClient_Connect_Disconnect(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping connect test: %v", err)
	}
	defer client.Close()

	// Should already be connected from NewClient
	if !client.IsConnected() {
		// If not, try to connect
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := client.Connect(ctx); err != nil {
			t.Logf("Connect failed (expected for 127.0.0.1:502): %v", err)
		}
	}

	// Test disconnect
	if err := client.Disconnect(); err != nil {
		t.Logf("Disconnect error (may be expected): %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected after Disconnect()")
	}
}

func TestClient_StateTransitions(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping state test: %v", err)
	}
	defer client.Close()

	// Check initial state
	if client.GetState() != Connected {
		t.Logf("Client state: %s (expected Connected)", client.GetState())
	}

	// Disconnect and check state
	client.Disconnect()
	if client.GetState() != Disconnected {
		t.Errorf("Expected Disconnected state after Disconnect(), got: %s", client.GetState())
	}
}

func TestShouldReconnect_AllCases(t *testing.T) {
	// We can't directly test shouldReconnect since it's not exported
	// But we can test classifyError which is the core logic

	tests := []struct {
		name     string
		err      error
		expected bool // Should be reconnectable
	}{
		// Nil
		{"nil error", nil, false},

		// Context errors - using BLACKLIST approach
		// Only context.Canceled is NOT reconnectable
		// context.DeadlineExceeded IS reconnectable (device timeout, should retry)
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped context canceled", fmt.Errorf("wrap: %w", context.Canceled), false},
		{"wrapped deadline exceeded", fmt.Errorf("wrap: %w", context.DeadlineExceeded), true},

		// Network errors - SHOULD be reconnectable
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset by peer", errors.New("connection reset by peer"), true},
		{"EOF", errors.New("EOF"), true},
		{"i/o timeout", errors.New("i/o timeout"), true},
		{"connection timed out", errors.New("connection timed out"), true},
		{"broken pipe", errors.New("broken pipe"), true},
		{"use of closed", errors.New("use of closed network connection"), true},
		{"no such host", errors.New("no such host"), true},
		{"network is down", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("network is down")}, true},
		{"write error", errors.New("write: connection reset by peer"), true},
		{"read error", errors.New("read: broken pipe"), true},

		// Timeout errors - SHOULD be reconnectable
		{"timeout", errors.New("timeout"), true},
		{"request timeout", errors.New("request timeout"), true},
		{"timed out", errors.New("connection timed out"), true},

		// Unknown errors - SHOULD be reconnectable (might be device issues)
		{"unknown error", errors.New("some unknown error"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classified := classifyError(tt.err)
			if classified == nil {
				if tt.expected {
					t.Errorf("classifyError(%v) returned nil, expected reconnectable", tt.err)
				}
				return
			}

			result := classified.IsReconnectable()
			if result != tt.expected {
				t.Errorf("classifyError(%v).IsReconnectable() = %v, want %v",
					tt.err, result, tt.expected)
			}
		})
	}
}

func TestState_String(t *testing.T) {
	states := []struct {
		state    State
		expected string
	}{
		{Disconnected, "disconnected"},
		{Connecting, "connecting"},
		{Connected, "connected"},
		{Error, "error"},
		{State(99), "unknown"},
	}

	for _, tt := range states {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("State(%d).String() = %v, want %v", tt.state, got, tt.expected)
			}
		})
	}
}

func TestModbusError_IsReconnectable(t *testing.T) {
	// Test that ModbusError.IsReconnectable works correctly
	connectionError := &ModbusError{
		Type:    ErrTypeConnection,
		Message: "connection refused",
		Cause:   nil,
	}

	if !connectionError.IsReconnectable() {
		t.Error("ErrTypeConnection should be reconnectable")
	}

	timeoutError := &ModbusError{
		Type:    ErrTypeTimeout,
		Message: "timeout",
		Cause:   nil,
	}

	if !timeoutError.IsReconnectable() {
		t.Error("ErrTypeTimeout should be reconnectable")
	}

	unknownError := &ModbusError{
		Type:    ErrTypeUnknown,
		Message: "unknown",
		Cause:   nil,
	}

	if unknownError.IsReconnectable() {
		t.Error("ErrTypeUnknown should NOT be reconnectable")
	}
}

func TestClient_Disconnect(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping disconnect test: %v", err)
	}
	defer client.Close()

	// Disconnect should not panic
	err = client.Disconnect()
	if err != nil {
		t.Logf("Disconnect error (may be expected): %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected after Disconnect()")
	}

	// Disconnecting when already disconnected should not panic
	err = client.Disconnect()
	if err != nil {
		t.Logf("Second Disconnect error (may be expected): %v", err)
	}
}

func TestClient_GetState(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping GetState test: %v", err)
	}
	defer client.Close()

	// GetState should return a valid state
	state := client.GetState()
	if state != Disconnected && state != Connecting && state != Connected && state != Error {
		t.Errorf("GetState() returned invalid state: %v", state)
	}
}

func TestClient_Close(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping Close test: %v", err)
	}

	// Close should not panic
	err = client.Close()
	if err != nil {
		t.Logf("Close error (may be expected): %v", err)
	}
}

func TestClient_IsConnected(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping IsConnected test: %v", err)
	}
	defer client.Close()

	// IsConnected should return false for unreachable host
	// (since connection failed in NewClient)
	_ = client.IsConnected()
	// Just ensure it doesn't panic
}

func TestClient_Config(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Second,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping config test: %v", err)
	}
	defer client.Close()

	returnedCfg := client.Config()
	if returnedCfg == nil {
		t.Fatal("Config() returned nil")
	}
	if returnedCfg.Host != cfg.Host {
		t.Errorf("Config().Host = %v, want %v", returnedCfg.Host, cfg.Host)
	}
}

func TestClassifyError_Nil(t *testing.T) {
	// Test nil error
	if result := classifyError(nil); result != nil {
		t.Errorf("classifyError(nil) should return nil, got %v", result)
	}
}

func TestClassifyError_ContextCanceled(t *testing.T) {
	// Test context.Canceled - should NOT be reconnectable
	classified := classifyError(context.Canceled)
	if classified == nil {
		t.Fatal("classifyError(context.Canceled) should not return nil")
	}
	if classified.IsReconnectable() {
		t.Error("context.Canceled should NOT be reconnectable")
	}
	if classified.Type != ErrTypeUnknown {
		t.Errorf("Expected ErrTypeUnknown for context.Canceled, got %v", classified.Type)
	}
}

func TestClassifyError_ContextDeadlineExceeded(t *testing.T) {
	// Test context.DeadlineExceeded - SHOULD be reconnectable
	classified := classifyError(context.DeadlineExceeded)
	if classified == nil {
		t.Fatal("classifyError(context.DeadlineExceeded) should not return nil")
	}
	if !classified.IsReconnectable() {
		t.Error("context.DeadlineExceeded SHOULD be reconnectable")
	}
	if classified.Type != ErrTypeConnection {
		t.Errorf("Expected ErrTypeConnection for context.DeadlineExceeded, got %v", classified.Type)
	}
}

func TestClassifyError_NetworkErrors(t *testing.T) {
	// Test various network errors - all should be reconnectable
	networkErrors := []string{
		"connection refused",
		"connection reset by peer",
		"EOF",
		"i/o timeout",
		"broken pipe",
		"use of closed network connection",
	}

	for _, errMsg := range networkErrors {
		err := errors.New(errMsg)
		classified := classifyError(err)
		if classified == nil {
			t.Errorf("classifyError(%s) should not return nil", errMsg)
			continue
		}
		if !classified.IsReconnectable() {
			t.Errorf("Error '%s' should be reconnectable", errMsg)
		}
	}
}

func TestModbusError_Types(t *testing.T) {
	// Test all error types
	errorTypes := []struct {
		errType   ErrorType
		reconnect bool
	}{
		{ErrTypeUnknown, false},
		{ErrTypeConnection, true},
		{ErrTypeTimeout, true},
	}

	for _, tt := range errorTypes {
		modbusErr := &ModbusError{
			Type:    tt.errType,
			Message: "test error",
			Cause:   nil,
		}

		if modbusErr.IsReconnectable() != tt.reconnect {
			t.Errorf("ErrorType %v: IsReconnectable() = %v, want %v",
				tt.errType, modbusErr.IsReconnectable(), tt.reconnect)
		}
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	// Test with invalid type
	cfg := &config.ModbusSettings{
		Type: "invalid_type",
	}
	if _, err := NewClient(cfg); err == nil {
		t.Error("NewClient() should return error for invalid type")
	}
}

func TestState_AllStates(t *testing.T) {
	// Test all state values
	states := []State{
		Disconnected,
		Connecting,
		Connected,
		Error,
		State(99), // unknown
	}

	for _, state := range states {
		// Just ensure String() doesn't panic
		_ = state.String()
	}
}

func TestClient_DoubleDisconnect(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping double disconnect test: %v", err)
	}
	defer client.Close()

	// First disconnect
	err = client.Disconnect()
	if err != nil {
		t.Logf("First Disconnect error (may be expected): %v", err)
	}

	// Second disconnect - should not panic
	err = client.Disconnect()
	if err != nil {
		t.Logf("Second Disconnect error (may be expected): %v", err)
	}
}

func TestClient_DoubleClose(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping double close test: %v", err)
	}

	// First close
	err = client.Close()
	if err != nil {
		t.Logf("First Close error (may be expected): %v", err)
	}

	// Second close - should not panic
	err = client.Close()
	if err != nil {
		t.Logf("Second Close error (may be expected): %v", err)
	}
}

func TestWaitForState_ContextCancelled(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping waitForState test: %v", err)
	}
	defer client.Close()

	// Test waitForState with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// This should return context.Canceled error
	err = client.WaitForConnection(ctx)
	if err == nil {
		t.Error("WaitForConnection should fail with cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

func TestWaitForState_Timeout(t *testing.T) {
	cfg := &config.ModbusSettings{
		Type:    "tcp",
		Host:    "127.0.0.1",
		Port:    502,
		Timeout: 1 * time.Millisecond,
		UnitID:  1,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping waitForState timeout test: %v", err)
	}
	defer client.Close()

	// Test waitForState with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// This should timeout quickly since the device is not connected
	err = client.WaitForConnection(ctx)
	if err == nil {
		t.Error("WaitForConnection should timeout")
	}
	if err != context.DeadlineExceeded {
		t.Logf("WaitForConnection returned error: %v (expected timeout)", err)
	}
}

func TestClassifyError_WrappedErrors(t *testing.T) {
	// Test that wrapped errors are properly classified
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{"wrapped context.Canceled", fmt.Errorf("outer: %w", context.Canceled), ErrTypeUnknown},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("outer: %w", context.DeadlineExceeded), ErrTypeConnection},
		{"double wrapped context.Canceled", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", context.Canceled)), ErrTypeUnknown},
		{"wrapped network error", fmt.Errorf("read failed: %w", errors.New("connection refused")), ErrTypeConnection},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classified := classifyError(tt.err)
			if classified == nil {
				t.Fatal("classifyError should not return nil")
			}
			if classified.Type != tt.expected {
				t.Errorf("Expected type %v, got %v", tt.expected, classified.Type)
			}
		})
	}
}
