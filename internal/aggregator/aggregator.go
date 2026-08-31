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

	// Sync mechanism to wait for first poll completion
	firstPollDone chan struct{}
	firstPollErr  error
}

// New creates a new Aggregator instance.
// If BackfillCurrentYearMonthly is enabled in config, it runs the backfill
// immediately before returning (synchronously).
func New(storage *storage.Storage, cache *cache.Cache, cfg *config.AggregatorSettings) *Aggregator {
	a := &Aggregator{
		storage:      storage,
		cache:        cache,
		config:       cfg,
		stopChan:     make(chan struct{}),
		isRunning:    false,
		firstPollDone: make(chan struct{}),
	}

	logger.Info().Msgf("Aggregator created with interval=%s, backfill_current_year_monthly=%v",
		cfg.Interval, cfg.BackfillCurrentYearMonthly)

	// Run backfill immediately in New() - BEFORE polling starts
	if cfg.BackfillCurrentYearMonthly {
		logger.Info().Msg("BackfillCurrentYearMonthly enabled - running backfill before polling starts")
		a.backfillCurrentYearMonthly()
		logger.Info().Msg("BackfillCurrentYearMonthly completed")
	}

	return a
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

// SignalFirstPollDone is called by the poller after the first poll completes.
// This allows the aggregator to wait for data before starting computation.
func (a *Aggregator) SignalFirstPollDone(err error) {
	a.firstPollErr = err
	close(a.firstPollDone)
}

// run is the main aggregation loop.
func (a *Aggregator) run() {
	defer a.wg.Done()

	// Wait for first poll to complete before starting regular aggregation
	// This ensures we have data to aggregate
	logger.Info().Msg("Aggregator waiting for first poll to complete...")
	select {
	case <-a.firstPollDone:
		if a.firstPollErr != nil {
			logger.Warn().Msgf("First poll completed with error: %v, starting aggregation anyway", a.firstPollErr)
		} else {
			logger.Info().Msg("First poll completed, starting aggregation")
		}
	case <-a.stopChan:
		logger.Info().Msg("Aggregator received stop signal before first poll")
		return
	}

	// Initial computation after first poll
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

	a.computeAndStoreDailyToMonthly()
	a.computeAndStoreDailyToYearly()
	a.computeAndStoreNetValues()

	logger.Debug().Msg("Aggregation cycle completed")
}

// computeAndStoreDailyToMonthly computes monthly values from daily storage and stores in DB.
// This only processes COMPUTED monthly registers (those in DailyToMonthlyMap), not directly-polled ones.
// Directly-polled monthly registers (like pv_energy_monthly, household_energy_monthly) are stored
// by the poller and should not be overwritten by the aggregator during normal operation.
func (a *Aggregator) computeAndStoreDailyToMonthly() {
	// Skip if storage is not configured
	if a.storage == nil {
		logger.Warn().Msg("Cannot compute monthly values: storage not configured")
		return
	}

	currentMonth := time.Now().Format(solis.MonthFormat)

	for dailyKey, monthlyKey := range solis.DailyToMonthlyMap {
		value, rawValue, err := a.storage.GetMonthlySum(dailyKey, currentMonth)
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

		// Store in database
		monthlyDp := &storage.MonthlyDataPoint{
			Month:    currentMonth,
			Value:    value,
			RawValue: rawValue,
		}
		if storeErr := a.storage.StoreMonthlyDataPoint(monthlyKey, monthlyDp); storeErr != nil {
			logger.Warn().Msgf("Failed to store monthly value for %s: %v", monthlyKey, storeErr)
		}

		// Update cache with computed value (merges into existing cache)
		a.updateCache(map[string]*solis.Value{monthlyKey: computedValue})
		logger.Debug().Msgf("Computed and stored %s: %.1f %s", monthlyKey, value, reg.Unit)
	}
}

// computeAndStoreDailyToYearly computes yearly values from daily storage and stores in DB.
// This only processes COMPUTED yearly registers (those in DailyToYearlyMap), not directly-polled ones.
// Directly-polled yearly registers (like pv_energy_yearly, household_energy_yearly) are stored
// by the poller and should not be overwritten by the aggregator during normal operation.
func (a *Aggregator) computeAndStoreDailyToYearly() {
	// Skip if storage is not configured
	if a.storage == nil {
		logger.Warn().Msg("Cannot compute yearly values: storage not configured")
		return
	}

	currentYear := time.Now().Format(solis.YearFormat)

	for dailyKey, yearlyKey := range solis.DailyToYearlyMap {
		value, rawValue, err := a.storage.GetYearlySum(dailyKey, currentYear)
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

		// Store in database
		yearlyDp := &storage.YearlyDataPoint{
			Year:     currentYear,
			Value:    value,
			RawValue: rawValue,
		}
		if storeErr := a.storage.StoreYearlyDataPoint(yearlyKey, yearlyDp); storeErr != nil {
			logger.Warn().Msgf("Failed to store yearly value for %s: %v", yearlyKey, storeErr)
		}

		// Update cache with computed value (merges into existing cache)
		a.updateCache(map[string]*solis.Value{yearlyKey: computedValue})
		logger.Debug().Msgf("Computed and stored %s: %.1f %s", yearlyKey, value, reg.Unit)
	}
}

// computeAndStoreNetValues computes net grid energy values and stores them in DB.
func (a *Aggregator) computeAndStoreNetValues() {
	a.computeAndStoreGridEnergyTotal()
	a.computeAndStoreGridEnergyDaily()
	a.computeAndStoreGridEnergyMonthly()
	a.computeAndStoreGridEnergyYearly()
}

// computeAndStoreGridEnergyTotal computes grid_energy_total = grid_export_total - grid_import_total and stores in DB.
func (a *Aggregator) computeAndStoreGridEnergyTotal() {
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
	netRawValue := fedTotal.RawValue - importTotal.RawValue
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

	// Store in database
	totalDp := &storage.TotalDataPoint{
		Value:     netValue,
		RawValue:  netRawValue,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if storeErr := a.storage.StoreTotalDataPoint("grid_energy_total", totalDp); storeErr != nil {
		logger.Warn().Msgf("Failed to store total value for grid_energy_total: %v", storeErr)
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_total": computedValue})
	logger.Debug().Msgf("Computed and stored grid_energy_total: %.1f kWh", netValue)
}

// computeAndStoreGridEnergyDaily computes grid_energy_daily = grid_export_daily - grid_import_daily and stores in DB.
func (a *Aggregator) computeAndStoreGridEnergyDaily() {
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
	netRawValue := fedDaily.RawValue - importDaily.RawValue
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

	// Store in database - use StoreAllRegisters to store as daily value
	// Since grid_energy_daily is computed, we store it directly via StoreAllRegisters
	values := map[string]*solis.Value{
		"grid_energy_daily": {
			Key:          "grid_energy_daily",
			Name:         reg.Name,
			RawValue:     netRawValue,
			DecodedValue: netValue,
			Unit:         reg.Unit,
			Timestamp:    time.Now(),
			DataType:     reg.DataType,
			Stability:    reg.Stability,
		},
	}
	if a.storage != nil {
		if storeErr := a.storage.StoreAllRegisters(values, time.Now()); storeErr != nil {
			logger.Warn().Msgf("Failed to store daily value for grid_energy_daily: %v", storeErr)
		}
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_daily": computedValue})
	logger.Debug().Msgf("Computed and stored grid_energy_daily: %.1f kWh", netValue)
}

// computeAndStoreGridEnergyMonthly computes grid_energy_monthly = grid_export_monthly - grid_import_monthly and stores in DB.
func (a *Aggregator) computeAndStoreGridEnergyMonthly() {
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
	netRawValue := fedMonth.RawValue - importMonth.RawValue
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

	// Store in database
	currentMonth := time.Now().Format(solis.MonthFormat)
	monthlyDp := &storage.MonthlyDataPoint{
		Month:    currentMonth,
		Value:    netValue,
		RawValue: netRawValue,
	}
	if storeErr := a.storage.StoreMonthlyDataPoint("grid_energy_monthly", monthlyDp); storeErr != nil {
		logger.Warn().Msgf("Failed to store monthly value for grid_energy_monthly: %v", storeErr)
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_monthly": computedValue})
	logger.Debug().Msgf("Computed and stored grid_energy_monthly: %.1f kWh", netValue)
}

// computeAndStoreGridEnergyYearly computes grid_energy_yearly = grid_export_yearly - grid_import_yearly and stores in DB.
func (a *Aggregator) computeAndStoreGridEnergyYearly() {
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
	netRawValue := fedYear.RawValue - importYear.RawValue
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

	// Store in database
	currentYear := time.Now().Format(solis.YearFormat)
	yearlyDp := &storage.YearlyDataPoint{
		Year:     currentYear,
		Value:    netValue,
		RawValue: netRawValue,
	}
	if storeErr := a.storage.StoreYearlyDataPoint("grid_energy_yearly", yearlyDp); storeErr != nil {
		logger.Warn().Msgf("Failed to store yearly value for grid_energy_yearly: %v", storeErr)
	}

	a.updateCache(map[string]*solis.Value{"grid_energy_yearly": computedValue})
	logger.Debug().Msgf("Computed and stored grid_energy_yearly: %.1f kWh", netValue)
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

// backfillCurrentYearMonthly recomputes and overwrites ALL monthly data for the current year.
// This runs once at startup if BackfillCurrentYearMonthly is enabled in config.
// 
// NOTE: This uses BackfillDailyToMonthlyMap which includes BOTH:
//   - Computed monthly registers (energy_consumption_monthly, grid_export_monthly, etc.)
//   - Directly-polled monthly registers (pv_energy_monthly, household_energy_monthly, backup_energy_monthly)
// 
// For directly-polled registers, the backfill will OVERWRITE the polled values with computed
// values from daily aggregation. This is intentional when backfill is enabled.
func (a *Aggregator) backfillCurrentYearMonthly() {
	currentYear := time.Now().Format("2006")
	logger.Info().Msgf("Starting backfill of monthly data for year %s", currentYear)

	for dailyKey, monthlyKey := range solis.BackfillDailyToMonthlyMap {
		// Get all daily data for current year
		startDate := time.Date(time.Now().Year(), time.January, 1, 0, 0, 0, 0, time.Local)
		endDate := time.Date(time.Now().Year(), time.December, 31, 23, 59, 59, 0, time.Local)

		dailyHistory, err := a.storage.GetDailyHistory(dailyKey, startDate, endDate)
		if err != nil {
			logger.Error().Msgf("Failed to get daily history for %s: %v", dailyKey, err)
			continue
		}

		// Aggregate to monthly
		monthlyMap := aggregateDailyToMonthly(dailyHistory)

		// Store each monthly value (overwrites existing)
		for month, dp := range monthlyMap {
			// For backfill, we need to FORCE overwrite existing values
			// Use a transaction to delete and re-insert
			tx, err := a.storage.DB().Begin()
			if err != nil {
				logger.Error().Msgf("Failed to begin backfill transaction for %s: %v", monthlyKey, err)
				continue
			}
			
			// Delete existing entry for this month if it exists
			_, err = tx.Exec(`DELETE FROM monthly_values WHERE register_key = ? AND month = ?`, monthlyKey, month)
			if err != nil {
				tx.Rollback()
				logger.Error().Msgf("Failed to delete existing monthly value for %s month %s: %v",
					monthlyKey, month, err)
				continue
			}
			
			// For backfill, dp.Value is already the sum of scaled daily values,
			// so we use it directly without any scaling
			decodedValue := dp.Value
			
			// Insert new value
			_, err = tx.Exec(`
				INSERT INTO monthly_values (month, register_key, value, raw_value)
				VALUES (?, ?, ?, ?)
			`, month, monthlyKey, decodedValue, dp.RawValue)
			
			if err != nil {
				tx.Rollback()
				logger.Error().Msgf("Failed to backfill monthly value for %s month %s: %v",
					monthlyKey, month, err)
			} else {
				tx.Commit()
				logger.Debug().Msgf("Backfilled %s month %s: %.1f", monthlyKey, month, dp.Value)
			}
		}
	}

	logger.Info().Msg("Monthly backfill completed for current year")
}

// aggregateDailyToMonthly aggregates daily data points into monthly data points
func aggregateDailyToMonthly(dailyHistory []*storage.DailyDataPoint) map[string]*storage.MonthlyDataPoint {
	calculatedMap := make(map[string]*storage.MonthlyDataPoint)
	for _, dp := range dailyHistory {
		month := dp.Date[:7] // "2006-01-02" -> "2006-01"
		if _, exists := calculatedMap[month]; !exists {
			calculatedMap[month] = &storage.MonthlyDataPoint{
				Month:    month,
				Value:    0,
				RawValue: 0,
			}
		}
		// Sum the already-scaled daily values for Value
		// Sum the raw daily values for RawValue
		calculatedMap[month].Value += dp.Value
		calculatedMap[month].RawValue += dp.RawValue
	}
	return calculatedMap
}
