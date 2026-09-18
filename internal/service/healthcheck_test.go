package service

import (
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/poller"
)

// MockPoller is a mock implementation of poller.PollerInterface for testing.
type MockPoller struct {
	isRunning    bool
	lastPollInfo *poller.LastPollInfo
	lastPollErr  error
}

// IsRunning implements poller.PollerInterface.
func (m *MockPoller) IsRunning() bool {
	return m.isRunning
}

// GetLastPollInfo implements poller.PollerInterface.
func (m *MockPoller) GetLastPollInfo() *poller.LastPollInfo {
	return m.lastPollInfo
}

// GetLastPollError implements poller.PollerInterface.
func (m *MockPoller) GetLastPollError() error {
	return m.lastPollErr
}

// TestHealthCheck_StalePoller tests that HealthCheck fails when the poller's last poll
// is older than 3x the configured interval.
func TestHealthCheck_StalePoller(t *testing.T) {
	cfg := &config.AppConfig{
		Poller: config.PollerSettings{
			Interval: 1 * time.Second,
		},
	}

	// Create a mock poller that is running but has a stale last poll time
	// Last poll was 4 seconds ago (older than 3x the 1 second interval)
	mockPoller := &MockPoller{
		isRunning: true,
		lastPollInfo: &poller.LastPollInfo{
			Timestamp:     time.Now().Add(-4 * time.Second),
			DurationMs:    100,
			RegistersRead: 10,
			ValuesStored:  10,
		},
	}

	service := &ReadService{
		config: cfg,
		poller: mockPoller,
	}

	status, err := service.HealthCheck()

	// Should return an error
	if err == nil {
		t.Fatal("HealthCheck() expected error for stale poller, got nil")
	}

	// Check error message contains expected text
	if err.Error() == "" {
		t.Error("HealthCheck() error should not be empty")
	}

	// Check status
	if status["status"] != "degraded" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'degraded'", status["status"])
	}

	if status["poller_stale"] != "true" {
		t.Errorf("HealthCheck() status['poller_stale'] = %v, want 'true'", status["poller_stale"])
	}

	// Check that the error message contains useful information
	if _, ok := status["poller_stale_error"]; !ok {
		t.Error("HealthCheck() status should contain 'poller_stale_error'")
	}
}

// TestHealthCheck_HealthyPoller tests that HealthCheck succeeds when the poller
// is running and the last poll is recent.
func TestHealthCheck_HealthyPoller(t *testing.T) {
	cfg := &config.AppConfig{
		Poller: config.PollerSettings{
			Interval: 1 * time.Second,
		},
	}

	// Create a mock poller that is running with a recent last poll time
	// Last poll was 100ms ago (well within 3x the 1 second interval)
	mockPoller := &MockPoller{
		isRunning: true,
		lastPollInfo: &poller.LastPollInfo{
			Timestamp:     time.Now().Add(-100 * time.Millisecond),
			DurationMs:    100,
			RegistersRead: 10,
			ValuesStored:  10,
		},
	}

	service := &ReadService{
		config: cfg,
		poller: mockPoller,
	}

	status, err := service.HealthCheck()

	if err != nil {
		t.Errorf("HealthCheck() error = %v, expected nil", err)
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}

	if _, ok := status["poller_stale"]; ok {
		t.Error("HealthCheck() status should not contain 'poller_stale' for healthy poller")
	}
}

// TestHealthCheck_PollerNotRunning tests that HealthCheck succeeds when the
// poller is not running (no stale check needed).
func TestHealthCheck_PollerNotRunning(t *testing.T) {
	cfg := &config.AppConfig{
		Poller: config.PollerSettings{
			Interval: 1 * time.Second,
		},
	}

	// Create a mock poller that is NOT running
	mockPoller := &MockPoller{
		isRunning: false,
		lastPollInfo: &poller.LastPollInfo{
			Timestamp:     time.Now().Add(-10 * time.Second), // Very old
			DurationMs:    100,
			RegistersRead: 10,
			ValuesStored:  10,
		},
	}

	service := &ReadService{
		config: cfg,
		poller: mockPoller,
	}

	// HealthCheck should succeed because poller is not running
	// (the stale check only applies when poller is running)
	status, err := service.HealthCheck()

	if err != nil {
		t.Errorf("HealthCheck() error = %v, expected nil", err)
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}
}

// TestHealthCheck_NoLastPollInfo tests that HealthCheck succeeds when
// the poller has no last poll info yet.
func TestHealthCheck_NoLastPollInfo(t *testing.T) {
	cfg := &config.AppConfig{
		Poller: config.PollerSettings{
			Interval: 1 * time.Second,
		},
	}

	// Create a mock poller that is running but has no last poll info
	mockPoller := &MockPoller{
		isRunning:    true,
		lastPollInfo: nil,
	}

	service := &ReadService{
		config: cfg,
		poller: mockPoller,
	}

	// HealthCheck should succeed because there's no last poll info to check
	status, err := service.HealthCheck()

	if err != nil {
		t.Errorf("HealthCheck() error = %v, expected nil", err)
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}
}

// TestHealthCheck_ExactlyAtThreshold tests that HealthCheck succeeds when
// the last poll is exactly at or just under the 3x interval threshold.
func TestHealthCheck_ExactlyAtThreshold(t *testing.T) {
	interval := 1 * time.Second
	cfg := &config.AppConfig{
		Poller: config.PollerSettings{
			Interval: interval,
		},
	}

	// Last poll was 2.999 seconds ago (just under 3x the interval)
	// We use a value slightly less than 3x to avoid timing issues
	mockPoller := &MockPoller{
		isRunning: true,
		lastPollInfo: &poller.LastPollInfo{
			Timestamp:     time.Now().Add(-2999 * time.Millisecond),
			DurationMs:    100,
			RegistersRead: 10,
			ValuesStored:  10,
		},
	}

	service := &ReadService{
		config: cfg,
		poller: mockPoller,
	}

	// HealthCheck should succeed because we use > not >=
	status, err := service.HealthCheck()

	if err != nil {
		t.Errorf("HealthCheck() error = %v, expected nil (under threshold)", err)
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}
}

// TestHealthCheck_JustOverThreshold tests that HealthCheck fails when
// the last poll is just over the 3x interval threshold.
func TestHealthCheck_JustOverThreshold(t *testing.T) {
	interval := 1 * time.Second
	cfg := &config.AppConfig{
		Poller: config.PollerSettings{
			Interval: interval,
		},
	}

	// Last poll was 3 seconds + 1ms ago (just over 3x the interval)
	mockPoller := &MockPoller{
		isRunning: true,
		lastPollInfo: &poller.LastPollInfo{
			Timestamp:     time.Now().Add(-3*interval - 1*time.Millisecond),
			DurationMs:    100,
			RegistersRead: 10,
			ValuesStored:  10,
		},
	}

	service := &ReadService{
		config: cfg,
		poller: mockPoller,
	}

	// HealthCheck should fail
	status, err := service.HealthCheck()

	if err == nil {
		t.Fatal("HealthCheck() expected error for stale poller, got nil")
	}

	if status["status"] != "degraded" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'degraded'", status["status"])
	}
}

// TestHealthCheck_NilConfig tests that HealthCheck handles nil config gracefully.
func TestHealthCheck_NilConfig(t *testing.T) {
	mockPoller := &MockPoller{
		isRunning: true,
		lastPollInfo: &poller.LastPollInfo{
			Timestamp:     time.Now().Add(-10 * time.Second),
			DurationMs:    100,
			RegistersRead: 10,
			ValuesStored:  10,
		},
	}

	service := &ReadService{
		config: nil, // nil config
		poller: mockPoller,
	}

	// HealthCheck should succeed (no stale check without config)
	status, err := service.HealthCheck()

	if err != nil {
		t.Errorf("HealthCheck() error = %v, expected nil with nil config", err)
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}
}

// TestHealthCheck_ZeroInterval tests that HealthCheck handles zero interval gracefully.
func TestHealthCheck_ZeroInterval(t *testing.T) {
	cfg := &config.AppConfig{
		Poller: config.PollerSettings{
			Interval: 0, // zero interval
		},
	}

	mockPoller := &MockPoller{
		isRunning: true,
		lastPollInfo: &poller.LastPollInfo{
			Timestamp:     time.Now().Add(-10 * time.Second),
			DurationMs:    100,
			RegistersRead: 10,
			ValuesStored:  10,
		},
	}

	service := &ReadService{
		config: cfg,
		poller: mockPoller,
	}

	// HealthCheck should succeed (no stale check with zero interval)
	status, err := service.HealthCheck()

	if err != nil {
		t.Errorf("HealthCheck() error = %v, expected nil with zero interval", err)
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}
}

// TestHealthCheck_NilPoller tests that HealthCheck succeeds when poller is nil.
func TestHealthCheck_NilPoller(t *testing.T) {
	cfg := &config.AppConfig{}

	service := &ReadService{
		config: cfg,
		poller: nil, // nil poller
	}

	// HealthCheck should succeed
	status, err := service.HealthCheck()

	if err != nil {
		t.Errorf("HealthCheck() error = %v, expected nil with nil poller", err)
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}

	if status["poller_running"] != "disabled" {
		t.Errorf("HealthCheck() status['poller_running'] = %v, want 'disabled'", status["poller_running"])
	}
}
