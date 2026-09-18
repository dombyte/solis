// Package aggregator provides background computation of derived register values
// (monthly, yearly, net values) separate from the polling cycle.
package aggregator

import (
	"database/sql"
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
	// firstPollSignaled tracks if SignalFirstPollDone has been called to prevent double-close
	firstPollSignaled bool
}

// New creates a new Aggregator instance.
// If BackfillCurrentYearMonthly is enabled in config, it runs the backfill
// immediately before returning (synchronously).
func New(storage *storage.Storage, cache *cache.Cache, cfg *config.AggregatorSettings) *Aggregator {
	a := &Aggregator{
		storage:       storage,
		cache:         cache,
		config:        cfg,
		stopChan:      make(chan struct{}),
		isRunning:     false,
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
// This method is idempotent - multiple calls will not cause a panic.
func (a *Aggregator) SignalFirstPollDone(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Prevent double-close of channel
	if a.firstPollSignaled {
		return
	}
	a.firstPollSignaled = true
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

	// Compute and store daily→monthly aggregated values
	a.computeAndStoreMonthlyAggregatedValues()

	// Compute and store daily→yearly aggregated values
	a.computeAndStoreYearlyAggregatedValues()

	a.computeAndStoreNetValues()

	logger.Debug().Msg("Aggregation cycle completed")
}

// computeAndStoreAggregatedValues is a generic helper that computes and stores aggregated values
// (monthly from daily, yearly from daily) by summing daily values for the given time period.
func (a *Aggregator) computeAndStoreAggregatedValues(
	keyMap map[string]string,
	format string,
	getSum func(string, string) (float64, float64, error),
	storeFunc func(string, string, float64, float64) error,
) {
	if a.storage == nil {
		logger.Warn().Msg("Cannot compute values: storage not configured")
		return
	}

	timeStr := time.Now().Format(format)

	for dailyKey, targetKey := range keyMap {
		value, rawValue, err := getSum(dailyKey, timeStr)
		if err != nil {
			logger.Warn().Msgf("Failed to compute %s from daily storage: %v", targetKey, err)
			continue
		}

		reg, ok := solis.RegisterMapByKey[targetKey]
		if !ok {
			logger.Warn().Msgf("Register %s not found in RegisterMapByKey", targetKey)
			continue
		}

		// For computed registers, we want RawValue * Scale = value (the already-scaled sum)
		// Since reg.Scale is 1 for these computed registers, RawValue should equal value
		computedValue := &solis.Value{
			Key:          targetKey,
			Name:         reg.Name,
			RawValue:     value,
			DecodedValue: value,
			Unit:         reg.Unit,
			Timestamp:    time.Now(),
			DataType:     reg.DataType,
			Stability:    reg.Stability,
		}

		// Store in database
		if storeErr := storeFunc(targetKey, timeStr, value, rawValue); storeErr != nil {
			logger.Warn().Msgf("Failed to store value for %s: %v", targetKey, storeErr)
		}

		// Update cache with computed value (merges into existing cache)
		a.updateCache(map[string]*solis.Value{targetKey: computedValue})
		logger.Debug().Msgf("Computed and stored %s: %.1f %s", targetKey, value, reg.Unit)
	}
}

// computeAndStoreMonthlyAggregatedValues computes and stores daily→monthly aggregated values.
//
//nolint:dupl // Different period (monthly) from computeAndStoreYearlyAggregatedValues
func (a *Aggregator) computeAndStoreMonthlyAggregatedValues() {
	a.computeAndStoreAggregatedValues(
		solis.DailyToMonthlyMap,
		solis.MonthFormat,
		a.storage.GetMonthlySum,
		func(key, timeStr string, value, rawValue float64) error {
			dp := &storage.MonthlyDataPoint{
				Month:    timeStr,
				Value:    value,
				RawValue: rawValue,
			}
			return a.storage.StoreMonthlyDataPoint(key, dp)
		},
	)
}

// computeAndStoreYearlyAggregatedValues computes and stores daily→yearly aggregated values.
//
//nolint:dupl // Different period (yearly) from computeAndStoreMonthlyAggregatedValues
func (a *Aggregator) computeAndStoreYearlyAggregatedValues() {
	a.computeAndStoreAggregatedValues(
		solis.DailyToYearlyMap,
		solis.YearFormat,
		a.storage.GetYearlySum,
		func(key, timeStr string, value, rawValue float64) error {
			dp := &storage.YearlyDataPoint{
				Year:     timeStr,
				Value:    value,
				RawValue: rawValue,
			}
			return a.storage.StoreYearlyDataPoint(key, dp)
		},
	)
}

// GridEnergyConfig holds configuration for computing grid energy values
type GridEnergyConfig struct {
	ExportKey     string
	ImportKey     string
	TargetKey     string
	TimeFormat    string
	StoreFunc     func(string, interface{}) error
	DataPointFunc func(float64, string) interface{}
}

// computeAndStoreNetValues computes net grid energy values and stores them in DB.
func (a *Aggregator) computeAndStoreNetValues() {
	// Compute grid_energy_total = grid_export_total - grid_import_total
	a.computeGridEnergyTotal()

	// Compute grid_energy_daily = grid_export_daily - grid_import_daily
	a.computeGridEnergyDaily()

	// Compute grid_energy_monthly = grid_export_monthly - grid_import_monthly
	a.computeGridEnergyMonthly()

	// Compute grid_energy_yearly = grid_export_yearly - grid_import_yearly
	a.computeGridEnergyYearly()
}

// computeGridEnergy is a generic helper that computes grid_energy = grid_export - grid_import
// and stores the result using the provided storage function.
func (a *Aggregator) computeGridEnergy(cfg GridEnergyConfig) {
	exportVal, err := a.getRegisterValueFromCache(cfg.ExportKey)
	if err != nil {
		logger.Warn().Msgf("Failed to get %s: %v", cfg.ExportKey, err)
		return
	}

	importVal, err := a.getRegisterValueFromCache(cfg.ImportKey)
	if err != nil {
		logger.Warn().Msgf("Failed to get %s: %v", cfg.ImportKey, err)
		return
	}

	reg, ok := solis.RegisterMapByKey[cfg.TargetKey]
	if !ok {
		logger.Warn().Msgf("Register %s not found in RegisterMapByKey", cfg.TargetKey)
		return
	}

	netValue := exportVal.DecodedValue - importVal.DecodedValue
	computedValue := &solis.Value{
		Key:          cfg.TargetKey,
		Name:         reg.Name,
		RawValue:     netValue,
		DecodedValue: netValue,
		Unit:         reg.Unit,
		Timestamp:    time.Now(),
		DataType:     reg.DataType,
		Stability:    reg.Stability,
	}

	// Store in database
	timeStr := time.Now().Format(cfg.TimeFormat)
	if storeErr := cfg.StoreFunc(cfg.TargetKey, cfg.DataPointFunc(netValue, timeStr)); storeErr != nil {
		logger.Warn().Msgf("Failed to store value for %s: %v", cfg.TargetKey, storeErr)
	}

	a.updateCache(map[string]*solis.Value{cfg.TargetKey: computedValue})
	logger.Debug().Msgf("Computed and stored %s: %.1f kWh", cfg.TargetKey, netValue)
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

// computeGridEnergyTotal computes grid_energy_total = grid_export_total - grid_import_total.
//
//nolint:dupl // Different target (total) from computeGridEnergyDaily/computeGridEnergyMonthly/computeGridEnergyYearly
func (a *Aggregator) computeGridEnergyTotal() {
	a.computeGridEnergy(GridEnergyConfig{
		ExportKey:  "grid_export_total",
		ImportKey:  "grid_import_total",
		TargetKey:  "grid_energy_total",
		TimeFormat: time.RFC3339,
		StoreFunc: func(key string, dp interface{}) error {
			return a.storage.StoreTotalDataPoint(key, dp.(*storage.TotalDataPoint))
		},
		DataPointFunc: func(value float64, timeStr string) interface{} {
			return &storage.TotalDataPoint{
				Value:     value,
				RawValue:  value,
				Timestamp: timeStr,
			}
		},
	})
}

// computeGridEnergyDaily computes grid_energy_daily = grid_export_daily - grid_import_daily.
//

func (a *Aggregator) computeGridEnergyDaily() {
	a.computeGridEnergy(GridEnergyConfig{
		ExportKey:  "grid_export_daily",
		ImportKey:  "grid_import_daily",
		TargetKey:  "grid_energy_daily",
		TimeFormat: "",
		StoreFunc: func(key string, dp interface{}) error {
			values := map[string]*solis.Value{key: dp.(*solis.Value)}
			if a.storage != nil {
				return a.storage.StoreAllRegisters(values, time.Now())
			}
			return nil
		},
		DataPointFunc: func(value float64, _ string) interface{} {
			reg := solis.RegisterMapByKey["grid_energy_daily"]
			return &solis.Value{
				Key:          "grid_energy_daily",
				Name:         reg.Name,
				RawValue:     value,
				DecodedValue: value,
				Unit:         reg.Unit,
				Timestamp:    time.Now(),
				DataType:     reg.DataType,
				Stability:    reg.Stability,
			}
		},
	})
}

// computeGridEnergyMonthly computes grid_energy_monthly = grid_export_monthly - grid_import_monthly.
//
//nolint:dupl // Different target (monthly) from computeGridEnergyTotal/computeGridEnergyDaily/computeGridEnergyYearly
func (a *Aggregator) computeGridEnergyMonthly() {
	a.computeGridEnergy(GridEnergyConfig{
		ExportKey:  "grid_export_monthly",
		ImportKey:  "grid_import_monthly",
		TargetKey:  "grid_energy_monthly",
		TimeFormat: solis.MonthFormat,
		StoreFunc: func(key string, dp interface{}) error {
			return a.storage.StoreMonthlyDataPoint(key, dp.(*storage.MonthlyDataPoint))
		},
		DataPointFunc: func(value float64, timeStr string) interface{} {
			return &storage.MonthlyDataPoint{
				Month:    timeStr,
				Value:    value,
				RawValue: value,
			}
		},
	})
}

// computeGridEnergyYearly computes grid_energy_yearly = grid_export_yearly - grid_import_yearly.
//
//nolint:dupl // Different target (yearly) from computeGridEnergyTotal/computeGridEnergyDaily/computeGridEnergyMonthly
func (a *Aggregator) computeGridEnergyYearly() {
	a.computeGridEnergy(GridEnergyConfig{
		ExportKey:  "grid_export_yearly",
		ImportKey:  "grid_import_yearly",
		TargetKey:  "grid_energy_yearly",
		TimeFormat: solis.YearFormat,
		StoreFunc: func(key string, dp interface{}) error {
			return a.storage.StoreYearlyDataPoint(key, dp.(*storage.YearlyDataPoint))
		},
		DataPointFunc: func(value float64, timeStr string) interface{} {
			return &storage.YearlyDataPoint{
				Year:     timeStr,
				Value:    value,
				RawValue: value,
			}
		},
	})
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
				if rbErr := tx.Rollback(); rbErr != nil {
					logger.Error().Msgf("Failed to rollback transaction: %v", rbErr)
				}
				logger.Error().Msgf("Failed to delete existing monthly value for %s month %s: %v",
					monthlyKey, month, err)
				continue
			}

			// For backfill, dp.Value is already the sum of scaled daily values,
			// so we use it directly without any scaling
			decodedValue := dp.Value

			// Get target register to compute raw_value based on its scale
			reg, ok := solis.RegisterMapByKey[monthlyKey]
			if !ok {
				if rbErr := tx.Rollback(); rbErr != nil {
					logger.Error().Msgf("Failed to rollback transaction: %v", rbErr)
				}
				logger.Error().Msgf("Register %s not found in RegisterMapByKey", monthlyKey)
				continue
			}
			rawValueForStorage := decodedValue / reg.Scale

			// Insert new value
			_, err = tx.Exec(`
				INSERT INTO monthly_values (month, register_key, value, raw_value)
				VALUES (?, ?, ?, ?)
			`, month, monthlyKey, decodedValue, rawValueForStorage)

			if err != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					logger.Error().Msgf("Failed to rollback transaction: %v", rbErr)
				}
				logger.Error().Msgf("Failed to backfill monthly value for %s month %s: %v",
					monthlyKey, month, err)
			} else {
				if commitErr := tx.Commit(); commitErr != nil {
					logger.Error().Msgf("Failed to commit transaction: %v", commitErr)
				} else {
					logger.Debug().Msgf("Backfilled %s month %s: %.1f", monthlyKey, month, dp.Value)
				}
			}
		}
	}

	// After backfilling all monthly values, also compute net monthly values
	// (like grid_energy_monthly) which depend on other monthly values
	a.backfillNetMonthlyValues()

	logger.Info().Msg("Monthly backfill completed for current year")
}

// backfillNetMonthlyValues computes net monthly values (like grid_energy_monthly) from
// the backfilled monthly values for the entire current year.
// This is needed because net values depend on other monthly values that were just backfilled,
// and the cache may not have them yet.
func (a *Aggregator) backfillNetMonthlyValues() {
	// Get all months for current year that have backfilled grid_export/import values
	currentYear := time.Now().Format("2006")

	// Get all months for grid_export_monthly in current year
	rows, err := a.storage.DB().Query(`
		SELECT month, value, raw_value 
		FROM monthly_values 
		WHERE register_key = ? AND month LIKE ?
		ORDER BY month
	`, "grid_export_monthly", currentYear+"-%")

	if err != nil {
		logger.Warn().Msgf("Failed to query grid_export_monthly months: %v", err)
		return
	}
	defer func() {
		if err := rows.Close(); err != nil {
			logger.Warn().Msgf("Failed to close rows: %v", err)
		}
	}()

	// Process each month
	for rows.Next() {
		var month string
		var fedMonthValue, fedMonthRawValue float64
		if err := rows.Scan(&month, &fedMonthValue, &fedMonthRawValue); err != nil {
			logger.Warn().Msgf("Failed to scan grid_export_monthly: %v", err)
			continue
		}

		// Get corresponding grid_import_monthly value
		var importMonthValue, importMonthRawValue float64
		err = a.storage.DB().QueryRow(`
			SELECT value, raw_value FROM monthly_values 
			WHERE register_key = ? AND month = ?
		`, "grid_import_monthly", month).Scan(&importMonthValue, &importMonthRawValue)

		if err != nil {
			if err == sql.ErrNoRows {
				logger.Debug().Msgf("No grid_import_monthly found for month %s, skipping grid_energy_monthly", month)
			} else {
				logger.Warn().Msgf("Failed to get grid_import_monthly for month %s: %v", month, err)
			}
			continue
		}

		// Compute net value
		netValue := fedMonthValue - importMonthValue
		// For grid_energy_monthly (scale=1), raw_value should equal value
		netRawValue := netValue

		// Store the net monthly value
		tx, err := a.storage.DB().Begin()
		if err != nil {
			logger.Error().Msgf("Failed to begin transaction for grid_energy_monthly backfill: %v", err)
			continue
		}

		// Delete existing entry for this month if it exists
		_, err = tx.Exec(`DELETE FROM monthly_values WHERE register_key = ? AND month = ?`, "grid_energy_monthly", month)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Error().Msgf("Failed to rollback transaction: %v", rbErr)
			}
			logger.Error().Msgf("Failed to delete existing grid_energy_monthly for month %s: %v", month, err)
			continue
		}

		// Insert new value
		_, err = tx.Exec(`
			INSERT INTO monthly_values (month, register_key, value, raw_value)
			VALUES (?, ?, ?, ?)
		`, month, "grid_energy_monthly", netValue, netRawValue)

		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Error().Msgf("Failed to rollback transaction: %v", rbErr)
			}
			logger.Error().Msgf("Failed to backfill grid_energy_monthly for month %s: %v", month, err)
		} else {
			if commitErr := tx.Commit(); commitErr != nil {
				logger.Error().Msgf("Failed to commit transaction: %v", commitErr)
			} else {
				logger.Debug().Msgf("Backfilled grid_energy_monthly for month %s: %.1f", month, netValue)
			}
		}
	}

	if err := rows.Err(); err != nil {
		logger.Warn().Msgf("Error iterating grid_export_monthly months: %v", err)
	}
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
