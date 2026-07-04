// Package poller provides a background polling service for reading Solis inverter
// registers at regular intervals and storing the results.
package poller

import (
	"context"
	"fmt"
	"math/rand"
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

func init() {
	// Seed random number generator for jitter
	rand.Seed(time.Now().UnixNano())
}

// LastPollInfo contains information about the last completed poll cycle.
type LastPollInfo struct {
	Timestamp     time.Time
	DurationMs    int64
	RegistersRead int
	ValuesStored  int
}

// Poller is the background service that polls the Solis inverter for register data.
type Poller struct {
	// config holds the poller configuration.
	config *config.PollerSettings
	// modbusClient is the Modbus client used for polling.
	modbusClient *modbus.Client
	// storage is the storage backend for persisting data.
	storage *storage.Storage
	// cache holds the latest register values for fast access.
	cache *cache.Cache
	// registerFilter holds the filter for disabled registers.
	registerFilter *solis.RegisterFilter
	// done is the channel used to signal the poller to stop.
	done chan struct{}
	// ctx is the context for the poller's lifetime.
	ctx context.Context
	// ctxCancel cancels the poller's context.
	ctxCancel context.CancelFunc
	// wg is used to wait for the polling goroutine to finish.
	wg sync.WaitGroup
	// isRunning tracks if the poller is currently running.
	isRunning bool
	// mu protects the running state.
	mu sync.Mutex
	// lastPollTime tracks when the last poll completed.
	lastPollTime time.Time
	// pollCount tracks the number of completed polls.
	pollCount int64
	// lastPollInfo stores the most recent poll metadata (replaces last_poll DB table).
	lastPollInfo *LastPollInfo
}

// PollerOption is a function that configures a Poller.
type PollerOption func(*Poller)

// WithStorage sets the storage backend for the poller.
func WithStorage(st *storage.Storage) PollerOption {
	return func(p *Poller) {
		p.storage = st
	}
}

// WithCache sets the cache for the poller.
func WithCache(ca *cache.Cache) PollerOption {
	return func(p *Poller) {
		p.cache = ca
	}
}

// WithRegisterFilter sets the register filter for the poller.
func WithRegisterFilter(rf *solis.RegisterFilter) PollerOption {
	return func(p *Poller) {
		p.registerFilter = rf
	}
}

// New creates a new Poller instance.
func New(cfg *config.PollerSettings, modbusClient *modbus.Client, opts ...PollerOption) *Poller {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Poller{
		config:       cfg,
		modbusClient: modbusClient,
		done:         make(chan struct{}),
		ctx:          ctx,
		ctxCancel:    cancel,
		isRunning:    false,
		lastPollTime: time.Time{},
		pollCount:    0,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Start starts the poller's background goroutine.
func (p *Poller) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isRunning {
		logger.Warn().Msg("Poller is already running")
		return
	}

	p.isRunning = true
	p.wg.Add(1)

	go p.run()
	logger.Info().Msg("Poller started")
}

// Stop stops the poller's background goroutine.
func (p *Poller) Stop() {
	p.mu.Lock()
	if !p.isRunning {
		p.mu.Unlock()
		logger.Warn().Msg("Poller is not running")
		return
	}
	p.isRunning = false
	p.mu.Unlock()

	// Cancel the context to interrupt ongoing operations
	p.ctxCancel()

	close(p.done)
	p.wg.Wait()

	logger.Info().Msg("Poller stopped")
}

// run is the main polling loop.
func (p *Poller) run() {
	defer p.wg.Done()

	// Initial delay before first poll
	time.Sleep(1 * time.Second)

	for {
		select {
		case <-p.done:
			logger.Info().Msg("Poller received stop signal")
			return
		default:
			// Check if we should poll
			p.mu.Lock()
			isRunning := p.isRunning
			p.mu.Unlock()

			if !isRunning {
				return
			}

			// Recover from panics to keep poller running
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error().Msgf("Poller recovered from panic: %v", r)
					}
				}()

				// Calculate time until next poll
				now := time.Now()
				if !p.lastPollTime.IsZero() {
					// Non-overlapping: next poll starts at max(interval, previous_poll_duration)
					elapsed := now.Sub(p.lastPollTime)
					if elapsed < p.config.Interval {
						sleepTime := p.config.Interval - elapsed
						logger.Debug().Msgf("Sleeping for %s until next poll", sleepTime)
						select {
						case <-p.ctx.Done():
							return
						case <-time.After(sleepTime):
						}
						return
					}
				}

				// For RTU, add random jitter to avoid collision with other devices on the bus
				if p.modbusClient != nil && p.modbusClient.Config().Type == "rtu" && p.config.JitterMax > 0 {
					jitter := time.Duration(rand.Int63n(int64(p.config.JitterMax))) // #nosec G404
					logger.Debug().Msgf("Adding %s jitter before poll to avoid RTU bus collision", jitter)
					select {
					case <-p.ctx.Done():
						return
					case <-time.After(jitter):
					}
				}

				// Perform poll
				pollStart := time.Now()
				values, registersRead, err := p.pollOnce(pollStart)
				pollDuration := time.Since(pollStart)

				if err != nil {
					logger.Error().Msgf("Poll failed: %v", err)
					// Still update last poll time to prevent rapid retries
					p.mu.Lock()
					p.lastPollTime = time.Now()
					p.mu.Unlock()
					return
				}

				// Store values
				if p.storage != nil {
					if err := p.storage.StoreAllRegisters(values, pollStart); err != nil {
						logger.Error().Msgf("Failed to store values: %v", err)
					} else {
						// Cleanup old daily data
						if err := p.storage.CleanupDailyData(); err != nil {
							logger.Error().Msgf("Failed to cleanup daily data: %v", err)
						}

						// Cleanup old error data
						if err := p.storage.CleanupErrorData(); err != nil {
							logger.Error().Msgf("Failed to cleanup error data: %v", err)
						}

						// Store poll info in memory (replaces last_poll DB table)
						p.mu.Lock()
						p.lastPollInfo = &LastPollInfo{
							Timestamp:     time.Now(),
							DurationMs:    pollDuration.Milliseconds(),
							RegistersRead: registersRead,
							ValuesStored:  len(values),
						}
						p.mu.Unlock()
					}
				}

				// Update cache with latest values (non-blocking for reads) - always update cache
				if p.cache != nil {
					p.cache.Set(values)
				}

				// Update poll statistics
				p.mu.Lock()
				p.lastPollTime = time.Now()
				p.pollCount++
				count := p.pollCount
				p.mu.Unlock()

				logger.Info().Msgf("Poll %d completed in %s: read %d registers, stored %d values",
					count, pollDuration, registersRead, len(values))
				// For RTU, close connection after poll to allow other devices to use the serial port
				if p.modbusClient != nil && p.modbusClient.Config().Type == "rtu" && p.modbusClient.IsConnected() {
					if err := p.modbusClient.Disconnect(); err != nil {
						logger.Debug().Msgf("RTU disconnect after poll: %v", err)
					} else {
						logger.Debug().Msg("RTU connection closed after poll")
					}
				}
			}()
		}
	}
}

// pollOnce performs a single poll cycle: reads all 4 ranges sequentially.
// Returns the decoded values, total registers read, and any error.
func (p *Poller) pollOnce(startTime time.Time) (map[string]*solis.Value, int, error) {
	logger.Debug().Msgf("Starting poll cycle at %s", startTime)

	// For RTU, ensure connection is open before polling
	if p.modbusClient != nil && p.modbusClient.Config().Type == "rtu" && !p.modbusClient.IsConnected() {
		if err := p.modbusClient.Connect(context.Background()); err != nil {
			return nil, 0, fmt.Errorf("failed to connect before poll: %w", err)
		}
		logger.Debug().Msg("RTU reconnected for poll")
	}

	// Track total registers read
	totalRegisters := 0

	// Result map for all decoded values (pre-allocated for ~109 registers)
	values := make(map[string]*solis.Value, 120)

	// Create a context with timeout for the full poll, derived from poller's context
	ctx, cancel := context.WithTimeout(p.ctx, p.config.PollTimeout)
	defer cancel()

	// Poll each range sequentially
	for i, rangeDef := range solis.ReadRanges {
		logger.Debug().Msgf("Reading range %d: address=%d, count=%d",
			i+1, rangeDef.StartAddr, rangeDef.Count)

		// Retry logic for this block
		var rawBytes []byte
		var err error

		for attempt := 0; attempt <= p.config.BlockAttempts; attempt++ {
			if attempt > 0 {
				// Wait before retry
				logger.Warn().Msgf("Range %d read attempt %d/%d failed, retrying...",
					i+1, attempt+1, p.config.BlockAttempts+1)
				time.Sleep(p.config.BlockRetryDelay)
			}

			rawBytes, err = p.modbusClient.ReadRegisters(ctx, rangeDef.StartAddr, rangeDef.Count)
			if err == nil {
				break
			}

			logger.Warn().Msgf("Range %d read failed (attempt %d/%d): %v",
				i+1, attempt+1, p.config.BlockAttempts+1, err)

			if attempt >= p.config.BlockAttempts {
				// All attempts exhausted
				logger.Error().Msgf("Range %d failed after %d attempts: %v",
					i+1, p.config.BlockAttempts+1, err)
				// Return partial results if we have any
				if len(values) > 0 {
					return values, totalRegisters, fmt.Errorf("partial poll: range %d failed: %w", i+1, err)
				}
				return nil, totalRegisters, fmt.Errorf("poll failed at range %d: %w", i+1, err)
			}
		}

		if rawBytes == nil {
			logger.Error().Msgf("Range %d returned nil data", i+1)
			continue
		}

		// Decode this range
		rangeValues := solis.DecodeRange(rangeDef.StartAddr, rawBytes)
		for key, value := range rangeValues {
			// Create a new value with timestamp set
			val := value
			val.Timestamp = startTime
			values[key] = &val
		}

		totalRegisters += int(rangeDef.Count)
		logger.Debug().Msgf("Decoded %d values from range %d", len(rangeValues), i+1)

		// Wait between blocks if configured
		if p.config.BlockInterval > 0 && i < len(solis.ReadRanges)-1 {
			time.Sleep(p.config.BlockInterval)
		}
		// For RTU, add delay between ranges to prevent serial port overload
		if p.modbusClient != nil && p.modbusClient.Config().Type == "rtu" && i < len(solis.ReadRanges)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Apply battery current direction to battery_current and battery_power
	// battery_current_direction: 1 = discharging (negative), 0 = charging (positive)
	if dirVal, dirExists := values["battery_current_direction"]; dirExists {
		// Determine sign based on direction
		// 1 = discharging (negative), 0 = charging (positive)
		sign := -1.0
		if dirVal.RawValue == 0 {
			sign = 1.0
		}

		// Apply sign to battery_current
		if bcVal, bcExists := values["battery_current"]; bcExists {
			bcVal.RawValue *= sign
			bcVal.DecodedValue *= sign
			logger.Debug().Msgf("Applied direction sign to battery_current: %.1f A", bcVal.DecodedValue)
		}

		// Apply sign to battery_power
		if bpVal, bpExists := values["battery_power"]; bpExists {
			bpVal.RawValue *= sign
			bpVal.DecodedValue *= sign
			logger.Debug().Msgf("Applied direction sign to battery_power: %.1f W", bpVal.DecodedValue)
		}

		// Remove battery_current_direction from values so it doesn't get saved
		delete(values, "battery_current_direction")
		logger.Debug().Msg("Removed battery_current_direction from values")
	}

	// Filter out disabled registers from the results
	if p.registerFilter != nil {
		for key := range values {
			if !p.registerFilter.IsEnabled(key) {
				delete(values, key)
				logger.Debug().Msgf("Filtered out disabled register: %s", key)
			}
		}
	}

	// Compute net grid energy values
	// total_grid_energy = total_energy_fed_into_grid - total_energy_imported_from_grid
	if fedTotal, fedExists := values["total_energy_fed_into_grid"]; fedExists {
		if importTotal, importExists := values["total_energy_imported_from_grid"]; importExists {
			netTotalValue := *fedTotal
			netTotalValue.RawValue = fedTotal.RawValue - importTotal.RawValue
			netTotalValue.DecodedValue = fedTotal.DecodedValue - importTotal.DecodedValue
			netTotalValue.Key = "total_grid_energy"
			netTotalValue.Name = "Total Grid Energy (Net)"
			netTotalValue.Unit = "kWh"
			values["total_grid_energy"] = &netTotalValue
			logger.Debug().Msgf("Computed total_grid_energy: %.1f kWh", netTotalValue.DecodedValue)
		}
	}

	// today_grid_energy = today_energy_fed_into_grid - today_energy_imported_from_grid
	if fedToday, fedExists := values["today_energy_fed_into_grid"]; fedExists {
		if importToday, importExists := values["today_energy_imported_from_grid"]; importExists {
			netTodayValue := *fedToday
			netTodayValue.RawValue = fedToday.RawValue - importToday.RawValue
			netTodayValue.DecodedValue = fedToday.DecodedValue - importToday.DecodedValue
			netTodayValue.Key = "today_grid_energy"
			netTodayValue.Name = "Today Grid Energy (Net)"
			netTodayValue.Unit = "kWh"
			values["today_grid_energy"] = &netTodayValue
			logger.Debug().Msgf("Computed today_grid_energy: %.1f kWh", netTodayValue.DecodedValue)
		}
	}

	// Compute monthly and yearly energy registers from daily storage
	// Only compute if storage is available
	if p.storage != nil {
		// Get current date for month/year calculation
		currentMonth := startTime.Format("2006-01")
		currentYear := startTime.Format("2006")

		// Define mappings from daily to monthly/yearly register keys
		dailyToMonthly := map[string]string{
			"today_energy_consumption":        "energy_consumption_month_energy",
			"today_energy_fed_into_grid":      "energy_fed_into_grid_month_energy",
			"today_energy_imported_from_grid": "energy_imported_from_grid_month_energy",
			"today_battery_discharge_energy":  "battery_discharge_month_energy",
			"today_battery_charge_energy":     "battery_charge_month_energy",
		}

		dailyToYearly := map[string]string{
			"today_energy_consumption":        "energy_consumption_year_energy",
			"today_energy_fed_into_grid":      "energy_fed_into_grid_year_energy",
			"today_energy_imported_from_grid": "energy_imported_from_grid_year_energy",
			"today_battery_discharge_energy":  "battery_discharge_year_energy",
			"today_battery_charge_energy":     "battery_charge_year_energy",
		}

		// Compute monthly values from daily storage
		for dailyKey, monthlyKey := range dailyToMonthly {
			value, _, err := p.storage.GetMonthlySum(dailyKey, currentMonth)
			if err != nil {
				logger.Warn().Msgf("Failed to compute %s from daily storage: %v", monthlyKey, err)
				continue
			}

			// Look up the register definition
			reg, ok := solis.RegisterMapByKey[monthlyKey]
			if !ok {
				logger.Warn().Msgf("Register %s not found in RegisterMapByKey", monthlyKey)
				continue
			}

			// For computed registers, we want RawValue * Scale = value (the already-scaled sum)
			// Since reg.Scale is 1 for these computed registers, RawValue should equal value
			// This ensures that when stored, decodedValue = RawValue * Scale = value * 1 = value
			computedValue := &solis.Value{
				Key:          monthlyKey,
				Name:         reg.Name,
				RawValue:     value, // Store the already-scaled value as RawValue
				DecodedValue: value,
				Unit:         reg.Unit,
				Timestamp:    startTime,
			}
			values[monthlyKey] = computedValue
			logger.Debug().Msgf("Computed %s: %.1f kWh", monthlyKey, value)
		}

		// Compute yearly values from daily storage
		for dailyKey, yearlyKey := range dailyToYearly {
			value, _, err := p.storage.GetYearlySum(dailyKey, currentYear)
			if err != nil {
				logger.Warn().Msgf("Failed to compute %s from daily storage: %v", yearlyKey, err)
				continue
			}

			// Look up the register definition
			reg, ok := solis.RegisterMapByKey[yearlyKey]
			if !ok {
				logger.Warn().Msgf("Register %s not found in RegisterMapByKey", yearlyKey)
				continue
			}

			// For computed registers, we want RawValue * Scale = value (the already-scaled sum)
			// Since reg.Scale is 1 for these computed registers, RawValue should equal value
			// This ensures that when stored, decodedValue = RawValue * Scale = value * 1 = value
			computedValue := &solis.Value{
				Key:          yearlyKey,
				Name:         reg.Name,
				RawValue:     value, // Store the already-scaled value as RawValue
				DecodedValue: value,
				Unit:         reg.Unit,
				Timestamp:    startTime,
			}
			values[yearlyKey] = computedValue
			logger.Debug().Msgf("Computed %s: %.1f kWh", yearlyKey, value)
		}

		// Compute net grid energy for monthly: month_grid_energy = energy_fed_into_grid_month_energy - energy_imported_from_grid_month_energy
		if fedMonth, fedExists := values["energy_fed_into_grid_month_energy"]; fedExists {
			if importMonth, importExists := values["energy_imported_from_grid_month_energy"]; importExists {
				// Both values are already scaled (in kWh), so we can subtract directly
				// For the net register with Scale=1, RawValue should equal DecodedValue
				netValue := fedMonth.DecodedValue - importMonth.DecodedValue
				reg, ok := solis.RegisterMapByKey["month_grid_energy"]
				if !ok {
					logger.Warn().Msg("Register month_grid_energy not found in RegisterMapByKey")
				} else {
					netMonthValue := &solis.Value{
						Key:          "month_grid_energy",
						Name:         reg.Name,
						RawValue:     netValue, // Store already-scaled value as RawValue
						DecodedValue: netValue,
						Unit:         reg.Unit,
						Timestamp:    startTime,
					}
					values["month_grid_energy"] = netMonthValue
					logger.Debug().Msgf("Computed month_grid_energy: %.1f kWh", netMonthValue.DecodedValue)
				}
			}
		}

		// Compute net grid energy for yearly: year_grid_energy = energy_fed_into_grid_year_energy - energy_imported_from_grid_year_energy
		if fedYear, fedExists := values["energy_fed_into_grid_year_energy"]; fedExists {
			if importYear, importExists := values["energy_imported_from_grid_year_energy"]; importExists {
				// Both values are already scaled (in kWh), so we can subtract directly
				// For the net register with Scale=1, RawValue should equal DecodedValue
				netValue := fedYear.DecodedValue - importYear.DecodedValue
				reg, ok := solis.RegisterMapByKey["year_grid_energy"]
				if !ok {
					logger.Warn().Msg("Register year_grid_energy not found in RegisterMapByKey")
				} else {
					netYearValue := &solis.Value{
						Key:          "year_grid_energy",
						Name:         reg.Name,
						RawValue:     netValue, // Store already-scaled value as RawValue
						DecodedValue: netValue,
						Unit:         reg.Unit,
						Timestamp:    startTime,
					}
					values["year_grid_energy"] = netYearValue
					logger.Debug().Msgf("Computed year_grid_energy: %.1f kWh", netYearValue.DecodedValue)
				}
			}
		}
	}

	logger.Debug().Msgf("Poll cycle completed: read %d total registers, decoded %d values",
		totalRegisters, len(values))

	return values, totalRegisters, nil
}

// PollNow triggers an immediate poll and returns the results.
// This can be called from HTTP handlers for direct reads.
func (p *Poller) PollNow() (map[string]*solis.Value, error) {
	logger.Info().Msg("Triggering immediate poll")

	startTime := time.Now()
	values, _, err := p.pollOnce(startTime)
	if err != nil {
		return nil, fmt.Errorf("poll failed: %w", err)
	}

	// Update cache with latest values
	if p.cache != nil {
		p.cache.Set(values)
	}

	logger.Info().Msgf("Immediate poll completed in %s: %d values",
		time.Since(startTime), len(values))

	return values, nil
}

// IsRunning returns whether the poller is currently running.
func (p *Poller) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isRunning
}

// GetLastPollInfo returns information about the most recent completed poll.
// Returns nil if no poll has completed yet.
func (p *Poller) GetLastPollInfo() *LastPollInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastPollInfo == nil {
		return nil
	}
	return p.lastPollInfo
}
