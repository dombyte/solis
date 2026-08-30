// Package poller provides a background service for reading Solis inverter registers
// at regular intervals using a single Modbus TCP connection for sequential reads.
package poller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

// logger is the package-level logger for poller operations.
var logger = logging.NewComponentLogger("poller")

// LastPollInfo contains information about the last completed poll cycle.
type LastPollInfo struct {
	Timestamp     time.Time
	DurationMs    int64
	RegistersRead int
	ValuesStored  int
}

// Poller is the background service that polls the Solis inverter for register data.
// It uses a single Modbus connection for sequential range reads.
type Poller struct {
	config  *config.PollerSettings
	modbus  *modbus.Client
	storage *storage.Storage
	cache   *cache.Cache

	// Lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex

	// Stats
	pollCount    int64
	lastPollTime time.Time
	lastPollInfo *LastPollInfo
	lastPollErr  error
}

// New creates a new Poller instance.
func New(
	cfg *config.PollerSettings,
	modbusClient *modbus.Client,
	opts ...Option,
) *Poller {
	p := &Poller{
		config:  cfg,
		modbus:  modbusClient,
		running: false,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Option is a function that configures a Poller.
type Option func(*Poller)

// WithStorage sets the storage backend for the poller.
func WithStorage(st *storage.Storage) Option {
	return func(p *Poller) {
		p.storage = st
	}
}

// WithCache sets the cache for the poller.
func WithCache(ca *cache.Cache) Option {
	return func(p *Poller) {
		p.cache = ca
	}
}

// Start starts the poller's background goroutine.
func (p *Poller) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("poller is already running")
	}

	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.running = true
	p.wg.Add(1)

	go p.run()

	logger.Info().Msgf("Poller started (interval=%s, poll_timeout=%s)",
		p.config.Interval, p.config.PollTimeout)

	return nil
}

// Stop stops the poller's background goroutine.
func (p *Poller) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return fmt.Errorf("poller is not running")
	}
	p.running = false
	p.mu.Unlock()

	p.cancel()
	p.wg.Wait()

	logger.Info().Msg("Poller stopped")
	return nil
}

// run is the main polling loop.
// Implements non-overlapping poll intervals: if a poll takes 300ms and interval is 5s,
// the next poll starts at 5s from the previous poll start, not immediately after completion.
func (p *Poller) run() {
	defer p.wg.Done()

	// Initial delay to let everything initialize
	time.Sleep(1 * time.Second)

	for {
		select {
		case <-p.ctx.Done():
			logger.Info().Msg("Poll loop stopped: context cancelled")
			return
		default:
			if !p.shouldContinue() {
				return
			}

			// Calculate time until next poll for non-overlapping intervals
			now := time.Now()
			p.mu.Lock()
			lastPollTime := p.lastPollTime
			p.mu.Unlock()

			if !lastPollTime.IsZero() {
				elapsed := now.Sub(lastPollTime)
				if elapsed < p.config.Interval {
					sleepTime := p.config.Interval - elapsed
					logger.Debug().Msgf("Sleeping for %s until next poll to maintain interval", sleepTime)
					select {
					case <-p.ctx.Done():
						return
					case <-time.After(sleepTime):
					}
					continue // Skip this iteration, check again
				}
			}

			p.executePollCycle()
		}
	}
}

// shouldContinue checks if the poller should continue running.
func (p *Poller) shouldContinue() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// executePollCycle executes a single poll cycle with panic recovery.
func (p *Poller) executePollCycle() {
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error().Msgf("Poller recovered from panic: %v", r)
			}
		}()

		p.performPollAndStore()
	}()
}

// performPollAndStore performs a poll and stores the results.
func (p *Poller) performPollAndStore() {
	pollStart := time.Now()

	// Use a context with timeout for the entire poll
	pollCtx, pollCancel := context.WithTimeout(p.ctx, p.config.PollTimeout)
	defer pollCancel()

	values, registersRead, err := p.pollOnce(pollCtx, pollStart)
	pollDuration := time.Since(pollStart)

	if err != nil {
		p.handlePollError(err, pollDuration)
		return
	}

	// Filter out computed registers
	rawValues := p.filterComputedRegisters(values)

	// Store results
	p.storePollResults(rawValues, pollStart, pollDuration, registersRead)

	// Update cache
	p.updateCache(rawValues)

	// Update stats
	p.updatePollStats(pollDuration, registersRead, len(values))
}

// handlePollError handles errors from poll operations.
func (p *Poller) handlePollError(err error, pollDuration time.Duration) {
	logger.Error().Msgf("Poll failed: %v", err)

	p.mu.Lock()
	p.lastPollErr = err
	p.lastPollTime = time.Now()
	p.mu.Unlock()

	// Don't add extra sleep - the next poll will happen at the scheduled interval
	// The modbus layer handles reconnection automatically
}

// filterComputedRegisters filters out computed registers from the results.
func (p *Poller) filterComputedRegisters(values map[string]*solis.Value) map[string]*solis.Value {
	rawValues := make(map[string]*solis.Value, len(values))
	for key, value := range values {
		if !solis.IsComputedRegister(key) {
			rawValues[key] = value
		}
	}
	return rawValues
}

// storePollResults stores poll results and updates poll info.
func (p *Poller) storePollResults(rawValues map[string]*solis.Value, pollStart time.Time, pollDuration time.Duration, registersRead int) {
	if p.storage != nil {
		if err := p.storage.StoreAllRegisters(rawValues, pollStart); err != nil {
			logger.Error().Msgf("Failed to store values: %v", err)
			return
		}

		p.mu.Lock()
		p.lastPollInfo = &LastPollInfo{
			Timestamp:     time.Now(),
			DurationMs:    pollDuration.Milliseconds(),
			RegistersRead: registersRead,
			ValuesStored:  len(rawValues),
		}
		p.mu.Unlock()
	}
}

// updateCache updates the cache with the latest values.
func (p *Poller) updateCache(rawValues map[string]*solis.Value) {
	if p.cache != nil {
		p.cache.Merge(rawValues)
	}
}

// updatePollStats updates poll statistics.
func (p *Poller) updatePollStats(pollDuration time.Duration, registersRead, valuesCount int) {
	p.mu.Lock()
	p.lastPollTime = time.Now()
	p.pollCount++
	count := p.pollCount
	p.mu.Unlock()

	logger.Info().Msgf("Poll %d completed in %s: read %d registers, stored %d values",
		count, pollDuration, registersRead, valuesCount)
}

// pollOnce performs a single poll cycle: reads all ranges sequentially.
// Uses a single Modbus connection for all reads.
func (p *Poller) pollOnce(ctx context.Context, startTime time.Time) (map[string]*solis.Value, int, error) {
	logger.Debug().Msgf("Starting poll cycle at %s", startTime)

	if err := p.validateModbusConnection(); err != nil {
		return nil, 0, err
	}

	values := make(map[string]*solis.Value, 120)
	totalRegisters := 0

	// Read all ranges sequentially using the same connection
	for i, rangeDef := range solis.ReadRanges {
		if err := p.checkContextCancelled(ctx); err != nil {
			return values, totalRegisters, err
		}

		if !p.modbus.IsConnected() {
			return p.handleDisconnectionDuringPoll(i+1, values, totalRegisters)
		}

		logger.Debug().Msgf("Reading range %d: address=%d, count=%d",
			i+1, rangeDef.StartAddr, rangeDef.Count)

		rawRegisters, err := p.readRangeWithRetry(ctx, rangeDef.StartAddr, rangeDef.Count)
		if err != nil {
			return p.handleRangeReadError(i+1, err, values, totalRegisters)
		}

		if rawRegisters == nil {
			logger.Error().Msgf("Range %d returned nil data", i+1)
			continue
		}

		p.decodeRangeAndUpdateCount(rangeDef.StartAddr, rawRegisters, startTime, values)
		totalRegisters += int(rangeDef.Count)

		if p.config.BlockInterval > 0 && i < len(solis.ReadRanges)-1 {
			if err := p.waitForBlockInterval(ctx); err != nil {
				return values, totalRegisters, err
			}
		}
	}

	logger.Debug().Msgf("Poll cycle completed: read %d total registers, decoded %d values",
		totalRegisters, len(values))

	return values, totalRegisters, nil
}

// validateModbusConnection checks if the modbus client is ready for polling.
func (p *Poller) validateModbusConnection() error {
	if p.modbus == nil {
		return fmt.Errorf("modbus client is nil")
	}
	if !p.modbus.IsConnected() {
		logger.Warn().Msg("Skipping poll: modbus client is not connected")
		return fmt.Errorf("modbus not connected")
	}
	return nil
}

// checkContextCancelled checks if the context has been cancelled.
func (p *Poller) checkContextCancelled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// handleDisconnectionDuringPoll handles disconnection detected during poll.
func (p *Poller) handleDisconnectionDuringPoll(rangeIndex int, values map[string]*solis.Value, totalRegisters int) (map[string]*solis.Value, int, error) {
	logger.Warn().Msgf("Skipping range %d: modbus disconnected during poll", rangeIndex)
	return values, totalRegisters, fmt.Errorf("modbus disconnected during poll")
}

// handleRangeReadError handles errors from range read operations.
func (p *Poller) handleRangeReadError(rangeIndex int, err error, values map[string]*solis.Value, totalRegisters int) (map[string]*solis.Value, int, error) {
	if len(values) > 0 {
		return values, totalRegisters, fmt.Errorf("partial poll: range %d failed: %w", rangeIndex, err)
	}
	return nil, totalRegisters, fmt.Errorf("poll failed at range %d: %w", rangeIndex, err)
}

// decodeRangeAndUpdateCount decodes a range and updates the values map.
func (p *Poller) decodeRangeAndUpdateCount(startAddr uint16, rawRegisters []uint16, startTime time.Time, values map[string]*solis.Value) {
	p.decodeRange(startAddr, rawRegisters, startTime, values)
}

// waitForBlockInterval waits for the configured interval between block reads.
func (p *Poller) waitForBlockInterval(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(p.config.BlockInterval):
		return nil
	}
}

// readRangeWithRetry reads a single range with retry logic.
// It relies on the background reconnection loop to restore connectivity.
func (p *Poller) readRangeWithRetry(ctx context.Context, startAddr, count uint16) ([]uint16, error) {
	if p.modbus == nil {
		return nil, fmt.Errorf("modbus client is nil")
	}

	var rawRegisters []uint16
	var err error

	for attempt := 0; attempt <= p.config.BlockAttempts; attempt++ {
		if attempt > 0 {
			if err := p.checkContextCancelled(ctx); err != nil {
				return nil, err
			}

			p.handleRetryAttempt(ctx, attempt, startAddr, count)

			if err := p.waitForRetryDelay(ctx); err != nil {
				return nil, err
			}
		}

		rawRegisters, err = p.modbus.ReadRegisters(ctx, startAddr, count)
		if err == nil {
			return rawRegisters, nil
		}

		logger.Warn().Msgf("Range read failed (attempt %d/%d): %v",
			attempt+1, p.config.BlockAttempts+1, err)
	}

	return p.handleFinalReadFailure(err)
}

// handleRetryAttempt handles logic for retry attempts after initial failure.
func (p *Poller) handleRetryAttempt(ctx context.Context, attempt int, startAddr, count uint16) {
	if !p.modbus.IsConnected() {
		p.waitForReconnection(ctx)
	} else {
		logger.Warn().Msgf("Range read attempt %d/%d failed, retrying...",
			attempt+1, p.config.BlockAttempts+1)
	}
}

// waitForReconnection waits for the modbus connection to be restored.
func (p *Poller) waitForReconnection(ctx context.Context) {
	logger.Warn().Msg("Modbus disconnected, waiting for background reconnection...")

	waitCtx, waitCancel := context.WithTimeout(ctx, p.config.PollTimeout/2)
	if waitErr := p.modbus.WaitForConnection(waitCtx); waitErr != nil {
		waitCancel()
		logger.Warn().Msgf("Wait for connection failed: %v", waitErr)
		if err := p.waitForRetryDelay(ctx); err != nil {
			return
		}
	} else {
		waitCancel()
		logger.Info().Msg("Modbus reconnected, retrying read")
	}
}

// waitForRetryDelay waits for the configured retry delay before next attempt.
func (p *Poller) waitForRetryDelay(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(p.config.BlockRetryDelay):
		return nil
	}
}

// handleFinalReadFailure handles the final failure after all retry attempts.
func (p *Poller) handleFinalReadFailure(err error) ([]uint16, error) {
	logger.Error().Msgf("Range failed after %d attempts: %v",
		p.config.BlockAttempts+1, err)
	return nil, err
}

// decodeRange decodes a range of raw registers into values.
func (p *Poller) decodeRange(startAddr uint16, rawRegisters []uint16, startTime time.Time, values map[string]*solis.Value) {
	rangeValues := solis.DecodeRange(startAddr, rawRegisters)
	for key, value := range rangeValues {
		value.Timestamp = startTime
		values[key] = &value
	}
	logger.Debug().Msgf("Decoded %d values from range", len(rangeValues))
}

// PollNow triggers an immediate poll and returns the results.
// This can be called from HTTP handlers for direct reads.
func (p *Poller) PollNow() (map[string]*solis.Value, error) {
	logger.Info().Msg("Triggering immediate poll")

	startTime := time.Now()

	// Use a generous timeout for immediate polls
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	values, _, err := p.pollOnce(ctx, startTime)
	if err != nil {
		return nil, fmt.Errorf("poll failed: %w", err)
	}

	// Filter out computed registers for immediate poll as well
	rawValues := make(map[string]*solis.Value, len(values))
	for key, value := range values {
		if !solis.IsComputedRegister(key) {
			rawValues[key] = value
		}
	}

	// Update cache with latest values
	if p.cache != nil {
		p.cache.Merge(rawValues)
	}

	logger.Info().Msgf("Immediate poll completed in %s: %d values",
		time.Since(startTime), len(rawValues))

	return values, nil
}

// IsRunning returns whether the poller is currently running.
func (p *Poller) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// GetLastPollInfo returns information about the most recent completed poll.
func (p *Poller) GetLastPollInfo() *LastPollInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastPollInfo == nil {
		return nil
	}
	return p.lastPollInfo
}

// GetLastPollError returns the error from the last poll, if any.
func (p *Poller) GetLastPollError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastPollErr
}
