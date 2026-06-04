package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/solis"
)

func TestNew(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &config.StorageSettings{
		Path:        dbPath,
		WalMode:     true,
		Synchronous: "NORMAL",
		TempStore:   "MEMORY",
	}

	// Create storage
	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if st == nil {
		t.Fatal("New() returned nil")
	}

	// Check configuration
	if st.config != cfg {
		t.Error("Storage.config is not set correctly")
	}

	if st.path != dbPath {
		t.Errorf("Storage.path = %v, want %v", st.path, dbPath)
	}

	// Check database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}

	// Clean up
	st.Close()

	// Verify file still exists after close
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was deleted after Close()")
	}

	// Clean up the file
	os.Remove(dbPath)
}

func TestNew_WithDefaultSettings(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_default.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
		// Use default values for other fields
		WalMode:     false,
		Synchronous: "",
		TempStore:   "",
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with defaults error = %v", err)
	}

	if st == nil {
		t.Fatal("New() with defaults returned nil")
	}

	// Clean up
	st.Close()
	os.Remove(dbPath)
}

func TestStorage_Close(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_close.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Close the storage
	err = st.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Close again (should be safe)
	err = st.Close()
	if err != nil {
		t.Logf("Close() second call error: %v", err)
	}

	// Clean up
	os.Remove(dbPath)
}

func TestStorage_InitSchema(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_schema.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// The schema should have been initialized
	// We can verify by checking if the database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created during schema initialization")
	}

	// Clean up
	st.Close()
	os.Remove(dbPath)
}

func TestStorage_MultipleInstances(t *testing.T) {
	tempDir := t.TempDir()
	dbPath1 := filepath.Join(tempDir, "test1.db")
	dbPath2 := filepath.Join(tempDir, "test2.db")

	cfg1 := &config.StorageSettings{
		Path: dbPath1,
	}

	cfg2 := &config.StorageSettings{
		Path: dbPath2,
	}

	st1, err := New(cfg1)
	if err != nil {
		t.Fatalf("New() first instance error = %v", err)
	}

	st2, err := New(cfg2)
	if err != nil {
		t.Fatalf("New() second instance error = %v", err)
	}

	// They should be different instances
	if st1 == st2 {
		t.Error("Two storage instances are the same")
	}

	// Clean up
	st1.Close()
	st2.Close()
	os.Remove(dbPath1)
	os.Remove(dbPath2)
}

func TestStorage_StoreAllRegisters(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_store.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Create test data
	timestamp := time.Now()
	values := map[string]*solis.Value{
		"test_register_1": {
			Key:          "test_register_1",
			Name:         "Test Register 1",
			RawValue:     100,
			DecodedValue: 10.0,
			Unit:         "V",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
		"test_register_2": {
			Key:          "test_register_2",
			Name:         "Test Register 2",
			RawValue:     200,
			DecodedValue: 20.0,
			Unit:         "A",
			Timestamp:    timestamp,
			DataType:     solis.Int16,
			Stability:    solis.Dynamic,
		},
	}

	// Store the data
	err = st.StoreAllRegisters(values, timestamp)
	if err != nil {
		t.Errorf("StoreAllRegisters() error = %v", err)
	}

	// Clean up
	st.Close()
	os.Remove(dbPath)
}

func TestStorage_StoreAllRegisters_Empty(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_empty.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Store empty data (should not error)
	err = st.StoreAllRegisters(map[string]*solis.Value{}, time.Now())
	if err != nil {
		t.Errorf("StoreAllRegisters() with empty map error = %v", err)
	}

	// Clean up
	st.Close()
	os.Remove(dbPath)
}

func TestStorage_StoreAllRegisters_StableAndDynamic(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_all.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Create test data with both stable and dynamic registers
	timestamp := time.Now()
	values := map[string]*solis.Value{
		"stable_register": {
			Key:          "stable_register",
			Name:         "Stable Register",
			RawValue:     100,
			DecodedValue: 100,
			Unit:         "",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Stable,
		},
		"dynamic_register": {
			Key:          "dynamic_register",
			Name:         "Dynamic Register",
			RawValue:     200,
			DecodedValue: 20.0,
			Unit:         "V",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
	}

	// Store all registers
	err = st.StoreAllRegisters(values, timestamp)
	if err != nil {
		t.Errorf("StoreAllRegisters() error = %v", err)
	}

	// Clean up
	st.Close()
	os.Remove(dbPath)
}

func TestStorage_IntervalTypes(t *testing.T) {
	// Only IntervalRaw is supported now (aggregated intervals removed)
	if string(IntervalRaw) != "raw" {
		t.Errorf("IntervalRaw = %v, want 'raw'", string(IntervalRaw))
	}
}

func TestStorage_RegisterValue(t *testing.T) {
	// Test RegisterValue struct
	value := RegisterValue{
		Key:         "test_key",
		RawValue:    100,
		StringValue: "",
		Timestamp:   time.Now(),
	}

	if value.Key != "test_key" {
		t.Errorf("RegisterValue.Key = %v, want test_key", value.Key)
	}

	if value.RawValue != 100 {
		t.Errorf("RegisterValue.RawValue = %v, want 100", value.RawValue)
	}
}

func TestStorage_HistoryResult(t *testing.T) {
	// Test HistoryResult struct
	result := &HistoryResult{
		Key:      "test_key",
		Unit:     "V",
		Interval: IntervalRaw,
		Data:     []HistoryDataPoint{},
	}

	if result.Key != "test_key" {
		t.Errorf("HistoryResult.Key = %v, want test_key", result.Key)
	}

	if result.Unit != "V" {
		t.Errorf("HistoryResult.Unit = %v, want V", result.Unit)
	}

	if result.Interval != IntervalRaw {
		t.Errorf("HistoryResult.Interval = %v, want %v", result.Interval, IntervalRaw)
	}

	if result.Data == nil {
		t.Error("HistoryResult.Data is nil")
	}
}

func TestStorage_HistoryDataPoint(t *testing.T) {
	// Test HistoryDataPoint struct
	point := HistoryDataPoint{
		Timestamp: "2024-01-01T00:00:00Z",
		Value:     10.5,
	}

	if point.Timestamp != "2024-01-01T00:00:00Z" {
		t.Errorf("HistoryDataPoint.Timestamp = %v, want 2024-01-01T00:00:00Z", point.Timestamp)
	}

	if point.Value != 10.5 {
		t.Errorf("HistoryDataPoint.Value = %v, want 10.5", point.Value)
	}
}
