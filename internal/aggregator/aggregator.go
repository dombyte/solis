// Package aggregator provides background computation of derived register values
// (monthly, yearly, net values) separate from the polling cycle.
package aggregator

import (
	"fmt"
	"sync"
	"time"

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

// logger is the package-level logger for aggregator operations.
var logger = logging.NewComponentLogger("aggregator")

// Aggregator is the background service that computes derived register values
// (monthly, yearly, net values) from stored daily data.
type Aggregator struct {
	// storage is the storage backend for reading daily values.
	storage *storage.Storage
	// cache is the cache for storing computed values.
	cache *cache.Cache
	// config holds the aggregator configuration.
	config *config.AggregatorSettings
	// stopChan is used to signal the aggregator to stop.
	stopChan chan struct{}
	// wg is used to wait for the background goroutine to finish.
	wg sync.WaitGroup
	// isRunning tracks if the aggregator is currently running.
	isRunning bool
	// mu protects the running state.
	mu sync.Mutex
}

// New creates a new Aggregator instance.
func New(storage *storage.Storage, cache *cache.Cache, cfg *config.AggregatorSettings) *Aggregator {
	return &Aggregator{
		storage:   storage,
		cache:     cache,
		config:    cfg,
		stopChan:  make(chan struct{}),
		isRunning: false,
	}
}

// Start starts the aggregator's background goroutine.
func (a *Aggregator) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.isRunning {
		logger.Warn().Msg("Aggregator is already running")
		return
	}

	a.isRunning = true
	a.wg.Add(1)

	go a.run()

	logger.Info().Msgf("Aggregator started with interval: %s", a.config.Interval)
}

// Stop stops the aggregator's background goroutine.
func (a *Aggregator) Stop() {
	a.mu.Lock()
	if !a.isRunning {
		a.mu.Unlock()
		logger.Warn().Msg("Aggregator is not running")
		return
	}
	a.isRunning = false
	a.mu.Unlock()

	close(a.stopChan)
	a.wg.Wait()

	logger.Info().Msg("Aggregator stopped")
}

// IsRunning returns whether the aggregator is currently running.
func (a *Aggregator) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isRunning
}

// run is the main aggregation loop.
func (a *Aggregator) run() {
	defer a.wg.Done()

	// Initial computation on startup
	a.computeAll()

	ticker := time.NewTicker(a.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopChan:
			logger.Info().Msg("Aggregator received stop signal")
			return
		case <-ticker.C:
			a.computeAll()
		}
	}
}

// computeAll computes all derived values (daily→monthly, daily→yearly, net values).
func (a *Aggregator) computeAll() {
	logger.Debug().Msg("Starting aggregation cycle")

	a.computeDailyToMonthly()
	a.computeDailyToYearly()
	a.computeNetValues()

	logger.Debug().Msg("Aggregation cycle completed")
}

// computeDailyToMonthly computes monthly values from daily storage.
func (a *Aggregator) computeDailyToMonthly() {
	// Skip if storage is not configured
	if a.storage == nil {
		logger.Warn().Msg("Cannot compute monthly values: storage not configured")
		return
	}

	currentMonth := time.Now().Format(solis.MonthFormat)

	for dailyKey, monthlyKey := range solis.DailyToMonthlyMap {
		value, _, err := a.storage.GetMonthlySum(dailyKey, currentMonth)
		if err != nil {
			logger.Warn().Msgf("Failed to compute %s from daily storage: %v", monthlyKey, err)
			continue
		}

		reg, ok := solis.RegisterMapByKey[monthlyKey]
		if !ok {
			logger.Warn().Msgf("Register %s not found in RegisterMapByKey", monthlyKey)
			continue
		}

		// For computed registers, we want RawValue * Scale = value (the already-scaled sum)
		// Since reg.Scale is 1 for these computed registers, RawValue should equal value
		computedValue := &solis.Value{
			Key:          monthlyKey,
			Name:         reg.Name,
			RawValue:     value,
			DecodedValue: value,
			Unit:         reg.Unit,
			Timestamp:    time.Now(),
			DataType:     reg.DataType,
			Stability:    reg.Stability,
		}

		// Update cache with computed value (merges into existing cache)
		a.updateCache(map[string]*solis.Value{monthlyKey: computedValue})
		logger.Debug().Msgf("Computed %s: %.1f %s", monthlyKey, value, reg.Unit)
	}
}

// computeDailyToYearly computes yearly values from daily storage.
func (a *Aggregator) computeDailyToYearly() {
	// Skip if storage is not configured
	if a.storage == nil {
		logger.Warn().Msg("Cannot compute yearly values: storage not configured")
		return
	}

	currentYear := time.Now().Format(solis.YearFormat)

	for dailyKey, yearlyKey := range solis.DailyToYearlyMap {
		value, _, err := a.storage.GetYearlySum(dailyKey, currentYear)
		if err != nil {
			logger.Warn().Msgf("Failed to compute %s from daily storage: %v", yearlyKey, err)
			continue
		}

		reg, ok := solis.RegisterMapByKey[yearlyKey]
		if !ok {
			logger.Warn().Msgf("Register %s not found in RegisterMapByKey", yearlyKey)
			continue
		}

		// For computed registers, we want RawValue * Scale = value (the already-scaled sum)
		computedValue := &solis.Value{
			Key:          yearlyKey,
			Name:         reg.Name,
			RawValue:     value,
			DecodedValue: value,
			Unit:         reg.Unit,
			Timestamp:    time.Now(),
			DataType:     reg.DataType,
			Stability:    reg.Stability,
		}

		// Update cache with computed value (merges into existing cache)
		a.updateCache(map[string]*solis.Value{yearlyKey: computedValue})
		logger.Debug().Msgf("Computed %s: %.1f %s", yearlyKey, value, reg.Unit)
	}
}

// computeNetValues computes net grid energy values.
func (a *Aggregator) computeNetValues() {
	a.computeGridEnergyTotal()
	a.computeGridEnergyDaily()
	a.computeGridEnergyMonthly()
	a.computeGridEnergyYearly()
}

// computeGridEnergyTotal computes grid_energy_total = grid_export_total - grid_import_total.
func (a *Aggregator) computeGridEnergyTotal() {
	fedTotal, err := a.getRegisterValueFromCache("grid_export_total")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_export_total: %v", err)
		return
	}

	importTotal, err := a.getRegisterValueFromCache("grid_import_total")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_import_total: %v", err)
		return
	}

	reg, ok := solis.RegisterMapByKey["grid_energy_total"]
	if !ok {
		logger.Warn().Msg("Register grid_energy_total not found in RegisterMapByKey")
		return
	}

	netValue := fedTotal.DecodedValue - importTotal.DecodedValue
	computedValue := &solis.Value{
		Key:          "grid_energy_total",
		Name:         reg.Name,
		RawValue:     netValue,
		DecodedValue: netValue,
		Unit:         reg.Unit,
		Timestamp:    time.Now(),
		DataType:     reg.DataType,
		Stability:    reg.Stability,
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_total": computedValue})
	logger.Debug().Msgf("Computed grid_energy_total: %.1f kWh", netValue)
}

// computeGridEnergyDaily computes grid_energy_daily = grid_export_daily - grid_import_daily.
func (a *Aggregator) computeGridEnergyDaily() {
	fedDaily, err := a.getRegisterValueFromCache("grid_export_daily")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_export_daily: %v", err)
		return
	}

	importDaily, err := a.getRegisterValueFromCache("grid_import_daily")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_import_daily: %v", err)
		return
	}

	reg, ok := solis.RegisterMapByKey["grid_energy_daily"]
	if !ok {
		logger.Warn().Msg("Register grid_energy_daily not found in RegisterMapByKey")
		return
	}

	netValue := fedDaily.DecodedValue - importDaily.DecodedValue
	computedValue := &solis.Value{
		Key:          "grid_energy_daily",
		Name:         reg.Name,
		RawValue:     netValue,
		DecodedValue: netValue,
		Unit:         reg.Unit,
		Timestamp:    time.Now(),
		DataType:     reg.DataType,
		Stability:    reg.Stability,
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_daily": computedValue})
	logger.Debug().Msgf("Computed grid_energy_daily: %.1f kWh", netValue)
}

// computeGridEnergyMonthly computes grid_energy_monthly = grid_export_monthly - grid_import_monthly.
func (a *Aggregator) computeGridEnergyMonthly() {
	fedMonth, err := a.getRegisterValueFromCache("grid_export_monthly")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_export_monthly: %v", err)
		return
	}

	importMonth, err := a.getRegisterValueFromCache("grid_import_monthly")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_import_monthly: %v", err)
		return
	}

	reg, ok := solis.RegisterMapByKey["grid_energy_monthly"]
	if !ok {
		logger.Warn().Msg("Register grid_energy_monthly not found in RegisterMapByKey")
		return
	}

	netValue := fedMonth.DecodedValue - importMonth.DecodedValue
	computedValue := &solis.Value{
		Key:          "grid_energy_monthly",
		Name:         reg.Name,
		RawValue:     netValue,
		DecodedValue: netValue,
		Unit:         reg.Unit,
		Timestamp:    time.Now(),
		DataType:     reg.DataType,
		Stability:    reg.Stability,
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_monthly": computedValue})
	logger.Debug().Msgf("Computed grid_energy_monthly: %.1f kWh", netValue)
}

// computeGridEnergyYearly computes grid_energy_yearly = grid_export_yearly - grid_import_yearly.
func (a *Aggregator) computeGridEnergyYearly() {
	fedYear, err := a.getRegisterValueFromCache("grid_export_yearly")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_export_yearly: %v", err)
		return
	}

	importYear, err := a.getRegisterValueFromCache("grid_import_yearly")
	if err != nil {
		logger.Warn().Msgf("Failed to get grid_import_yearly: %v", err)
		return
	}

	reg, ok := solis.RegisterMapByKey["grid_energy_yearly"]
	if !ok {
		logger.Warn().Msg("Register grid_energy_yearly not found in RegisterMapByKey")
		return
	}

	netValue := fedYear.DecodedValue - importYear.DecodedValue
	computedValue := &solis.Value{
		Key:          "grid_energy_yearly",
		Name:         reg.Name,
		RawValue:     netValue,
		DecodedValue: netValue,
		Unit:         reg.Unit,
		Timestamp:    time.Now(),
		DataType:     reg.DataType,
		Stability:    reg.Stability,
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_yearly": computedValue})
	logger.Debug().Msgf("Computed grid_energy_yearly: %.1f kWh", netValue)
}

// getRegisterValueFromCache retrieves a register value from cache.
// For net value computations, we rely on the cache having the latest values
// from the most recent poll.
func (a *Aggregator) getRegisterValueFromCache(key string) (*solis.Value, error) {
	if a.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}
	if value := a.cache.Get(key); value != nil {
		return value, nil
	}
	return nil, fmt.Errorf("value not found in cache for %s", key)
}

// updateCache safely updates the cache with computed values.
// Does nothing if cache is not configured.
func (a *Aggregator) updateCache(values map[string]*solis.Value) {
	if a.cache == nil {
		return
	}
	a.cache.Merge(values)
}
