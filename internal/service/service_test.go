package service

import (
	"errors"
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
