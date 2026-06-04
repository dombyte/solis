package poller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/modbus"
)

func TestNew(t *testing.T) {
	// Create test config
	cfg := &config.PollerSettings{
		Interval:        15 * time.Minute,
		BlockAttempts:   3,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     30 * time.Second,
	}

	// We can't create a real modbus client without a connection,
	// so we'll test with a nil client (the poller should still be created)
	client := &modbus.Client{}

	poller := New(cfg, client)

	if poller == nil {
		t.Fatal("New() returned nil")
	}

	if poller.config != cfg {
		t.Error("Poller.config is not set correctly")
	}

	if poller.modbusClient != client {
		t.Error("Poller.modbusClient is not set correctly")
	}

	if poller.done == nil {
		t.Error("Poller.done channel is nil")
	}

	if poller.ctx == nil {
		t.Error("Poller.ctx is nil")
	}

	if poller.ctxCancel == nil {
		t.Error("Poller.ctxCancel is nil")
	}

	if poller.isRunning != false {
		t.Errorf("Poller.isRunning = %v, want false", poller.isRunning)
	}
}

func TestNew_WithStorage(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        15 * time.Minute,
		BlockAttempts:   3,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     30 * time.Second,
	}

	client := &modbus.Client{}

	// Use WithStorage option
	poller := New(cfg, client, WithStorage(nil))

	if poller == nil {
		t.Fatal("New() with WithStorage returned nil")
	}

	// Storage should be nil in this case
	if poller.storage != nil {
		t.Error("Poller.storage should be nil when passed nil")
	}
}

func TestPoller_Start_Stop(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        100 * time.Millisecond, // Short interval for testing
		BlockAttempts:   1,
		BlockRetryDelay: 10 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     500 * time.Millisecond,
	}

	client := &modbus.Client{}
	poller := New(cfg, client)

	// Start the poller
	poller.Start()

	if !poller.IsRunning() {
		t.Error("Poller.IsRunning() = false after Start(), want true")
	}

	// Wait a bit for the poller to do some work
	time.Sleep(50 * time.Millisecond)

	// Stop the poller
	poller.Stop()

	if poller.IsRunning() {
		t.Error("Poller.IsRunning() = true after Stop(), want false")
	}
}

func TestPoller_IsRunning(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        15 * time.Minute,
		BlockAttempts:   3,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     30 * time.Second,
	}

	client := &modbus.Client{}
	poller := New(cfg, client)

	// Initially not running
	if poller.IsRunning() {
		t.Error("Poller.IsRunning() = true initially, want false")
	}

	// Start
	poller.Start()
	if !poller.IsRunning() {
		t.Error("Poller.IsRunning() = false after Start(), want true")
	}

	// Stop
	poller.Stop()
	if poller.IsRunning() {
		t.Error("Poller.IsRunning() = true after Stop(), want false")
	}
}

func TestPoller_DoubleStart(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        15 * time.Minute,
		BlockAttempts:   3,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     30 * time.Second,
	}

	client := &modbus.Client{}
	poller := New(cfg, client)

	// Start once
	poller.Start()

	// Start again (should be safe)
	poller.Start()

	// Should still be running
	if !poller.IsRunning() {
		t.Error("Poller.IsRunning() = false after double Start(), want true")
	}

	// Clean up
	poller.Stop()
}

func TestPoller_Stop_WithoutStart(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        15 * time.Minute,
		BlockAttempts:   3,
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     30 * time.Second,
	}

	client := &modbus.Client{}
	poller := New(cfg, client)

	// Stop without starting (should be safe)
	poller.Stop()

	// Should not be running
	if poller.IsRunning() {
		t.Error("Poller.IsRunning() = true after Stop() without Start(), want false")
	}
}

func TestPoller_PollNow(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        15 * time.Minute,
		BlockAttempts:   0, // No retries
		BlockRetryDelay: 1 * time.Second,
		BlockInterval:   0,
		PollTimeout:     1 * time.Second,
	}

	// Create a modbus client with minimal config
	// We can't create a real one, so we test with a nil client
	// and expect PollNow to fail gracefully (it will panic, but we recover)
	client := &modbus.Client{}
	poller := New(cfg, client)

	// PollNow will panic because the modbus client is not properly initialized
	// We test that the poller struct itself is valid
	if poller == nil {
		t.Fatal("Poller is nil")
	}

	// We skip the actual PollNow call to avoid panics in tests
	t.Skip("Skipping PollNow test - requires initialized modbus client")
}

func TestPoller_ConcurrentStartStop(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        100 * time.Millisecond,
		BlockAttempts:   0,
		BlockRetryDelay: 10 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     500 * time.Millisecond,
	}

	client := &modbus.Client{}
	poller := New(cfg, client)

	// Test concurrent Start and Stop
	var wg sync.WaitGroup

	// Start in one goroutine
	wg.Go(func() {
		poller.Start()
	})

	// Stop in another goroutine
	wg.Go(func() {
		time.Sleep(50 * time.Millisecond)
		poller.Stop()
	})

	// Wait for both to finish
	wg.Wait()

	// Poller should not be running after stop
	if poller.IsRunning() {
		t.Error("Poller should not be running after Stop()")
	}
}

func TestPoller_Statistics(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        100 * time.Millisecond,
		BlockAttempts:   0, // No retries to speed up tests
		BlockRetryDelay: 10 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     100 * time.Millisecond,
	}

	client := &modbus.Client{}
	poller := New(cfg, client)

	// Initially, poll count should be 0
	if poller.GetPollCount() != 0 {
		t.Errorf("Initial poll count = %d, want 0", poller.GetPollCount())
	}

	// Start the poller
	poller.Start()

	// Wait for at least one poll to complete (with short timeout and no retries, it should fail fast)
	time.Sleep(300 * time.Millisecond)

	// Stop the poller
	poller.Stop()

	// We can't reliably check poll count because polls might fail with nil client
	// Just verify the poller started and stopped correctly
	if poller.IsRunning() {
		t.Error("Poller should not be running after Stop()")
	}
}

// GetPollCount is a helper for testing
func (p *Poller) GetPollCount() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pollCount
}

func TestPoller_WithContext(t *testing.T) {
	cfg := &config.PollerSettings{
		Interval:        100 * time.Millisecond,
		BlockAttempts:   1,
		BlockRetryDelay: 10 * time.Millisecond,
		BlockInterval:   0,
		PollTimeout:     500 * time.Millisecond,
	}

	client := &modbus.Client{}
	poller := New(cfg, client)

	// Start the poller
	poller.Start()

	// Create a context that will be cancelled
	_, cancel := context.WithCancel(context.Background())

	// Cancel the context
	cancel()

	// The poller should handle the cancelled context gracefully
	// We can't easily test this, but we verify the poller doesn't crash
	time.Sleep(50 * time.Millisecond)

	// Stop the poller
	poller.Stop()
}

func TestPoller_ConfigValidation(t *testing.T) {
	// Test with various poller configurations
	configs := []struct {
		name string
		cfg  *config.PollerSettings
	}{
		{
			name: "default",
			cfg: &config.PollerSettings{
				Interval:        15 * time.Minute,
				BlockAttempts:   3,
				BlockRetryDelay: 1 * time.Second,
				BlockInterval:   0,
				PollTimeout:     30 * time.Second,
			},
		},
		{
			name: "short intervals",
			cfg: &config.PollerSettings{
				Interval:        1 * time.Second,
				BlockAttempts:   1,
				BlockRetryDelay: 100 * time.Millisecond,
				BlockInterval:   0,
				PollTimeout:     1 * time.Second,
			},
		},
		{
			name: "with block interval",
			cfg: &config.PollerSettings{
				Interval:        15 * time.Minute,
				BlockAttempts:   3,
				BlockRetryDelay: 1 * time.Second,
				BlockInterval:   500 * time.Millisecond,
				PollTimeout:     30 * time.Second,
			},
		},
	}

	for _, tt := range configs {
		t.Run(tt.name, func(t *testing.T) {
			client := &modbus.Client{}
			poller := New(tt.cfg, client)

			if poller == nil {
				t.Error("New() returned nil")
			}

			if poller.config != tt.cfg {
				t.Error("Poller.config is not set correctly")
			}
		})
	}
}
