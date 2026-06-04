// Package service provides business logic orchestration for the Solis monitor application.
// It coordinates between the Modbus client, poller, storage, and HTTP handlers.
package service

import (
	"fmt"
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
	}

	if s.poller != nil {
		status["poller_running"] = fmt.Sprintf("%v", s.poller.IsRunning())
	}

	if s.poller != nil {
		if info := s.poller.GetLastPollInfo(); info != nil {
			status["last_poll"] = info.Timestamp.Format(time.RFC3339)
			status["poll_duration_ms"] = fmt.Sprintf("%d", info.DurationMs)
		}
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

	return s.storage.GetTotalHistory(key)
}
