package aggregator

import (
	"testing"
	"time"

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
)

func TestNewAggregator(t *testing.T) {
	cfg := &config.AggregatorSettings{
		Interval: 60 * time.Second,
	}

	agg := New(nil, nil, cfg)
	if agg == nil {
		t.Fatal("New returned nil")
	}

	if agg.config == nil {
		t.Error("config is nil")
	}

	if agg.config.Interval != 60*time.Second {
		t.Errorf("expected interval 60s, got %v", agg.config.Interval)
	}

	if agg.isRunning {
		t.Error("aggregator should not be running initially")
	}
}

func TestAggregator_StartStop(t *testing.T) {
	cfg := &config.AggregatorSettings{
		Interval: 100 * time.Millisecond, // Short interval for testing
	}

	ca := cache.New()
	agg := New(nil, ca, cfg)

	// Start the aggregator
	agg.Start()

	if !agg.IsRunning() {
		t.Error("aggregator should be running after Start()")
	}

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop the aggregator
	agg.Stop()

	if agg.IsRunning() {
		t.Error("aggregator should not be running after Stop()")
	}
}

func TestAggregator_DoubleStart(t *testing.T) {
	cfg := &config.AggregatorSettings{
		Interval: 100 * time.Millisecond,
	}

	agg := New(nil, nil, cfg)

	// Start once
	agg.Start()

	// Try to start again - should not panic or create duplicate goroutines
	agg.Start()

	if !agg.IsRunning() {
		t.Error("aggregator should still be running")
	}

	agg.Stop()
}

func TestAggregator_DoubleStop(t *testing.T) {
	cfg := &config.AggregatorSettings{
		Interval: 100 * time.Millisecond,
	}

	agg := New(nil, nil, cfg)

	// Stop without starting - should not panic
	agg.Stop()

	// Start and stop
	agg.Start()
	time.Sleep(50 * time.Millisecond)
	agg.Stop()

	// Try to stop again - should not panic
	agg.Stop()
}
