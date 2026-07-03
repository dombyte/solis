// Package service provides business logic orchestration for the Solis monitor application.
// It coordinates between the Modbus client, poller, storage, and HTTP handlers.
package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

// logger is the package-level logger for service operations.
var logger = logging.NewComponentLogger("service")

// ReadService provides read operations for the Solis monitor.
// It handles reading registers, either from cache, storage, or directly from the device.
type ReadService struct {
	// config holds the application configuration.
	config *config.AppConfig
	// modbusClient is the Modbus client for direct reads.
	modbusClient *modbus.Client
	// storage is the storage backend.
	storage *storage.Storage
	// poller is the background poller (may be nil if not using background polling).
	poller *poller.Poller
	// cache holds the latest register values for fast access.
	cache *cache.Cache
	// registerFilter holds the filter for disabled registers.
	registerFilter *solis.RegisterFilter
}

// NewReadService creates a new ReadService instance.
func NewReadService(
	cfg *config.AppConfig,
	modbusClient *modbus.Client,
	st *storage.Storage,
	pl *poller.Poller,
	ca *cache.Cache,
	rf *solis.RegisterFilter,
) *ReadService {
	return &ReadService{
		config:         cfg,
		modbusClient:   modbusClient,
		storage:        st,
		poller:         pl,
		cache:          ca,
		registerFilter: rf,
	}
}

// IsRegisterEnabled returns true if the register key is enabled (not disabled in config).
// Returns false for:
// - Keys that are explicitly disabled in config
// - Keys that don't exist in RegisterMapByKey
// Returns true if:
// - No filter is configured AND the key exists
// - The key exists and is not in the disabled list
func (s *ReadService) IsRegisterEnabled(key string) bool {
	// First check if the register exists
	if _, ok := solis.RegisterMapByKey[key]; !ok {
		return false
	}
	// Then check if it's disabled
	if s.registerFilter == nil {
		return true // Key exists and no filter = enabled
	}
	return s.registerFilter.IsEnabled(key)
}

// GetValues returns values for specific register keys.
// All values are served from cache only.
// Disabled registers are filtered out from the results.
func (s *ReadService) GetValues(keys []string) (map[string]*solis.Value, error) {
	result := make(map[string]*solis.Value)

	// Skip disabled registers and internal keys
	var enabledKeys []string
	for _, key := range keys {
		if s.IsRegisterEnabled(key) && key != "battery_current_direction" {
			enabledKeys = append(enabledKeys, key)
		}
	}

	// Get all values from cache
	if s.cache != nil {
		cachedValues := s.cache.GetMultiple(enabledKeys)
		// Copy all cached values that are found
		for key, value := range cachedValues {
			result[key] = value
		}
	}

	return result, nil
}

// validateRegisterKey validates that a register key is valid, enabled, and exposed via API.
// Returns an error if the key is disabled, doesn't exist, or is an internal-only key.
func (s *ReadService) validateRegisterKey(key string) error {
	// battery_current_direction is used internally for sign correction but not exposed via API
	if key == "battery_current_direction" {
		return fmt.Errorf("unknown register key: %s", key)
	}
	// Check if the register exists
	if _, ok := solis.RegisterMapByKey[key]; !ok {
		return fmt.Errorf("unknown register key: %s", key)
	}
	// Check if the register is enabled
	if !s.IsRegisterEnabled(key) {
		return fmt.Errorf("unknown register key: %s", key)
	}
	return nil
}

// GetRegister returns a single register value.
// Returns an error if the register is disabled or doesn't exist.
func (s *ReadService) GetRegister(key string) (*solis.Value, error) {
	if err := s.validateRegisterKey(key); err != nil {
		return nil, err
	}

	values, err := s.GetValues([]string{key})
	if err != nil {
		return nil, err
	}

	if value, ok := values[key]; ok {
		return value, nil
	}

	return nil, fmt.Errorf("register %s not found", key)
}

// validateRegisterType checks if a register has the specified type.
// Returns an error if the register is not of the expected type.
func (s *ReadService) validateRegisterType(key string, checkFunc func(string) bool, typeName string) error {
	if !checkFunc(key) {
		return fmt.Errorf("register %s is not a %s register", key, typeName)
	}
	return nil
}

// GetHistoricalData returns historical data for a specific register key.
// Parameters:
// - key: the register key (e.g., "pv1_voltage", "battery_soc")
// - start: start time (optional, default: 24 hours ago)
// - end: end time (optional, default: now)
// - interval: only "raw" is supported (aggregated intervals removed)
func (s *ReadService) GetHistoricalData(key string, start, end time.Time, interval storage.Interval) (*storage.HistoryResult, error) {
	if err := s.validateRegisterKey(key); err != nil {
		return nil, err
	}

	// Raw historical data is no longer stored in database - only latest values in cache
	return nil, fmt.Errorf("historical raw data is not available")
}

// filterInternalKeys removes internal-only keys from a list of register keys.
// Currently only filters out "battery_current_direction" which is used internally.
func filterInternalKeys(keys []string) []string {
	var filtered []string
	for _, k := range keys {
		if k != "battery_current_direction" {
			filtered = append(filtered, k)
		}
	}
	return filtered
}

// GetKeys returns all enabled register keys.
// If a register filter is configured, only non-disabled keys are returned.
func (s *ReadService) GetKeys() []string {
	var keys []string
	if s.registerFilter == nil {
		keys = make([]string, 0, len(solis.RegisterMapByKey))
		for k := range solis.RegisterMapByKey {
			keys = append(keys, k)
		}
	} else {
		keys = s.registerFilter.FilterKeys()
	}
	return filterInternalKeys(keys)
}

// GetAllCachedValues returns all enabled values currently in the cache.
// This is used by the metrics endpoint for fast access to all latest values.
// Disabled registers are filtered out from the results.
func (s *ReadService) GetAllCachedValues() map[string]*solis.Value {
	if s.cache == nil {
		return nil
	}
	allValues := s.cache.GetAll()

	// Filter out disabled registers and internal keys
	result := make(map[string]*solis.Value, len(allValues))
	for key, value := range allValues {
		// Skip internal keys
		if key == "battery_current_direction" {
			continue
		}
		// Skip disabled registers
		if s.registerFilter != nil && !s.registerFilter.IsEnabled(key) {
			continue
		}
		result[key] = value
	}
	return result
}

// validateRegisterStatus checks if a register is a status register.
// Returns an error if the register is not a status register.
func (s *ReadService) validateRegisterStatus(key string) error {
	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return fmt.Errorf("unknown register key: %s", key)
	}
	if !reg.Status {
		return fmt.Errorf("register %s is not a status register", key)
	}
	return nil
}

// GetErrorHistory returns historical error data for a specific register key.
func (s *ReadService) GetErrorHistory(key string, start, end time.Time) ([]*storage.ErrorDataPoint, error) {
	if err := s.validateRegisterKey(key); err != nil {
		return nil, err
	}
	if err := s.validateRegisterStatus(key); err != nil {
		return nil, err
	}

	return s.storage.GetErrorHistory(key, start, end)
}

// GetDailyHistory returns daily values for a specific register key.
func (s *ReadService) GetDailyHistory(key string, start, end time.Time) ([]*storage.DailyDataPoint, error) {
	if err := s.validateRegisterKey(key); err != nil {
		return nil, err
	}
	if err := s.validateRegisterType(key, solis.IsDailyRegister, "daily energy"); err != nil {
		return nil, err
	}

	// Handle computed net grid energy register
	if key == "today_grid_energy" {
		return s.getComputedDailyGridEnergy(start, end)
	}

	return s.storage.GetDailyHistory(key, start, end)
}

// getComputedDailyGridEnergy computes the net daily grid energy from source registers.
// Returns today_energy_fed_into_grid - today_energy_imported_from_grid for each day
func (s *ReadService) getComputedDailyGridEnergy(start, end time.Time) ([]*storage.DailyDataPoint, error) {
	// Validate date range
	if end.Before(start) {
		return nil, fmt.Errorf("end date (%s) cannot be before start date (%s)", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	// Get daily fed into grid
	fedDaily, err := s.storage.GetDailyHistory("today_energy_fed_into_grid", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get today_energy_fed_into_grid: %w", err)
	}

	// Get daily imported from grid
	importDaily, err := s.storage.GetDailyHistory("today_energy_imported_from_grid", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get today_energy_imported_from_grid: %w", err)
	}

	// Create a map of date -> value for both
	fedMap := make(map[string]*storage.DailyDataPoint)
	for _, dp := range fedDaily {
		fedMap[dp.Date] = dp
	}

	importMap := make(map[string]*storage.DailyDataPoint)
	for _, dp := range importDaily {
		importMap[dp.Date] = dp
	}

	// Compute net for each date - include ALL dates from both datasets
	var result []*storage.DailyDataPoint
	// Collect all unique dates
	allDates := make(map[string]bool)
	for _, dp := range fedDaily {
		allDates[dp.Date] = true
	}
	for _, dp := range importDaily {
		allDates[dp.Date] = true
	}

	// Sort dates for consistent ordering
	var sortedDates []string
	for date := range allDates {
		sortedDates = append(sortedDates, date)
	}
	sort.Strings(sortedDates)

	for _, date := range sortedDates {
		fedDp, fedExists := fedMap[date]
		importDp, importExists := importMap[date]

		var netValue, netRawValue float64

		if fedExists && importExists {
			netValue = fedDp.Value - importDp.Value
			netRawValue = fedDp.RawValue - importDp.RawValue
		} else if fedExists {
			// If no import data, net = fed value
			netValue = fedDp.Value
			netRawValue = fedDp.RawValue
		} else if importExists {
			// If no fed data, net = -import value
			netValue = -importDp.Value
			netRawValue = -importDp.RawValue
		}

		result = append(result, &storage.DailyDataPoint{
			Date:     date,
			Value:    netValue,
			RawValue: netRawValue,
		})
	}

	return result, nil
}

// GetDeviceInfo returns all stable register values (device information).
// Stable registers are only stored in cache, not in the database.
func (s *ReadService) GetDeviceInfo() (map[string]*solis.Value, error) {
	// Get all stable register keys
	allKeys := s.GetKeys()
	var stableKeys []string
	for _, key := range allKeys {
		if reg, ok := solis.RegisterMapByKey[key]; ok && reg.Stability == solis.Stable {
			stableKeys = append(stableKeys, key)
		}
	}

	if len(stableKeys) == 0 {
		return nil, nil
	}

	// Stable registers are only in cache
	if s.cache == nil {
		return nil, fmt.Errorf("cache not available - stable registers are cache-only")
	}

	return s.cache.GetMultiple(stableKeys), nil
}

// HealthCheck returns a simple health status.
func (s *ReadService) HealthCheck() (map[string]string, error) {
	status := map[string]string{
		"status": "ok",
	}

	if s.modbusClient != nil {
		status["modbus_connected"] = fmt.Sprintf("%v", s.modbusClient.IsConnected())
	} else {
		status["modbus_connected"] = "disabled"
	}

	if s.poller != nil {
		status["poller_running"] = fmt.Sprintf("%v", s.poller.IsRunning())
		if info := s.poller.GetLastPollInfo(); info != nil {
			status["last_poll"] = info.Timestamp.Format(time.RFC3339)
			status["poll_duration_ms"] = fmt.Sprintf("%d", info.DurationMs)
		}
	} else {
		status["poller_running"] = "disabled"
	}

	// Storage status
	if s.storage != nil {
		// Simple connectivity check - just try to ping the database
		if err := s.storage.DB().Ping(); err != nil {
			status["storage"] = "error"
			status["storage_error"] = err.Error()
		} else {
			status["storage"] = "ok"
		}
	} else {
		status["storage"] = "disabled"
	}

	return status, nil
}

// GetMonthlyHistory returns monthly values for a specific register key.
func (s *ReadService) GetMonthlyHistory(key string, start, end time.Time) ([]*storage.MonthlyDataPoint, error) {
	if err := s.validateRegisterKey(key); err != nil {
		return nil, err
	}
	if err := s.validateRegisterType(key, solis.IsMonthlyRegister, "monthly energy"); err != nil {
		return nil, err
	}

	// Handle computed monthly energy registers
	if key == "month_grid_energy" {
		return s.getComputedMonthlyGridEnergy(start, end)
	}

	// Handle computed monthly registers that aggregate daily values
	computedMonthlyKeys := map[string]string{
		"energy_consumption_month_energy":        "today_energy_consumption",
		"energy_fed_into_grid_month_energy":      "today_energy_fed_into_grid",
		"energy_imported_from_grid_month_energy": "today_energy_imported_from_grid",
		"battery_discharge_month_energy":         "today_battery_discharge_energy",
		"battery_charge_month_energy":            "today_battery_charge_energy",
	}
	if dailyKey, ok := computedMonthlyKeys[key]; ok {
		return s.getComputedMonthlyEnergy(dailyKey, start, end, key)
	}

	return s.storage.GetMonthlyHistory(key, start, end)
}

// GetYearlyHistory returns yearly values for a specific register key.
func (s *ReadService) GetYearlyHistory(key string, start, end time.Time) ([]*storage.YearlyDataPoint, error) {
	if err := s.validateRegisterKey(key); err != nil {
		return nil, err
	}
	if err := s.validateRegisterType(key, solis.IsYearlyRegister, "yearly energy"); err != nil {
		return nil, err
	}

	// Handle computed yearly energy registers
	if key == "year_grid_energy" {
		return s.getComputedYearlyGridEnergy(start, end)
	}

	// Handle computed yearly registers that aggregate daily values
	computedYearlyKeys := map[string]string{
		"energy_consumption_year_energy":        "today_energy_consumption",
		"energy_fed_into_grid_year_energy":      "today_energy_fed_into_grid",
		"energy_imported_from_grid_year_energy": "today_energy_imported_from_grid",
		"battery_discharge_year_energy":         "today_battery_discharge_energy",
		"battery_charge_year_energy":            "today_battery_charge_energy",
	}
	if dailyKey, ok := computedYearlyKeys[key]; ok {
		return s.getComputedYearlyEnergy(dailyKey, start, end, key)
	}

	return s.storage.GetYearlyHistory(key, start, end)
}

// GetTotalHistory returns the total (lifetime) value for a specific register key.
func (s *ReadService) GetTotalHistory(key string) (*storage.TotalDataPoint, error) {
	if err := s.validateRegisterKey(key); err != nil {
		return nil, err
	}
	if err := s.validateRegisterType(key, solis.IsTotalRegister, "total energy"); err != nil {
		return nil, err
	}

	// Handle computed net grid energy register
	if key == "total_grid_energy" {
		return s.getComputedTotalGridEnergy()
	}

	return s.storage.GetTotalHistory(key)
}

// getComputedTotalGridEnergy computes the net total grid energy from source registers.
// Returns total_energy_fed_into_grid - total_energy_imported_from_grid
func (s *ReadService) getComputedTotalGridEnergy() (*storage.TotalDataPoint, error) {
	// Get total fed into grid
	fedTotal, err := s.storage.GetTotalHistory("total_energy_fed_into_grid")
	if err != nil {
		return nil, fmt.Errorf("failed to get total_energy_fed_into_grid: %w", err)
	}
	if fedTotal == nil {
		return nil, fmt.Errorf("no data found for total_energy_fed_into_grid")
	}

	// Get total imported from grid
	importTotal, err := s.storage.GetTotalHistory("total_energy_imported_from_grid")
	if err != nil {
		return nil, fmt.Errorf("failed to get total_energy_imported_from_grid: %w", err)
	}
	if importTotal == nil {
		return nil, fmt.Errorf("no data found for total_energy_imported_from_grid")
	}

	// Compute net: positive = export, negative = import
	netValue := fedTotal.Value - importTotal.Value
	netRawValue := fedTotal.RawValue - importTotal.RawValue

	return &storage.TotalDataPoint{
		Value:     netValue,
		RawValue:  netRawValue,
		Timestamp: fedTotal.Timestamp, // Use same timestamp as fed value
	}, nil
}

// getComputedMonthlyEnergy returns monthly history for computed registers by aggregating daily values.
func (s *ReadService) getComputedMonthlyEnergy(dailyKey string, start, end time.Time, monthlyKey string) ([]*storage.MonthlyDataPoint, error) {
	// Validate date range
	if end.Before(start) {
		return nil, fmt.Errorf("end date (%s) cannot be before start date (%s)", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	// Get stored data for the requested range (may be partial)
	stored, err := s.storage.GetMonthlyHistory(monthlyKey, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored monthly history: %w", err)
	}

	// Expand date range to full months to get all daily data
	startExpanded := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	// Add one month to end and go to the first day of that month (exclusive end)
	// This ensures we include all days of the end month
	endExpanded := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location()).AddDate(0, 1, 0)

	// Get all daily values for the expanded date range
	dailyHistory, err := s.storage.GetDailyHistory(dailyKey, startExpanded, endExpanded)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily history: %w", err)
	}

	// Group daily values by month and sum
	// For computed registers, we want RawValue * Scale = Value
	// Since reg.Scale is 1 for these computed registers, RawValue should equal Value
	// Value is the sum of already-scaled daily values
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
		// Sum the already-scaled daily values (dp.Value contains the scaled value from daily_values)
		// For computed registers with Scale=1, both Value and RawValue should equal this sum
		calculatedMap[month].Value += dp.Value
		calculatedMap[month].RawValue += dp.Value
	}

	// Build map of stored data for easy lookup
	storedMap := make(map[string]*storage.MonthlyDataPoint)
	for _, dp := range stored {
		storedMap[dp.Month] = dp
	}

	// Determine current month
	currentMonth := time.Now().Format("2006-01")

	// Merge: for past months, use stored data; for current month, always use calculated
	for month, storedDp := range storedMap {
		// Only use stored data for past months (not current month)
		if month != currentMonth {
			calculatedMap[month] = storedDp
		}
		// For current month, we keep the calculated value (which will be backfilled)
	}

	// Convert map to slice and sort by month
	result := make([]*storage.MonthlyDataPoint, 0, len(calculatedMap))
	for _, dp := range calculatedMap {
		result = append(result, dp)

		// Store computed value in database for future queries
		if storeErr := s.storage.StoreMonthlyDataPoint(monthlyKey, dp); storeErr != nil {
			logger.Warn().Msgf("Failed to store computed monthly value for %s month %s: %v", monthlyKey, dp.Month, storeErr)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Month < result[j].Month
	})

	return result, nil
}

// getComputedMonthlyGridEnergy returns monthly history for the net grid energy register.
// Computes: energy_fed_into_grid_month_energy - energy_imported_from_grid_month_energy
func (s *ReadService) getComputedMonthlyGridEnergy(start, end time.Time) ([]*storage.MonthlyDataPoint, error) {
	// Validate date range
	if end.Before(start) {
		return nil, fmt.Errorf("end date (%s) cannot be before start date (%s)", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	// Get stored net values from database (may be partial)
	stored, err := s.storage.GetMonthlyHistory("month_grid_energy", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored month_grid_energy: %w", err)
	}

	// Expand date range to cover full months
	startExpanded := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	// Add one month to end and go to the first day of that month (exclusive end)
	// This ensures we include all days of the end month
	endExpanded := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location()).AddDate(0, 1, 0)

	// Get source monthly registers using the FIXED aggregation functions
	// These will handle missing data by calculating from daily values
	fedMonthly, err := s.getComputedMonthlyEnergy("today_energy_fed_into_grid", startExpanded, endExpanded, "energy_fed_into_grid_month_energy")
	if err != nil {
		return nil, fmt.Errorf("failed to get fed monthly history: %w", err)
	}

	importMonthly, err := s.getComputedMonthlyEnergy("today_energy_imported_from_grid", startExpanded, endExpanded, "energy_imported_from_grid_month_energy")
	if err != nil {
		return nil, fmt.Errorf("failed to get import monthly history: %w", err)
	}

	// Create maps for easy lookup
	fedMap := make(map[string]*storage.MonthlyDataPoint)
	for _, dp := range fedMonthly {
		fedMap[dp.Month] = dp
	}

	importMap := make(map[string]*storage.MonthlyDataPoint)
	for _, dp := range importMonthly {
		importMap[dp.Month] = dp
	}

	// Build map of stored net values
	storedMap := make(map[string]*storage.MonthlyDataPoint)
	for _, dp := range stored {
		storedMap[dp.Month] = dp
	}

	// Compute net for each month
	var result []*storage.MonthlyDataPoint
	// Collect all unique months from all sources
	allMonths := make(map[string]bool)
	for _, dp := range fedMonthly {
		allMonths[dp.Month] = true
	}
	for _, dp := range importMonthly {
		allMonths[dp.Month] = true
	}
	for _, dp := range stored {
		allMonths[dp.Month] = true
	}

	// Determine current month
	currentMonth := time.Now().Format("2006-01")

	for month := range allMonths {
		// Use stored net value if available and it's not the current month
		// For current month, always calculate to ensure it's up-to-date
		if storedDp, exists := storedMap[month]; exists && month != currentMonth {
			result = append(result, storedDp)
			continue
		}

		fedDp, fedExists := fedMap[month]
		importDp, importExists := importMap[month]

		var netValue, netRawValue float64

		if fedExists && importExists {
			netValue = fedDp.Value - importDp.Value
			netRawValue = fedDp.RawValue - importDp.RawValue
		} else if fedExists {
			netValue = fedDp.Value
			netRawValue = fedDp.RawValue
		} else if importExists {
			netValue = -importDp.Value
			netRawValue = -importDp.RawValue
		}

		netDp := &storage.MonthlyDataPoint{
			Month:    month,
			Value:    netValue,
			RawValue: netRawValue,
		}
		result = append(result, netDp)

		// Store computed net value in database for future queries
		if storeErr := s.storage.StoreMonthlyDataPoint("month_grid_energy", netDp); storeErr != nil {
			logger.Warn().Msgf("Failed to store computed month_grid_energy for month %s: %v", month, storeErr)
		}
	}

	// Sort by month
	sort.Slice(result, func(i, j int) bool {
		return result[i].Month < result[j].Month
	})

	return result, nil
}

// getComputedYearlyEnergy returns yearly history for computed registers by aggregating daily values.
func (s *ReadService) getComputedYearlyEnergy(dailyKey string, start, end time.Time, yearlyKey string) ([]*storage.YearlyDataPoint, error) {
	// Validate date range
	if end.Before(start) {
		return nil, fmt.Errorf("end date (%s) cannot be before start date (%s)", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	// Get stored data for the requested range (may be partial)
	stored, err := s.storage.GetYearlyHistory(yearlyKey, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored yearly history: %w", err)
	}

	// Expand date range to full years to get all daily data
	startExpanded := time.Date(start.Year(), time.January, 1, 0, 0, 0, 0, start.Location())
	// Add one year to end and go to the first day of that year (exclusive end)
	// This ensures we include all days of the end year
	endExpanded := time.Date(end.Year(), time.January, 1, 0, 0, 0, 0, end.Location()).AddDate(1, 0, 0)

	// Get all daily values for the expanded date range
	dailyHistory, err := s.storage.GetDailyHistory(dailyKey, startExpanded, endExpanded)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily history: %w", err)
	}

	// Group daily values by year and sum
	// For computed registers, we want RawValue * Scale = Value
	// Since reg.Scale is 1 for these computed registers, RawValue should equal Value
	// Value is the sum of already-scaled daily values
	calculatedMap := make(map[string]*storage.YearlyDataPoint)
	for _, dp := range dailyHistory {
		year := dp.Date[:4] // "2006-01-02" -> "2006"
		if _, exists := calculatedMap[year]; !exists {
			calculatedMap[year] = &storage.YearlyDataPoint{
				Year:     year,
				Value:    0,
				RawValue: 0,
			}
		}
		// Sum the already-scaled daily values (dp.Value contains the scaled value from daily_values)
		// For computed registers with Scale=1, both Value and RawValue should equal this sum
		calculatedMap[year].Value += dp.Value
		calculatedMap[year].RawValue += dp.Value
	}

	// Build map of stored data for easy lookup
	storedMap := make(map[string]*storage.YearlyDataPoint)
	for _, dp := range stored {
		storedMap[dp.Year] = dp
	}

	// Determine current year
	currentYear := time.Now().Format("2006")

	// Merge: for past years, use stored data; for current year, always use calculated
	for year, storedDp := range storedMap {
		// Only use stored data for past years (not current year)
		if year != currentYear {
			calculatedMap[year] = storedDp
		}
		// For current year, we keep the calculated value (which will be backfilled)
	}

	// Convert map to slice and sort by year
	result := make([]*storage.YearlyDataPoint, 0, len(calculatedMap))
	for _, dp := range calculatedMap {
		result = append(result, dp)

		// Store computed value in database for future queries
		if storeErr := s.storage.StoreYearlyDataPoint(yearlyKey, dp); storeErr != nil {
			logger.Warn().Msgf("Failed to store computed yearly value for %s year %s: %v", yearlyKey, dp.Year, storeErr)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Year < result[j].Year
	})

	return result, nil
}

// getComputedYearlyGridEnergy returns yearly history for the net grid energy register.
// Computes: energy_fed_into_grid_year_energy - energy_imported_from_grid_year_energy
func (s *ReadService) getComputedYearlyGridEnergy(start, end time.Time) ([]*storage.YearlyDataPoint, error) {
	// Validate date range
	if end.Before(start) {
		return nil, fmt.Errorf("end date (%s) cannot be before start date (%s)", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	// Get stored net values from database (may be partial)
	stored, err := s.storage.GetYearlyHistory("year_grid_energy", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored year_grid_energy: %w", err)
	}

	// Expand date range to cover full years
	startExpanded := time.Date(start.Year(), time.January, 1, 0, 0, 0, 0, start.Location())
	// Add one year to end and go to the first day of that year (exclusive end)
	// This ensures we include all days of the end year
	endExpanded := time.Date(end.Year(), time.January, 1, 0, 0, 0, 0, end.Location()).AddDate(1, 0, 0)

	// Get source yearly registers using the FIXED aggregation functions
	// These will handle missing data by calculating from daily values
	fedYearly, err := s.getComputedYearlyEnergy("today_energy_fed_into_grid", startExpanded, endExpanded, "energy_fed_into_grid_year_energy")
	if err != nil {
		return nil, fmt.Errorf("failed to get fed yearly history: %w", err)
	}

	importYearly, err := s.getComputedYearlyEnergy("today_energy_imported_from_grid", startExpanded, endExpanded, "energy_imported_from_grid_year_energy")
	if err != nil {
		return nil, fmt.Errorf("failed to get import yearly history: %w", err)
	}

	// Create maps for easy lookup
	fedMap := make(map[string]*storage.YearlyDataPoint)
	for _, dp := range fedYearly {
		fedMap[dp.Year] = dp
	}

	importMap := make(map[string]*storage.YearlyDataPoint)
	for _, dp := range importYearly {
		importMap[dp.Year] = dp
	}

	// Build map of stored net values
	storedMap := make(map[string]*storage.YearlyDataPoint)
	for _, dp := range stored {
		storedMap[dp.Year] = dp
	}

	// Compute net for each year
	var result []*storage.YearlyDataPoint
	// Collect all unique years from all sources
	allYears := make(map[string]bool)
	for _, dp := range fedYearly {
		allYears[dp.Year] = true
	}
	for _, dp := range importYearly {
		allYears[dp.Year] = true
	}
	for _, dp := range stored {
		allYears[dp.Year] = true
	}

	// Determine current year
	currentYear := time.Now().Format("2006")

	for year := range allYears {
		// Use stored net value if available and it's not the current year
		// For current year, always calculate to ensure it's up-to-date
		if storedDp, exists := storedMap[year]; exists && year != currentYear {
			result = append(result, storedDp)
			continue
		}

		fedDp, fedExists := fedMap[year]
		importDp, importExists := importMap[year]

		var netValue, netRawValue float64

		if fedExists && importExists {
			netValue = fedDp.Value - importDp.Value
			netRawValue = fedDp.RawValue - importDp.RawValue
		} else if fedExists {
			netValue = fedDp.Value
			netRawValue = fedDp.RawValue
		} else if importExists {
			netValue = -importDp.Value
			netRawValue = -importDp.RawValue
		}

		netDp := &storage.YearlyDataPoint{
			Year:     year,
			Value:    netValue,
			RawValue: netRawValue,
		}
		result = append(result, netDp)

		// Store computed net value in database for future queries
		if storeErr := s.storage.StoreYearlyDataPoint("year_grid_energy", netDp); storeErr != nil {
			logger.Warn().Msgf("Failed to store computed year_grid_energy for year %s: %v", year, storeErr)
		}
	}

	// Sort by year
	sort.Slice(result, func(i, j int) bool {
		return result[i].Year < result[j].Year
	})

	return result, nil
}
