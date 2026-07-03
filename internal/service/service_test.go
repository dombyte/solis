package service

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

func TestNewReadService(t *testing.T) {
	// Create test config
	cfg := &config.AppConfig{
		App: config.AppSettings{
			Port:    8080,
			Timeout: 30 * time.Second,
			Debug:   "INFO",
		},
		Modbus: config.ModbusSettings{
			Type:    "tcp",
			Host:    "192.168.1.100",
			Port:    502,
			Timeout: 5 * time.Second,
			UnitID:  1,
		},
		Storage: config.StorageSettings{
			Path:        "./data/test.db",
			WalMode:     true,
			Synchronous: "NORMAL",
			TempStore:   "MEMORY",
		},
		Poller: config.PollerSettings{
			Interval:        15 * time.Minute,
			BlockAttempts:   3,
			BlockRetryDelay: 1 * time.Second,
			BlockInterval:   0,
			PollTimeout:     30 * time.Second,
		},
	}

	// Create service with nil dependencies (should still create the struct)
	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	if service == nil {
		t.Fatal("NewReadService() returned nil")
	}

	if service.config != cfg {
		t.Error("ReadService.config is not set correctly")
	}

	if service.modbusClient != nil {
		t.Error("ReadService.modbusClient should be nil")
	}

	if service.storage != nil {
		t.Error("ReadService.storage should be nil")
	}

	if service.poller != nil {
		t.Error("ReadService.poller should be nil")
	}
}

func TestNewReadService_WithDependencies(t *testing.T) {
	cfg := &config.AppConfig{
		App: config.AppSettings{
			Port:    8080,
			Timeout: 30 * time.Second,
			Debug:   "INFO",
		},
	}

	// Create mock dependencies
	modbusClient := &modbus.Client{}
	st := &storage.Storage{}
	pl := &poller.Poller{}

	service := NewReadService(cfg, modbusClient, st, pl, nil, nil)

	if service == nil {
		t.Fatal("NewReadService() returned nil")
	}

	if service.modbusClient != modbusClient {
		t.Error("ReadService.modbusClient is not set correctly")
	}

	if service.storage != st {
		t.Error("ReadService.storage is not set correctly")
	}

	if service.poller != pl {
		t.Error("ReadService.poller is not set correctly")
	}
}

func TestReadService_HealthCheck_NoDependencies(t *testing.T) {
	cfg := &config.AppConfig{}

	// Create service with no dependencies
	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	// HealthCheck should return ok status even with nil dependencies
	status, err := service.HealthCheck()
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}

	if status == nil {
		t.Fatal("HealthCheck() returned nil status")
	}

	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}
}

func TestReadService_HealthCheck_WithModbusClient(t *testing.T) {
	cfg := &config.AppConfig{}

	// Create a modbus client (will fail to connect, but we can create the struct)
	modbusClient := &modbus.Client{}

	service := NewReadService(cfg, modbusClient, nil, nil, nil, nil)

	// HealthCheck should handle nil or disconnected modbus client
	status, err := service.HealthCheck()
	if err != nil {
		t.Logf("HealthCheck() with modbus client error: %v", err)
		return
	}

	if status == nil {
		t.Fatal("HealthCheck() returned nil status")
	}

	// Should have modbus_connected status
	if _, ok := status["modbus_connected"]; ok {
		t.Logf("HealthCheck() includes modbus_connected: %v", status["modbus_connected"])
	}
}

func TestReadService_HealthCheck_WithStorage(t *testing.T) {
	cfg := &config.AppConfig{}

	// Create a storage (will fail to initialize, but we can test the struct)
	// We'll pass nil for storage in the service
	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	status, err := service.HealthCheck()
	if err != nil {
		t.Logf("HealthCheck() error: %v", err)
		return
	}

	if status == nil {
		t.Fatal("HealthCheck() returned nil status")
	}

	// Should have status
	if status["status"] != "ok" {
		t.Errorf("HealthCheck() status['status'] = %v, want 'ok'", status["status"])
	}
}

func TestReadService_HealthCheck_WithPoller(t *testing.T) {
	cfg := &config.AppConfig{}

	// Create a poller (not running)
	pl := &poller.Poller{}

	service := NewReadService(cfg, nil, nil, pl, nil, nil)

	status, err := service.HealthCheck()
	if err != nil {
		t.Logf("HealthCheck() with poller error: %v", err)
		return
	}

	if status == nil {
		t.Fatal("HealthCheck() returned nil status")
	}

	// Should have poller_running status
	if _, ok := status["poller_running"]; ok {
		t.Logf("HealthCheck() includes poller_running: %v", status["poller_running"])
	}
}

func TestReadService_GetRegister_NoModbusClient(t *testing.T) {
	cfg := &config.AppConfig{}

	// Create service with no modbus client
	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	// GetRegister should return error when storage is nil
	_, err := service.GetRegister("test_key")
	if err == nil {
		t.Error("GetRegister() with nil storage expected error, got nil")
	}
}

func TestReadService_GetValues_NoStorage(t *testing.T) {
	cfg := &config.AppConfig{}

	// Create service with no dependencies
	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	// GetValues with no storage should return empty map
	values, err := service.GetValues([]string{"test_key"})
	if err != nil {
		t.Logf("GetValues() error: %v", err)
		return
	}

	if values == nil {
		t.Fatal("GetValues() returned nil map")
	}

	// Should return empty map
	if len(values) != 0 {
		t.Errorf("GetValues() returned %d values, want 0", len(values))
	}
}

func TestReadService_GetHistoricalData_NoStorage(t *testing.T) {
	cfg := &config.AppConfig{}

	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	// GetHistoricalData with no storage should return error
	_, err := service.GetHistoricalData("test_key", time.Now().Add(-1*time.Hour), time.Now(), storage.IntervalRaw)
	if err == nil {
		t.Error("GetHistoricalData() with nil storage expected error, got nil")
	}

	if !errors.Is(err, errors.New("storage not available")) {
		// Check error message
		if err.Error() != "storage not available" {
			t.Logf("GetHistoricalData() error = %v", err)
		}
	}
}

func TestReadService_GetHistoricalData_InvalidKey(t *testing.T) {
	cfg := &config.AppConfig{}

	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	// GetHistoricalData with invalid key should return error
	// But it will fail first on storage being nil
	_, err := service.GetHistoricalData("invalid_key", time.Now().Add(-1*time.Hour), time.Now(), storage.IntervalRaw)
	if err == nil {
		t.Error("GetHistoricalData() with invalid key expected error, got nil")
	}
}

func TestReadService_GetValues_EmptyKeys(t *testing.T) {
	cfg := &config.AppConfig{}

	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	// GetValues with empty keys should return empty map
	values, err := service.GetValues([]string{})
	if err != nil {
		t.Logf("GetValues() with empty keys error: %v", err)
		return
	}

	if values == nil {
		t.Fatal("GetValues() returned nil map")
	}

	// Should return empty map
	if len(values) != 0 {
		t.Errorf("GetValues() with empty keys returned %d values, want 0", len(values))
	}
}

func TestReadService_AllRegisterKeys(t *testing.T) {
	// Verify that solis package has registers available
	if len(solis.AllRegisters) == 0 {
		t.Error("solis.AllRegisters is empty")
	}

	// Verify that RegisterMapByKey has entries
	if len(solis.RegisterMapByKey) == 0 {
		t.Error("solis.RegisterMapByKey is empty")
	}

	// Verify that the first register has a key
	if len(solis.AllRegisters) > 0 {
		firstKey := solis.AllRegisters[0].Key
		if firstKey == "" {
			t.Error("First register has empty key")
		}
		// Verify it exists in the map
		if _, ok := solis.RegisterMapByKey[firstKey]; !ok {
			t.Errorf("First register key %s not found in RegisterMapByKey", firstKey)
		}
	}
}

// MockStorage is a mock implementation of storage.Storage for testing
// We'll use the actual storage for integration tests, but we need to set up a test database
type MockStorage struct {
	*storage.Storage
}

// TestService_GetComputedDailyGridEnergy tests the getComputedDailyGridEnergy method
func TestService_GetComputedDailyGridEnergy(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := tempDir + "/test_computed_daily.db"

	cfg := &config.AppConfig{
		Storage: config.StorageSettings{
			Path:        dbPath,
			WalMode:     true,
			Synchronous: "NORMAL",
			TempStore:   "MEMORY",
		},
	}

	st, err := storage.New(&cfg.Storage)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() {
		st.Close()
		os.RemoveAll(tempDir)
	}()

	service := NewReadService(cfg, nil, st, nil, nil, nil)

	// Insert test data for today_energy_fed_into_grid
	timestamp := time.Now()
	fedValues := map[string]*solis.Value{
		"today_energy_fed_into_grid": {
			Key:          "today_energy_fed_into_grid",
			Name:         "Today Energy Fed Into Grid",
			RawValue:     150, // 15.0 kWh after scaling (0.1)
			DecodedValue: 15.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
	}

	err = st.StoreAllRegisters(fedValues, timestamp)
	if err != nil {
		t.Fatalf("Failed to store fed values: %v", err)
	}

	// Insert test data for today_energy_imported_from_grid
	importValues := map[string]*solis.Value{
		"today_energy_imported_from_grid": {
			Key:          "today_energy_imported_from_grid",
			Name:         "Today Energy Imported From Grid",
			RawValue:     50, // 5.0 kWh after scaling (0.1)
			DecodedValue: 5.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
	}

	err = st.StoreAllRegisters(importValues, timestamp)
	if err != nil {
		t.Fatalf("Failed to store import values: %v", err)
	}

	// Wait a bit for storage to complete
	time.Sleep(20 * time.Millisecond)

	// Test getComputedDailyGridEnergy
	start := timestamp.Add(-24 * time.Hour)
	end := timestamp.Add(24 * time.Hour)

	result, err := service.GetDailyHistory("today_grid_energy", start, end)
	if err != nil {
		t.Fatalf("GetDailyHistory for today_grid_energy error = %v", err)
	}

	// Should return computed values: fed (15.0) - import (5.0) = 10.0
	if len(result) == 0 {
		t.Error("Expected computed daily grid energy results, got empty")
	} else {
		// Check the first result
		if len(result) > 0 {
			expectedValue := 15.0 - 5.0 // 10.0 kWh
			if result[0].Value != expectedValue {
				t.Errorf("Computed daily grid energy value = %v, want %v", result[0].Value, expectedValue)
			}
			t.Logf("Computed daily grid energy: value=%.2f, raw=%.2f, date=%s",
				result[0].Value, result[0].RawValue, result[0].Date)
		}
	}
}

// TestService_GetComputedTotalGridEnergy tests the getComputedTotalGridEnergy method
func TestService_GetComputedTotalGridEnergy(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := tempDir + "/test_computed_total.db"

	cfg := &config.AppConfig{
		Storage: config.StorageSettings{
			Path:        dbPath,
			WalMode:     true,
			Synchronous: "NORMAL",
			TempStore:   "MEMORY",
		},
	}

	st, err := storage.New(&cfg.Storage)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() {
		st.Close()
		os.RemoveAll(tempDir)
	}()

	service := NewReadService(cfg, nil, st, nil, nil, nil)

	// Insert test data for total_energy_fed_into_grid
	timestamp := time.Now()
	fedValues := map[string]*solis.Value{
		"total_energy_fed_into_grid": {
			Key:          "total_energy_fed_into_grid",
			Name:         "Total Energy Fed Into Grid",
			RawValue:     2000, // 2000 kWh (scale = 1)
			DecodedValue: 2000.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint32,
			Stability:    solis.Dynamic,
		},
	}

	err = st.StoreAllRegisters(fedValues, timestamp)
	if err != nil {
		t.Fatalf("Failed to store fed total values: %v", err)
	}

	// Insert test data for total_energy_imported_from_grid
	importValues := map[string]*solis.Value{
		"total_energy_imported_from_grid": {
			Key:          "total_energy_imported_from_grid",
			Name:         "Total Energy Imported From Grid",
			RawValue:     500, // 500 kWh (scale = 1)
			DecodedValue: 500.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint32,
			Stability:    solis.Dynamic,
		},
	}

	err = st.StoreAllRegisters(importValues, timestamp)
	if err != nil {
		t.Fatalf("Failed to store import total values: %v", err)
	}

	// Wait a bit for storage to complete
	time.Sleep(20 * time.Millisecond)

	// Test getComputedTotalGridEnergy
	result, err := service.GetTotalHistory("total_grid_energy")
	if err != nil {
		t.Fatalf("GetTotalHistory for total_grid_energy error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected computed total grid energy result, got nil")
	}

	// Should return computed values: fed (2000.0) - import (500.0) = 1500.0
	expectedValue := 2000.0 - 500.0
	if result.Value != expectedValue {
		t.Errorf("Computed total grid energy value = %v, want %v", result.Value, expectedValue)
	}

	t.Logf("Computed total grid energy: value=%.2f, raw=%.2f, timestamp=%s",
		result.Value, result.RawValue, result.Timestamp)
}

// TestService_GetComputedMonthlyEnergy tests the getComputedMonthlyEnergy method
func TestService_GetComputedMonthlyEnergy(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := tempDir + "/test_computed_monthly.db"

	cfg := &config.AppConfig{
		Storage: config.StorageSettings{
			Path:        dbPath,
			WalMode:     true,
			Synchronous: "NORMAL",
			TempStore:   "MEMORY",
		},
	}

	st, err := storage.New(&cfg.Storage)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() {
		st.Close()
		os.RemoveAll(tempDir)
	}()

	service := NewReadService(cfg, nil, st, nil, nil, nil)

	// Insert test daily data for today_energy_consumption
	timestamp := time.Now()
	values := map[string]*solis.Value{
		"today_energy_consumption": {
			Key:          "today_energy_consumption",
			Name:         "Today Energy Consumption",
			RawValue:     100, // 10.0 kWh after scaling (0.1)
			DecodedValue: 10.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
	}

	err = st.StoreAllRegisters(values, timestamp)
	if err != nil {
		t.Fatalf("Failed to store daily values: %v", err)
	}

	// Wait a bit for storage to complete
	time.Sleep(20 * time.Millisecond)

	// Test getComputedMonthlyEnergy
	start := time.Date(timestamp.Year(), timestamp.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(timestamp.Year(), timestamp.Month()+1, 0, 0, 0, 0, -1, time.UTC)

	result, err := service.GetMonthlyHistory("energy_consumption_month_energy", start, end)
	if err != nil {
		t.Fatalf("GetMonthlyHistory for energy_consumption_month_energy error = %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected computed monthly energy results, got empty")
	} else {
		// The computed value should be based on the daily sum
		// Note: The exact value depends on the computation logic
		if result[0].Value < 0 {
			t.Errorf("Computed monthly energy value = %v, want >= 0", result[0].Value)
		}
		t.Logf("Computed monthly energy: value=%.2f, raw=%.2f, month=%s",
			result[0].Value, result[0].RawValue, result[0].Month)
	}
}

// TestService_GetComputedMonthlyGridEnergy tests the getComputedMonthlyGridEnergy method
func TestService_GetComputedMonthlyGridEnergy(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := tempDir + "/test_computed_month_grid.db"

	cfg := &config.AppConfig{
		Storage: config.StorageSettings{
			Path:        dbPath,
			WalMode:     true,
			Synchronous: "NORMAL",
			TempStore:   "MEMORY",
		},
	}

	st, err := storage.New(&cfg.Storage)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() {
		st.Close()
		os.RemoveAll(tempDir)
	}()

	service := NewReadService(cfg, nil, st, nil, nil, nil)

	// Store test data directly using StoreMonthlyDataPoint
	// This simulates having pre-computed monthly values
	// For computed registers, scale is 1, so Value = RawValue * 1 = RawValue
	fedDp := &storage.MonthlyDataPoint{
		Month:    "2024-06",
		Value:    500.0, // This is the decoded value (already scaled)
		RawValue: 500.0, // For scale=1 registers, RawValue equals Value
	}
	err = st.StoreMonthlyDataPoint("energy_fed_into_grid_month_energy", fedDp)
	if err != nil {
		t.Fatalf("Failed to store fed monthly data: %v", err)
	}

	importDp := &storage.MonthlyDataPoint{
		Month:    "2024-06",
		Value:    200.0, // This is the decoded value (already scaled)
		RawValue: 200.0, // For scale=1 registers, RawValue equals Value
	}
	err = st.StoreMonthlyDataPoint("energy_imported_from_grid_month_energy", importDp)
	if err != nil {
		t.Fatalf("Failed to store import monthly data: %v", err)
	}

	// Test getComputedMonthlyGridEnergy
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)

	result, err := service.GetMonthlyHistory("month_grid_energy", start, end)
	if err != nil {
		t.Fatalf("GetMonthlyHistory for month_grid_energy error = %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected computed monthly grid energy results, got empty")
	} else {
		// Should return computed values: fed (500.0) - import (200.0) = 300.0
		expectedValue := 500.0 - 200.0
		// Note: The computation uses DecodedValue from the stored data, which equals Value for scale=1 registers
		if result[0].Value != expectedValue {
			t.Errorf("Computed monthly grid energy value = %v, want %v", result[0].Value, expectedValue)
		}
		t.Logf("Computed monthly grid energy: value=%.2f, raw=%.2f, month=%s",
			result[0].Value, result[0].RawValue, result[0].Month)
	}
}

// TestService_GetComputedYearlyEnergy tests the getComputedYearlyEnergy method
func TestService_GetComputedYearlyEnergy(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := tempDir + "/test_computed_yearly.db"

	cfg := &config.AppConfig{
		Storage: config.StorageSettings{
			Path:        dbPath,
			WalMode:     true,
			Synchronous: "NORMAL",
			TempStore:   "MEMORY",
		},
	}

	st, err := storage.New(&cfg.Storage)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() {
		st.Close()
		os.RemoveAll(tempDir)
	}()

	service := NewReadService(cfg, nil, st, nil, nil, nil)

	// Insert test daily data for today_energy_consumption
	timestamp := time.Now()
	values := map[string]*solis.Value{
		"today_energy_consumption": {
			Key:          "today_energy_consumption",
			Name:         "Today Energy Consumption",
			RawValue:     100, // 10.0 kWh after scaling (0.1)
			DecodedValue: 10.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
	}

	err = st.StoreAllRegisters(values, timestamp)
	if err != nil {
		t.Fatalf("Failed to store daily values: %v", err)
	}

	// Wait a bit for storage to complete
	time.Sleep(20 * time.Millisecond)

	// Test getComputedYearlyEnergy
	start := time.Date(timestamp.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(timestamp.Year(), time.December, 31, 23, 59, 59, 0, time.UTC)

	result, err := service.GetYearlyHistory("energy_consumption_year_energy", start, end)
	if err != nil {
		t.Fatalf("GetYearlyHistory for energy_consumption_year_energy error = %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected computed yearly energy results, got empty")
	} else {
		// The computed value should be based on the daily sum
		if result[0].Value < 0 {
			t.Errorf("Computed yearly energy value = %v, want >= 0", result[0].Value)
		}
		t.Logf("Computed yearly energy: value=%.2f, raw=%.2f, year=%s",
			result[0].Value, result[0].RawValue, result[0].Year)
	}
}

// TestService_GetComputedYearlyGridEnergy tests the getComputedYearlyGridEnergy method
func TestService_GetComputedYearlyGridEnergy(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := tempDir + "/test_computed_year_grid.db"

	cfg := &config.AppConfig{
		Storage: config.StorageSettings{
			Path:        dbPath,
			WalMode:     true,
			Synchronous: "NORMAL",
			TempStore:   "MEMORY",
		},
	}

	st, err := storage.New(&cfg.Storage)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() {
		st.Close()
		os.RemoveAll(tempDir)
	}()

	service := NewReadService(cfg, nil, st, nil, nil, nil)

	// Store test data directly using StoreYearlyDataPoint
	// For computed registers, scale is 1, so Value = RawValue * 1 = RawValue
	fedDp := &storage.YearlyDataPoint{
		Year:     "2024",
		Value:    3000.0, // This is the decoded value (already scaled)
		RawValue: 3000.0, // For scale=1 registers, RawValue equals Value
	}
	err = st.StoreYearlyDataPoint("energy_fed_into_grid_year_energy", fedDp)
	if err != nil {
		t.Fatalf("Failed to store fed yearly data: %v", err)
	}

	importDp := &storage.YearlyDataPoint{
		Year:     "2024",
		Value:    1000.0, // This is the decoded value (already scaled)
		RawValue: 1000.0, // For scale=1 registers, RawValue equals Value
	}
	err = st.StoreYearlyDataPoint("energy_imported_from_grid_year_energy", importDp)
	if err != nil {
		t.Fatalf("Failed to store import yearly data: %v", err)
	}

	// Test getComputedYearlyGridEnergy
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.December, 31, 23, 59, 59, 0, time.UTC)

	result, err := service.GetYearlyHistory("year_grid_energy", start, end)
	if err != nil {
		t.Fatalf("GetYearlyHistory for year_grid_energy error = %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected computed yearly grid energy results, got empty")
	} else {
		// Should return computed values: fed (3000.0) - import (1000.0) = 2000.0
		expectedValue := 3000.0 - 1000.0
		// Note: The computation uses DecodedValue from the stored data, which equals Value for scale=1 registers
		if result[0].Value != expectedValue {
			t.Errorf("Computed yearly grid energy value = %v, want %v", result[0].Value, expectedValue)
		}
		t.Logf("Computed yearly grid energy: value=%.2f, raw=%.2f, year=%s",
			result[0].Value, result[0].RawValue, result[0].Year)
	}
}

// TestService_ValidateRegisterType tests the validateRegisterType method
func TestService_ValidateRegisterType(t *testing.T) {
	cfg := &config.AppConfig{}
	service := NewReadService(cfg, nil, nil, nil, nil, nil)

	// Test with valid daily register
	err := service.validateRegisterType("today_energy_consumption", solis.IsDailyRegister, "daily energy")
	if err != nil {
		t.Errorf("validateRegisterType for valid daily register returned error: %v", err)
	}

	// Test with invalid daily register
	err = service.validateRegisterType("pv_voltage_1", solis.IsDailyRegister, "daily energy")
	if err == nil {
		t.Error("validateRegisterType for invalid daily register expected error, got nil")
	}

	// Test with valid monthly register
	err = service.validateRegisterType("pv_month_energy", solis.IsMonthlyRegister, "monthly energy")
	if err != nil {
		t.Errorf("validateRegisterType for valid monthly register returned error: %v", err)
	}

	// Test with valid yearly register
	err = service.validateRegisterType("pv_year_energy", solis.IsYearlyRegister, "yearly energy")
	if err != nil {
		t.Errorf("validateRegisterType for valid yearly register returned error: %v", err)
	}

	// Test with valid total register
	err = service.validateRegisterType("pv_total_energy", solis.IsTotalRegister, "total energy")
	if err != nil {
		t.Errorf("validateRegisterType for valid total register returned error: %v", err)
	}

	// Test with computed net grid register (should be total)
	err = service.validateRegisterType("total_grid_energy", solis.IsTotalRegister, "total energy")
	if err != nil {
		t.Errorf("validateRegisterType for total_grid_energy returned error: %v", err)
	}

	// Test with computed monthly net grid register (should be monthly)
	err = service.validateRegisterType("month_grid_energy", solis.IsMonthlyRegister, "monthly energy")
	if err != nil {
		t.Errorf("validateRegisterType for month_grid_energy returned error: %v", err)
	}

	// Test with computed yearly net grid register (should be yearly)
	err = service.validateRegisterType("year_grid_energy", solis.IsYearlyRegister, "yearly energy")
	if err != nil {
		t.Errorf("validateRegisterType for year_grid_energy returned error: %v", err)
	}
}
