// Package service provides business logic orchestration for the Solis monitor application.
// It coordinates between the Modbus client, poller, storage, and HTTP handlers.
package service

import (
	"fmt"
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

	// Handle computed net grid energy register - now uses storage directly
	// The aggregator computes and stores grid_energy_daily in the database
	if key == "grid_energy_daily" {
		return s.storage.GetDailyHistory(key, start, end)
	}

	return s.storage.GetDailyHistory(key, start, end)
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

	// All computed monthly registers are now handled by the aggregator
	// which stores them in the database. Simply retrieve from storage.
	// This includes:
	// - grid_energy_monthly (net value)
	// - energy_consumption_monthly (computed from daily)
	// - grid_export_monthly (computed from daily)
	// - grid_import_monthly (computed from daily)
	// - battery_discharge_monthly (computed from daily)
	// - battery_charge_monthly (computed from daily)
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

	// All computed yearly registers are now handled by the aggregator
	// which stores them in the database. Simply retrieve from storage.
	// This includes:
	// - grid_energy_yearly (net value)
	// - energy_consumption_yearly (computed from daily)
	// - grid_export_yearly (computed from daily)
	// - grid_import_yearly (computed from daily)
	// - battery_discharge_yearly (computed from daily)
	// - battery_charge_yearly (computed from daily)
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

	// All total registers including grid_energy_total are now handled by the aggregator
	// which stores them in the database. Simply retrieve from storage.
	return s.storage.GetTotalHistory(key)
}






