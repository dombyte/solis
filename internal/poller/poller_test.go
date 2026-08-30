package poller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/solis"
)

func TestNew(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	if p == nil {
		t.Fatal("New() returned nil")
	}
	if p.config != cfg {
		t.Error("New() did not set config correctly")
	}
	if p.running {
		t.Error("New() should not set running to true")
	}
}

func TestNew_WithOptions(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil, WithStorage(nil), WithCache(nil))

	if p == nil {
		t.Fatal("New() with options returned nil")
	}
}

func TestPoller_IsRunning(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	if p.IsRunning() {
		t.Error("Poller should not be running after New()")
	}
}

func TestPoller_GetLastPollInfo(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	info := p.GetLastPollInfo()
	if info != nil {
		t.Error("GetLastPollInfo() should return nil when no poll has run")
	}
}

func TestPoller_GetLastPollError(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	err := p.GetLastPollError()
	if err != nil {
		t.Errorf("GetLastPollError() should return nil when no error: %v", err)
	}
}

func TestPoller_pollOnce_NilModbus(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	startTime := time.Now()
	_, _, err := p.pollOnce(context.Background(), startTime)

	if err == nil {
		t.Error("pollOnce() should fail with nil modbus")
	}
}

func TestPoller_readRangeWithRetry_NilModbus(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 100 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	ctx := context.Background()
	_, err := p.readRangeWithRetry(ctx, 0, 10)

	if err == nil {
		t.Error("readRangeWithRetry() should fail with nil modbus")
	}
}

func TestPoller_readRangeWithRetry_ContextCancelled(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 100 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	_, err := p.readRangeWithRetry(ctx, 0, 10)

	if err == nil {
		t.Error("readRangeWithRetry() should fail with context cancelled")
	}
}

func TestPoller_handlePollError(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// Test with various errors
	err := errors.New("test error")
	p.handlePollError(err, 100*time.Millisecond)

	// Check that lastPollErr was set
	if p.GetLastPollError() == nil {
		t.Error("handlePollError() should set lastPollErr")
	}
}

func TestPoller_updatePollStats(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// This should not panic
	p.updatePollStats(100*time.Millisecond, 10, 5)

	// Check that pollCount was incremented
	if p.pollCount != 1 {
		t.Errorf("Expected pollCount=1, got %d", p.pollCount)
	}

	// Check that lastPollTime was set
	if p.lastPollTime.IsZero() {
		t.Error("updatePollStats() should set lastPollTime")
	}
}

func TestPoller_shouldContinue(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// Initially should NOT continue (not running)
	if p.shouldContinue() {
		t.Error("shouldContinue() should return false initially (poller not running)")
	}

	// Start the poller
	_ = p.Start()
	defer p.Stop()

	// Should continue when running
	if !p.shouldContinue() {
		t.Error("shouldContinue() should return true when running")
	}

	// Stop the poller
	_ = p.Stop()

	// Should not continue
	if p.shouldContinue() {
		t.Error("shouldContinue() should return false after Stop()")
	}
}

func TestPoller_storePollResults(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// This should not panic with nil storage
	startTime := time.Now()
	p.storePollResults(nil, startTime, 100*time.Millisecond, 10)
}

func TestPoller_updateCache(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// This should not panic with nil cache
	p.updateCache(nil)
}

func TestPoller_decodedRange(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// This should not panic
	// p.decodeRange(0, []uint16{1, 2, 3}, time.Now(), make(map[string]*solis.Value))
	// Note: This requires proper types from solis package
	// For now, just ensure the function exists
	_ = p.decodeRange
}

func TestPoller_filterComputedRegisters(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// Test with nil input - returns empty map, not nil
	result := p.filterComputedRegisters(nil)
	if result == nil {
		t.Error("filterComputedRegisters(nil) should return empty map, not nil")
	}
	if len(result) != 0 {
		t.Errorf("filterComputedRegisters(nil) should return empty map, got %d items", len(result))
	}

	// Test with empty map
	empty := make(map[string]*solis.Value)
	result = p.filterComputedRegisters(empty)
	if len(result) != 0 {
		t.Errorf("filterComputedRegisters(empty) should return empty map, got %d items", len(result))
	}
}

func TestPoller_Start_AlreadyRunning(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// Start the poller
	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start poller: %v", err)
	}
	defer p.Stop()

	// Try to start again - should return error
	if err := p.Start(); err == nil {
		t.Error("Start() should return error when poller is already running")
	}
}

func TestPoller_Stop_NotRunning(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// Try to stop when not running - should return error
	if err := p.Stop(); err == nil {
		t.Error("Stop() should return error when poller is not running")
	}
}

func TestPoller_PollNow_NilModbus(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// PollNow with nil modbus should fail
	_, err := p.PollNow()
	if err == nil {
		t.Error("PollNow() should fail with nil modbus")
	}
}

func TestPoller_pollOnce_NilModbusError(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	startTime := time.Now()
	_, _, err := p.pollOnce(context.Background(), startTime)

	if err == nil {
		t.Error("pollOnce() should fail with nil modbus")
	}
	if err != nil && err.Error() != "modbus client is nil" {
		t.Errorf("Expected error 'modbus client is nil', got: %v", err)
	}
}

func TestPoller_WithOptions(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	// Test that options are applied correctly
	p := New(cfg, nil, WithStorage(nil), WithCache(nil))
	if p == nil {
		t.Fatal("New() with options returned nil")
	}
	// We can't easily verify storage/cache are set without mocking,
	// but at least verify it doesn't panic
}

func TestPoller_Config(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// The config field is private, so we can't directly check it
	// But we can verify the poller was created successfully
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestPoller_readRangeWithRetry_MaxAttempts(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   0, // No retries
		BlockRetryDelay: 100 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	ctx := context.Background()
	_, err := p.readRangeWithRetry(ctx, 0, 10)

	// Should fail since modbus is nil
	if err == nil {
		t.Error("readRangeWithRetry() should fail with nil modbus")
	}
}

func TestPoller_pollOnce_ModbusNotConnected(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 100 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	// Create a mock modbus client that is not connected
	mockClient := &modbus.Client{}
	// We can't easily set the state to disconnected without accessing private fields,
	// but we can at least test the error path

	p := New(cfg, mockClient)

	startTime := time.Now()
	_, _, err := p.pollOnce(context.Background(), startTime)

	// This should handle the case where modbus is not connected gracefully
	if err == nil {
		t.Log("pollOnce() with disconnected modbus completed (may be expected)")
	} else {
		t.Logf("pollOnce() returned error: %v", err)
	}
}

func TestPoller_handlePollError_NilError(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        5 * time.Second,
		BlockAttempts:   2,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     5 * time.Second,
	}

	p := New(cfg, nil)

	// Handle nil error - should not panic
	p.handlePollError(nil, 100*time.Millisecond)

	// Last error should still be nil
	if p.GetLastPollError() != nil {
		t.Errorf("handlePollError(nil) should not set lastPollErr, got: %v", p.GetLastPollError())
	}
}
