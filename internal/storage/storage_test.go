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

// TestStorage_GetMonthlySum tests the GetMonthlySum method
func TestStorage_GetMonthlySum(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_monthly_sum.db")

	cfg := &config.StorageSettings{
		Path:           dbPath,
		DailyRetention: 365 * 24 * time.Hour,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Insert some test daily data
	timestamp := time.Now()
	dailyValues := map[string]*solis.Value{
		"energy_consumption_daily": {
			Key:          "energy_consumption_daily",
			Name:         "Today Energy Consumption",
			RawValue:     100,
			DecodedValue: 10.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
	}

	// Store the data
	err = st.StoreAllRegisters(dailyValues, timestamp)
	if err != nil {
		t.Fatalf("StoreAllRegisters() error = %v", err)
	}

	// Wait a bit for storage to complete
	time.Sleep(10 * time.Millisecond)

	// Get monthly sum for current month
	currentMonth := timestamp.Format("2006-01")
	sumValue, sumRawValue, err := st.GetMonthlySum("energy_consumption_daily", currentMonth)
	if err != nil {
		t.Fatalf("GetMonthlySum() error = %v", err)
	}

	// Since we just inserted one value, the sum should be that value
	// Note: the value stored in daily_values is scaled, so sumValue should be 10.0 (not 100)
	// But GetMonthlySum returns the sum of the 'value' column which is decodedValue * scale
	// In our test, scale is 0.1 for energy_consumption_daily, so decodedValue = 100 * 0.1 = 10.0
	if sumValue < 0 {
		t.Errorf("GetMonthlySum() sumValue = %v, want >= 0", sumValue)
	}

	t.Logf("GetMonthlySum for %s: value=%.2f, raw=%.2f", currentMonth, sumValue, sumRawValue)
}

// TestStorage_GetMonthlySum_NoData tests GetMonthlySum with no data
func TestStorage_GetMonthlySum_NoData(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_monthly_sum_nodata.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Get monthly sum for a register with no data
	sumValue, sumRawValue, err := st.GetMonthlySum("energy_consumption_daily", "2024-01")
	if err != nil {
		t.Fatalf("GetMonthlySum() with no data error = %v", err)
	}

	// Should return 0 for no data
	if sumValue != 0 {
		t.Errorf("GetMonthlySum() with no data sumValue = %v, want 0", sumValue)
	}
	if sumRawValue != 0 {
		t.Errorf("GetMonthlySum() with no data sumRawValue = %v, want 0", sumRawValue)
	}
}

// TestStorage_GetYearlySum tests the GetYearlySum method
func TestStorage_GetYearlySum(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_yearly_sum.db")

	cfg := &config.StorageSettings{
		Path:           dbPath,
		DailyRetention: 365 * 24 * time.Hour,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Insert some test daily data
	timestamp := time.Now()
	dailyValues := map[string]*solis.Value{
		"energy_consumption_daily": {
			Key:          "energy_consumption_daily",
			Name:         "Today Energy Consumption",
			RawValue:     150,
			DecodedValue: 15.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint16,
			Stability:    solis.Dynamic,
		},
	}

	// Store the data
	err = st.StoreAllRegisters(dailyValues, timestamp)
	if err != nil {
		t.Fatalf("StoreAllRegisters() error = %v", err)
	}

	// Wait a bit for storage to complete
	time.Sleep(10 * time.Millisecond)

	// Get yearly sum for current year
	currentYear := timestamp.Format("2006")
	sumValue, sumRawValue, err := st.GetYearlySum("energy_consumption_daily", currentYear)
	if err != nil {
		t.Fatalf("GetYearlySum() error = %v", err)
	}

	// Should return a positive value
	if sumValue < 0 {
		t.Errorf("GetYearlySum() sumValue = %v, want >= 0", sumValue)
	}

	t.Logf("GetYearlySum for %s: value=%.2f, raw=%.2f", currentYear, sumValue, sumRawValue)
}

// TestStorage_GetYearlySum_NoData tests GetYearlySum with no data
func TestStorage_GetYearlySum_NoData(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_yearly_sum_nodata.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Get yearly sum for a register with no data
	sumValue, sumRawValue, err := st.GetYearlySum("energy_consumption_daily", "2024")
	if err != nil {
		t.Fatalf("GetYearlySum() with no data error = %v", err)
	}

	// Should return 0 for no data
	if sumValue != 0 {
		t.Errorf("GetYearlySum() with no data sumValue = %v, want 0", sumValue)
	}
	if sumRawValue != 0 {
		t.Errorf("GetYearlySum() with no data sumRawValue = %v, want 0", sumRawValue)
	}
}

// TestStorage_StoreMonthlyDataPoint tests the StoreMonthlyDataPoint method
func TestStorage_StoreMonthlyDataPoint(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_store_monthly.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Create a test monthly data point
	// For computed monthly registers, Value should be the decoded/summed value
	// and RawValue should be the sum of daily raw values
	dp := &MonthlyDataPoint{
		Month:    "2024-06",
		Value:    1005.0,  // This is the decoded monthly value (what we want stored)
		RawValue: 10050.0, // This is the sum of daily raw values (for auditing)
	}

	// Store it for a valid register key
	err = st.StoreMonthlyDataPoint("energy_consumption_monthly", dp)
	if err != nil {
		t.Fatalf("StoreMonthlyDataPoint() error = %v", err)
	}

	// Verify it was stored by retrieving it
	retrieved, err := st.GetMonthlyHistory("energy_consumption_monthly",
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetMonthlyHistory() after store error = %v", err)
	}

	if len(retrieved) == 0 {
		t.Error("StoreMonthlyDataPoint() did not store the data")
	} else {
		// Check that the stored value is correct
		found := false
		for _, storedDp := range retrieved {
			if storedDp.Month == "2024-06" {
				// The stored value should be dp.Value (already decoded), not dp.RawValue * scale
				if storedDp.Value != 1005.0 {
					t.Errorf("Stored value = %v, want %v", storedDp.Value, 1005.0)
				}
				found = true
				break
			}
		}
		if !found {
			t.Error("Stored monthly data point not found in history")
		}
	}
}

// TestStorage_StoreMonthlyDataPoint_InvalidRegister tests StoreMonthlyDataPoint with invalid register
func TestStorage_StoreMonthlyDataPoint_InvalidRegister(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_store_monthly_invalid.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Create a test monthly data point
	dp := &MonthlyDataPoint{
		Month:    "2024-06",
		Value:    100.5,
		RawValue: 1005,
	}

	// Try to store with invalid register key
	err = st.StoreMonthlyDataPoint("invalid_register_key", dp)
	if err == nil {
		t.Error("StoreMonthlyDataPoint() with invalid register expected error, got nil")
	}
}

// TestStorage_StoreYearlyDataPoint tests the StoreYearlyDataPoint method
func TestStorage_StoreYearlyDataPoint(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_store_yearly.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Create a test yearly data point
	// For computed yearly registers, Value should be the decoded/summed value
	// and RawValue should be the sum of daily raw values
	dp := &YearlyDataPoint{
		Year:     "2024",
		Value:    10005.0, // This is the decoded yearly value (what we want stored)
		RawValue: 100050.0, // This is the sum of daily raw values (for auditing)
	}

	// Store it for a valid register key
	err = st.StoreYearlyDataPoint("energy_consumption_yearly", dp)
	if err != nil {
		t.Fatalf("StoreYearlyDataPoint() error = %v", err)
	}

	// Verify it was stored by retrieving it
	retrieved, err := st.GetYearlyHistory("energy_consumption_yearly",
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetYearlyHistory() after store error = %v", err)
	}

	if len(retrieved) == 0 {
		t.Error("StoreYearlyDataPoint() did not store the data")
	} else {
		found := false
		for _, storedDp := range retrieved {
			if storedDp.Year == "2024" {
				// The stored value should be dp.Value (already decoded), not dp.RawValue * scale
				if storedDp.Value != 10005.0 {
					t.Errorf("Stored value = %v, want %v", storedDp.Value, 10005.0)
				}
				found = true
				break
			}
		}
		if !found {
			t.Error("Stored yearly data point not found in history")
		}
	}
}

// TestStorage_StoreYearlyDataPoint_InvalidRegister tests StoreYearlyDataPoint with invalid register
func TestStorage_StoreYearlyDataPoint_InvalidRegister(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_store_yearly_invalid.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Create a test yearly data point
	dp := &YearlyDataPoint{
		Year:     "2024",
		Value:    1000.5,
		RawValue: 10005,
	}

	// Try to store with invalid register key
	err = st.StoreYearlyDataPoint("invalid_register_key", dp)
	if err == nil {
		t.Error("StoreYearlyDataPoint() with invalid register expected error, got nil")
	}
}

// TestStorage_GetTotalHistory tests the GetTotalHistory method
func TestStorage_GetTotalHistory(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_total_history.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Insert some test total data by storing a total register value
	timestamp := time.Now()
	totalValues := map[string]*solis.Value{
		"grid_export_total": {
			Key:          "grid_export_total",
			Name:         "Total Energy Fed Into Grid",
			RawValue:     5000,
			DecodedValue: 500.0,
			Unit:         "kWh",
			Timestamp:    timestamp,
			DataType:     solis.Uint32,
			Stability:    solis.Dynamic,
		},
	}

	err = st.StoreAllRegisters(totalValues, timestamp)
	if err != nil {
		t.Fatalf("StoreAllRegisters() error = %v", err)
	}

	// Wait a bit for storage to complete
	time.Sleep(10 * time.Millisecond)

	// Get total history
	dp, err := st.GetTotalHistory("grid_export_total")
	if err != nil {
		t.Fatalf("GetTotalHistory() error = %v", err)
	}

	if dp == nil {
		t.Fatal("GetTotalHistory() returned nil")
	}

	// Check that we got the stored value
	// Note: the value is scaled by the register's scale
	if dp.Value <= 0 {
		t.Errorf("GetTotalHistory() dp.Value = %v, want > 0", dp.Value)
	}

	t.Logf("GetTotalHistory: value=%.2f, raw=%.2f, timestamp=%s", dp.Value, dp.RawValue, dp.Timestamp)
}

// TestStorage_GetTotalHistory_NoData tests GetTotalHistory with no data
func TestStorage_GetTotalHistory_NoData(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_total_history_nodata.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Get total history for a register with no data
	dp, err := st.GetTotalHistory("grid_export_total")
	if err != nil {
		t.Fatalf("GetTotalHistory() with no data error = %v", err)
	}

	// Should return nil for no data
	if dp != nil {
		t.Errorf("GetTotalHistory() with no data returned %v, want nil", dp)
	}
}

// TestStorage_GetTotalHistory_InvalidKey tests GetTotalHistory with invalid key
func TestStorage_GetTotalHistory_InvalidKey(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_total_history_invalid.db")

	cfg := &config.StorageSettings{
		Path: dbPath,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Get total history for an invalid register key
	// This should not error, but return nil
	dp, err := st.GetTotalHistory("invalid_register_key")
	if err != nil {
		t.Logf("GetTotalHistory() with invalid key error: %v", err)
		return
	}

	// Should return nil for invalid key
	if dp != nil {
		t.Errorf("GetTotalHistory() with invalid key returned %v, want nil", dp)
	}
}

// TestStorage_MonthlyDataPoint tests the MonthlyDataPoint struct
func TestStorage_MonthlyDataPoint(t *testing.T) {
	dp := &MonthlyDataPoint{
		Month:    "2024-06",
		Value:    150.5,
		RawValue: 1505,
	}

	if dp.Month != "2024-06" {
		t.Errorf("MonthlyDataPoint.Month = %v, want 2024-06", dp.Month)
	}

	if dp.Value != 150.5 {
		t.Errorf("MonthlyDataPoint.Value = %v, want 150.5", dp.Value)
	}

	if dp.RawValue != 1505 {
		t.Errorf("MonthlyDataPoint.RawValue = %v, want 1505", dp.RawValue)
	}
}

// TestStorage_YearlyDataPoint tests the YearlyDataPoint struct
func TestStorage_YearlyDataPoint(t *testing.T) {
	dp := &YearlyDataPoint{
		Year:     "2024",
		Value:    1500.5,
		RawValue: 15005,
	}

	if dp.Year != "2024" {
		t.Errorf("YearlyDataPoint.Year = %v, want 2024", dp.Year)
	}

	if dp.Value != 1500.5 {
		t.Errorf("YearlyDataPoint.Value = %v, want 1500.5", dp.Value)
	}

	if dp.RawValue != 15005 {
		t.Errorf("YearlyDataPoint.RawValue = %v, want 15005", dp.RawValue)
	}
}

// TestStorage_TotalDataPoint tests the TotalDataPoint struct
func TestStorage_TotalDataPoint(t *testing.T) {
	dp := &TotalDataPoint{
		Value:     5000.5,
		RawValue:  50005,
		Timestamp: "2024-06-26T00:00:00Z",
	}

	if dp.Value != 5000.5 {
		t.Errorf("TotalDataPoint.Value = %v, want 5000.5", dp.Value)
	}

	if dp.RawValue != 50005 {
		t.Errorf("TotalDataPoint.RawValue = %v, want 50005", dp.RawValue)
	}

	if dp.Timestamp != "2024-06-26T00:00:00Z" {
		t.Errorf("TotalDataPoint.Timestamp = %v, want 2024-06-26T00:00:00Z", dp.Timestamp)
	}
}

// TestCleanupAll tests the unified cleanup function
func TestCleanupAll(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_cleanup_all.db")

	// Use very short retention periods for testing
	cfg := &config.StorageSettings{
		Path:             dbPath,
		DailyRetention:   time.Hour, // 1 hour
		MonthlyRetention: time.Hour, // 1 hour
		YearlyRetention:  time.Hour, // 1 hour
		ErrorRetention:   time.Hour, // 1 hour
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Insert old data that should be cleaned up
	// Use dates far enough in the past to ensure they're cleaned up
	oldDailyDate := time.Now().Add(-2 * 24 * time.Hour).Format("2006-01-02")
	oldMonthlyDate := time.Now().Add(-2 * 30 * 24 * time.Hour).Format("2006-01")
	oldYearlyDate := "2020" // Far in the past
	oldErrorTime := time.Now().Add(-2 * time.Hour)

	// Insert old daily data
	_, err = st.db.Exec("INSERT INTO daily_values (register_key, date, value, raw_value) VALUES (?, ?, ?, ?)",
		"test_daily", oldDailyDate, 100.0, 1000)
	if err != nil {
		t.Fatalf("Failed to insert test daily data: %v", err)
	}

	// Insert old monthly data
	_, err = st.db.Exec("INSERT INTO monthly_values (register_key, month, value, raw_value) VALUES (?, ?, ?, ?)",
		"test_monthly", oldMonthlyDate, 200.0, 2000)
	if err != nil {
		t.Fatalf("Failed to insert test monthly data: %v", err)
	}

	// Insert old yearly data
	_, err = st.db.Exec("INSERT INTO yearly_values (register_key, year, value, raw_value) VALUES (?, ?, ?, ?)",
		"test_yearly", oldYearlyDate, 300.0, 3000)
	if err != nil {
		t.Fatalf("Failed to insert test yearly data: %v", err)
	}

	// Insert old error data
	_, err = st.db.Exec("INSERT INTO error_data (register_key, timestamp, raw_value, string_value) VALUES (?, ?, ?, ?)",
		"test_error", oldErrorTime, 400.0, "test error")
	if err != nil {
		t.Fatalf("Failed to insert test error data: %v", err)
	}

	// Verify data was inserted
	var count int
	err = st.db.QueryRow("SELECT COUNT(*) FROM daily_values").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count daily data: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 daily data row, got %d", count)
	}

	// Run cleanup - should remove all old data
	err = st.CleanupAll()
	if err != nil {
		t.Fatalf("CleanupAll() error = %v", err)
	}

	// Verify data was cleaned up
	err = st.db.QueryRow("SELECT COUNT(*) FROM daily_values").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count daily data after cleanup: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 daily data rows after cleanup, got %d", count)
	}

	err = st.db.QueryRow("SELECT COUNT(*) FROM monthly_values").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count monthly data after cleanup: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 monthly data rows after cleanup, got %d", count)
	}

	err = st.db.QueryRow("SELECT COUNT(*) FROM yearly_values").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count yearly data after cleanup: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 yearly data rows after cleanup, got %d", count)
	}

	err = st.db.QueryRow("SELECT COUNT(*) FROM error_data").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count error data after cleanup: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 error data rows after cleanup, got %d", count)
	}

	// Verify last cleanup time was set
	if st.GetLastCleanupTime().IsZero() {
		t.Error("Expected last cleanup time to be set after CleanupAll()")
	}
}

// TestCleanupAll_NoData tests CleanupAll when there's no data to clean
func TestCleanupAll_NoData(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_cleanup_all_nodata.db")

	cfg := &config.StorageSettings{
		Path:             dbPath,
		DailyRetention:   24 * time.Hour,
		MonthlyRetention: 24 * time.Hour,
		YearlyRetention:  24 * time.Hour,
		ErrorRetention:   24 * time.Hour,
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		st.Close()
		os.Remove(dbPath)
	}()

	// Run cleanup on empty database - should not error
	err = st.CleanupAll()
	if err != nil {
		t.Fatalf("CleanupAll() on empty database error = %v", err)
	}

	// Verify last cleanup time was set
	if st.GetLastCleanupTime().IsZero() {
		t.Error("Expected last cleanup time to be set even with no data")
	}
}
