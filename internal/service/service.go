// Package service provides business logic orchestration for the Solis monitor application.
// It coordinates between the Modbus client, poller, storage, and HTTP handlers.
package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/dombyte/solis/internal/aggregator"
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
	// aggregator is the background aggregator for computed values (may be nil if disabled).
	aggregator *aggregator.Aggregator
	// cache holds the latest register values for fast access.
	cache *cache.Cache
	// registerFilter removed per v2 plan - all registers are always enabled
}

// NewReadService creates a new ReadService instance.
func NewReadService(
	cfg *config.AppConfig,
	modbusClient *modbus.Client,
	st *storage.Storage,
	pl *poller.Poller,
	ca *cache.Cache,
	ag *aggregator.Aggregator,
) *ReadService {
	return &ReadService{
		config:       cfg,
		modbusClient: modbusClient,
		storage:      st,
		poller:       pl,
		aggregator:   ag,
		cache:        ca,
	}
}

// IsRegisterEnabled returns true if the register key is valid.
// All registers in RegisterMapByKey are considered enabled.
func (s *ReadService) IsRegisterEnabled(key string) bool {
	// Check if the register exists
	_, ok := solis.RegisterMapByKey[key]
	return ok
}

// GetValues returns values for specific register keys from cache.
// Only valid registers present in the cache are returned.
func (s *ReadService) GetValues(keys []string) (map[string]*solis.Value, error) {
	result := make(map[string]*solis.Value)

	// Skip disabled registers
	var enabledKeys []string
	for _, key := range keys {
		if s.IsRegisterEnabled(key) {
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
// Returns an error if the key is disabled or doesn't exist.
func (s *ReadService) validateRegisterKey(key string) error {
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

// GetKeys returns all register keys.
func (s *ReadService) GetKeys() []string {
	keys := make([]string, 0, len(solis.RegisterMapByKey))
	for k := range solis.RegisterMapByKey {
		keys = append(keys, k)
	}
	return keys
}

// GetAllCachedValues returns all values currently in the cache.
func (s *ReadService) GetAllCachedValues() map[string]*solis.Value {
	if s.cache == nil {
		return nil
	}
	return s.cache.GetAll()
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
	if key == "grid_energy_daily" {
		return s.getComputedDailyGridEnergy(start, end)
	}

	return s.storage.GetDailyHistory(key, start, end)
}

// getComputedDailyGridEnergy computes the net daily grid energy from source registers.
// Returns grid_export_daily - grid_import_daily for each day
func (s *ReadService) getComputedDailyGridEnergy(start, end time.Time) ([]*storage.DailyDataPoint, error) {
	if err := validateDateRange(start, end); err != nil {
		return nil, err
	}

	fedDaily, err := s.storage.GetDailyHistory("grid_export_daily", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get grid_export_daily: %w", err)
	}

	importDaily, err := s.storage.GetDailyHistory("grid_import_daily", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get grid_import_daily: %w", err)
	}

	return computeNetDailyEnergy(fedDaily, importDaily)
}

// computeNetDailyEnergy computes net daily energy from fed and import data
func computeNetDailyEnergy(fed, imp []*storage.DailyDataPoint) ([]*storage.DailyDataPoint, error) {
	fedMap := buildDailyDataPointMap(fed)
	importMap := buildDailyDataPointMap(imp)

	allDates := collectUniqueDates(fed, imp)
	var sortedDates []string
	for date := range allDates {
		sortedDates = append(sortedDates, date)
	}
	sort.Strings(sortedDates)

	var result []*storage.DailyDataPoint
	for _, date := range sortedDates {
		netDp := computeNetDailyDataPoint(date, fedMap[date], importMap[date])
		if netDp != nil {
			result = append(result, netDp)
		}
	}

	return result, nil
}

// buildDailyDataPointMap creates a map from DailyDataPoint slice
func buildDailyDataPointMap(dps []*storage.DailyDataPoint) map[string]*storage.DailyDataPoint {
	m := make(map[string]*storage.DailyDataPoint)
	for _, dp := range dps {
		m[dp.Date] = dp
	}
	return m
}

// collectUniqueDates collects all unique dates from multiple DailyDataPoint slices
func collectUniqueDates(slices ...[]*storage.DailyDataPoint) map[string]bool {
	allDates := make(map[string]bool)
	for _, slice := range slices {
		for _, dp := range slice {
			allDates[dp.Date] = true
		}
	}
	return allDates
}

// computeNetDailyDataPoint computes net value from fed and import data points
func computeNetDailyDataPoint(date string, fedDp, importDp *storage.DailyDataPoint) *storage.DailyDataPoint {
	var netValue, netRawValue float64

	if fedDp != nil && importDp != nil {
		netValue = fedDp.Value - importDp.Value
		netRawValue = fedDp.RawValue - importDp.RawValue
	} else if fedDp != nil {
		netValue = fedDp.Value
		netRawValue = fedDp.RawValue
	} else if importDp != nil {
		netValue = -importDp.Value
		netRawValue = -importDp.RawValue
	} else {
		return nil
	}

	return &storage.DailyDataPoint{
		Date:     date,
		Value:    netValue,
		RawValue: netRawValue,
	}
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

	if s.aggregator != nil {
		status["aggregator_running"] = fmt.Sprintf("%v", s.aggregator.IsRunning())
	} else {
		status["aggregator_running"] = "disabled"
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
	if key == "grid_energy_monthly" {
		return s.getComputedMonthlyGridEnergy(start, end)
	}

	// Handle computed monthly registers that aggregate daily values
	computedMonthlyKeys := map[string]string{
		"energy_consumption_monthly": "energy_consumption_daily",
		"grid_export_monthly":        "grid_export_daily",
		"grid_import_monthly":        "grid_import_daily",
		"battery_discharge_monthly":  "battery_discharge_daily",
		"battery_charge_monthly":     "battery_charge_daily",
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
	if key == "grid_energy_yearly" {
		return s.getComputedYearlyGridEnergy(start, end)
	}

	// Handle computed yearly registers that aggregate daily values
	computedYearlyKeys := map[string]string{
		"energy_consumption_yearly": "energy_consumption_daily",
		"grid_export_yearly":        "grid_export_daily",
		"grid_import_yearly":        "grid_import_daily",
		"battery_discharge_yearly":  "battery_discharge_daily",
		"battery_charge_yearly":     "battery_charge_daily",
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
	if key == "grid_energy_total" {
		return s.getComputedTotalGridEnergy()
	}

	return s.storage.GetTotalHistory(key)
}

// getComputedTotalGridEnergy computes the net total grid energy from source registers.
// Returns grid_export_total - grid_import_total
func (s *ReadService) getComputedTotalGridEnergy() (*storage.TotalDataPoint, error) {
	// Get total fed into grid
	fedTotal, err := s.storage.GetTotalHistory("grid_export_total")
	if err != nil {
		return nil, fmt.Errorf("failed to get grid_export_total: %w", err)
	}
	if fedTotal == nil {
		return nil, fmt.Errorf("no data found for grid_export_total")
	}

	// Get total imported from grid
	importTotal, err := s.storage.GetTotalHistory("grid_import_total")
	if err != nil {
		return nil, fmt.Errorf("failed to get grid_import_total: %w", err)
	}
	if importTotal == nil {
		return nil, fmt.Errorf("no data found for grid_import_total")
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
	if err := validateDateRange(start, end); err != nil {
		return nil, err
	}

	stored, err := s.storage.GetMonthlyHistory(monthlyKey, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored monthly history: %w", err)
	}

	startExpanded, endExpanded := expandToFullMonths(start, end)

	dailyHistory, err := s.storage.GetDailyHistory(dailyKey, startExpanded, endExpanded)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily history: %w", err)
	}

	calculatedMap := aggregateDailyToMonthly(dailyHistory)
	storedMap := buildMonthlyDataPointMap(stored)

	mergeMonthlyMaps(calculatedMap, storedMap)

	return s.finalizeMonthlyResult(calculatedMap, monthlyKey)
}

// getComputedMonthlyGridEnergy returns monthly history for the net grid energy register.
// Computes: grid_export_monthly - grid_import_monthly
func (s *ReadService) getComputedMonthlyGridEnergy(start, end time.Time) ([]*storage.MonthlyDataPoint, error) {
	if err := validateDateRange(start, end); err != nil {
		return nil, err
	}

	stored, err := s.storage.GetMonthlyHistory("grid_energy_monthly", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored grid_energy_monthly: %w", err)
	}

	startExpanded, endExpanded := expandToFullMonths(start, end)

	fedMonthly, err := s.getComputedMonthlyEnergy("grid_export_daily", startExpanded, endExpanded, "grid_export_monthly")
	if err != nil {
		return nil, fmt.Errorf("failed to get fed monthly history: %w", err)
	}

	importMonthly, err := s.getComputedMonthlyEnergy("grid_import_daily", startExpanded, endExpanded, "grid_import_monthly")
	if err != nil {
		return nil, fmt.Errorf("failed to get import monthly history: %w", err)
	}

	return computeNetMonthlyEnergy(stored, fedMonthly, importMonthly, "grid_energy_monthly")
}

// expandToFullMonths expands the date range to cover full months
func expandToFullMonths(start, end time.Time) (time.Time, time.Time) {
	startExpanded := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	endExpanded := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location()).AddDate(0, 1, 0)
	return startExpanded, endExpanded
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
		// Sum the already-scaled daily values (dp.Value contains the scaled value from daily_values)
		// For computed registers with Scale=1, both Value and RawValue should equal this sum
		calculatedMap[month].Value += dp.Value
		calculatedMap[month].RawValue += dp.Value
	}
	return calculatedMap
}

// mergeMonthlyMaps merges stored and calculated monthly data
func mergeMonthlyMaps(calculatedMap map[string]*storage.MonthlyDataPoint, storedMap map[string]*storage.MonthlyDataPoint) {
	currentMonth := time.Now().Format(solis.MonthFormat)
	for month, storedDp := range storedMap {
		// Only use stored data for past months (not current month)
		if month != currentMonth {
			calculatedMap[month] = storedDp
		}
		// For current month, we keep the calculated value (which will be backfilled)
	}
}

// finalizeMonthlyResult converts map to sorted slice and stores results
func (s *ReadService) finalizeMonthlyResult(calculatedMap map[string]*storage.MonthlyDataPoint, monthlyKey string) ([]*storage.MonthlyDataPoint, error) {
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

// computeNetMonthlyEnergy computes net monthly energy from fed and import data
func computeNetMonthlyEnergy(
	stored []*storage.MonthlyDataPoint,
	fed []*storage.MonthlyDataPoint,
	imp []*storage.MonthlyDataPoint,
	registerKey string,
) ([]*storage.MonthlyDataPoint, error) {
	fedMap := buildMonthlyDataPointMap(fed)
	importMap := buildMonthlyDataPointMap(imp)
	storedMap := buildMonthlyDataPointMap(stored)

	allMonths := collectUniqueMonths(fed, imp, stored)
	currentMonth := time.Now().Format(solis.MonthFormat)

	var result []*storage.MonthlyDataPoint
	for month := range allMonths {
		if storedDp, exists := storedMap[month]; exists && month != currentMonth {
			result = append(result, storedDp)
			continue
		}

		netDp := computeNetMonthlyDataPoint(month, fedMap[month], importMap[month])
		if netDp != nil {
			result = append(result, netDp)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Month < result[j].Month
	})

	return result, nil
}

// buildMonthlyDataPointMap creates a map from MonthlyDataPoint slice
func buildMonthlyDataPointMap(dps []*storage.MonthlyDataPoint) map[string]*storage.MonthlyDataPoint {
	m := make(map[string]*storage.MonthlyDataPoint)
	for _, dp := range dps {
		m[dp.Month] = dp
	}
	return m
}

// collectUniqueMonths collects all unique months from multiple MonthlyDataPoint slices
func collectUniqueMonths(slices ...[]*storage.MonthlyDataPoint) map[string]bool {
	allMonths := make(map[string]bool)
	for _, slice := range slices {
		for _, dp := range slice {
			allMonths[dp.Month] = true
		}
	}
	return allMonths
}

// computeNetMonthlyDataPoint computes net value from fed and import data points
func computeNetMonthlyDataPoint(month string, fedDp, importDp *storage.MonthlyDataPoint) *storage.MonthlyDataPoint {
	var netValue, netRawValue float64

	if fedDp != nil && importDp != nil {
		netValue = fedDp.Value - importDp.Value
		netRawValue = fedDp.RawValue - importDp.RawValue
	} else if fedDp != nil {
		netValue = fedDp.Value
		netRawValue = fedDp.RawValue
	} else if importDp != nil {
		netValue = -importDp.Value
		netRawValue = -importDp.RawValue
	} else {
		return nil
	}

	return &storage.MonthlyDataPoint{
		Month:    month,
		Value:    netValue,
		RawValue: netRawValue,
	}
}

// getComputedYearlyEnergy returns yearly history for computed registers by aggregating daily values.
func (s *ReadService) getComputedYearlyEnergy(dailyKey string, start, end time.Time, yearlyKey string) ([]*storage.YearlyDataPoint, error) {
	if err := validateDateRange(start, end); err != nil {
		return nil, err
	}

	stored, err := s.storage.GetYearlyHistory(yearlyKey, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored yearly history: %w", err)
	}

	startExpanded, endExpanded := expandToFullYears(start, end)

	dailyHistory, err := s.storage.GetDailyHistory(dailyKey, startExpanded, endExpanded)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily history: %w", err)
	}

	calculatedMap := aggregateDailyToYearly(dailyHistory)
	storedMap := buildYearlyDataPointMap(stored)

	mergeYearlyMaps(calculatedMap, storedMap)

	return s.finalizeYearlyResult(calculatedMap, yearlyKey)
}

// aggregateDailyToYearly aggregates daily data points into yearly data points
func aggregateDailyToYearly(dailyHistory []*storage.DailyDataPoint) map[string]*storage.YearlyDataPoint {
	calculatedMap := make(map[string]*storage.YearlyDataPoint)
	for _, dp := range dailyHistory {
		year := dp.Date[:4]
		if _, exists := calculatedMap[year]; !exists {
			calculatedMap[year] = &storage.YearlyDataPoint{
				Year:     year,
				Value:    0,
				RawValue: 0,
			}
		}
		calculatedMap[year].Value += dp.Value
		calculatedMap[year].RawValue += dp.Value
	}
	return calculatedMap
}

// mergeYearlyMaps merges stored and calculated yearly data
func mergeYearlyMaps(calculatedMap map[string]*storage.YearlyDataPoint, storedMap map[string]*storage.YearlyDataPoint) {
	currentYear := time.Now().Format(solis.YearFormat)
	for year, storedDp := range storedMap {
		if year != currentYear {
			calculatedMap[year] = storedDp
		}
	}
}

// finalizeYearlyResult converts map to sorted slice and stores results
func (s *ReadService) finalizeYearlyResult(calculatedMap map[string]*storage.YearlyDataPoint, yearlyKey string) ([]*storage.YearlyDataPoint, error) {
	result := make([]*storage.YearlyDataPoint, 0, len(calculatedMap))
	for _, dp := range calculatedMap {
		result = append(result, dp)
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
// Computes: grid_export_yearly - grid_import_yearly
func (s *ReadService) getComputedYearlyGridEnergy(start, end time.Time) ([]*storage.YearlyDataPoint, error) {
	if err := validateDateRange(start, end); err != nil {
		return nil, err
	}

	stored, err := s.storage.GetYearlyHistory("grid_energy_yearly", start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored grid_energy_yearly: %w", err)
	}

	startExpanded, endExpanded := expandToFullYears(start, end)

	fedYearly, err := s.getComputedYearlyEnergy("grid_export_daily", startExpanded, endExpanded, "grid_export_yearly")
	if err != nil {
		return nil, fmt.Errorf("failed to get fed yearly history: %w", err)
	}

	importYearly, err := s.getComputedYearlyEnergy("grid_import_daily", startExpanded, endExpanded, "grid_import_yearly")
	if err != nil {
		return nil, fmt.Errorf("failed to get import yearly history: %w", err)
	}

	return computeNetYearlyEnergy(stored, fedYearly, importYearly, "grid_energy_yearly")
}

// validateDateRange validates that end date is not before start date
func validateDateRange(start, end time.Time) error {
	if end.Before(start) {
		return fmt.Errorf("end date (%s) cannot be before start date (%s)", end.Format(solis.DateFormat), start.Format(solis.DateFormat))
	}
	return nil
}

// expandToFullYears expands the date range to cover full years
func expandToFullYears(start, end time.Time) (time.Time, time.Time) {
	startExpanded := time.Date(start.Year(), time.January, 1, 0, 0, 0, 0, start.Location())
	endExpanded := time.Date(end.Year(), time.January, 1, 0, 0, 0, 0, end.Location()).AddDate(1, 0, 0)
	return startExpanded, endExpanded
}

// computeNetYearlyEnergy computes net yearly energy from fed and import data
func computeNetYearlyEnergy(
	stored []*storage.YearlyDataPoint,
	fed []*storage.YearlyDataPoint,
	imp []*storage.YearlyDataPoint,
	registerKey string,
) ([]*storage.YearlyDataPoint, error) {
	fedMap := buildYearlyDataPointMap(fed)
	importMap := buildYearlyDataPointMap(imp)
	storedMap := buildYearlyDataPointMap(stored)

	allYears := collectUniqueYears(fed, imp, stored)
	currentYear := time.Now().Format(solis.YearFormat)

	var result []*storage.YearlyDataPoint
	for year := range allYears {
		if storedDp, exists := storedMap[year]; exists && year != currentYear {
			result = append(result, storedDp)
			continue
		}

		netDp := computeNetYearlyDataPoint(year, fedMap[year], importMap[year])
		if netDp != nil {
			result = append(result, netDp)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Year < result[j].Year
	})

	return result, nil
}

// buildYearlyDataPointMap creates a map from YearlyDataPoint slice
func buildYearlyDataPointMap(dps []*storage.YearlyDataPoint) map[string]*storage.YearlyDataPoint {
	m := make(map[string]*storage.YearlyDataPoint)
	for _, dp := range dps {
		m[dp.Year] = dp
	}
	return m
}

// collectUniqueYears collects all unique years from multiple YearlyDataPoint slices
func collectUniqueYears(slices ...[]*storage.YearlyDataPoint) map[string]bool {
	allYears := make(map[string]bool)
	for _, slice := range slices {
		for _, dp := range slice {
			allYears[dp.Year] = true
		}
	}
	return allYears
}

// computeNetYearlyDataPoint computes net value from fed and import data points
func computeNetYearlyDataPoint(year string, fedDp, importDp *storage.YearlyDataPoint) *storage.YearlyDataPoint {
	var netValue, netRawValue float64

	if fedDp != nil && importDp != nil {
		netValue = fedDp.Value - importDp.Value
		netRawValue = fedDp.RawValue - importDp.RawValue
	} else if fedDp != nil {
		netValue = fedDp.Value
		netRawValue = fedDp.RawValue
	} else if importDp != nil {
		netValue = -importDp.Value
		netRawValue = -importDp.RawValue
	} else {
		return nil
	}

	return &storage.YearlyDataPoint{
		Year:     year,
		Value:    netValue,
		RawValue: netRawValue,
	}
}
