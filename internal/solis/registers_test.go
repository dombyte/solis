package solis

import (
	"testing"
)

// TestIsDailyRegister tests the IsDailyRegister function
func TestIsDailyRegister(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		// Daily registers
		{"household_load_today_energy", true},
		{"today_energy_consumption", true},
		{"today_energy_fed_into_grid", true},
		{"today_energy_imported_from_grid", true},
		{"today_battery_discharge_energy", true},
		{"today_battery_charge_energy", true},
		{"pv_today_energy", true},
		{"backup_load_today_energy", true},
		{"today_grid_energy", true},

		// Non-daily registers
		{"pv_voltage_1", false},
		{"battery_voltage", false},
		{"total_energy_consumption", false},
		{"total_grid_energy", false},
		{"energy_consumption_month_energy", false},
		{"month_grid_energy", false},

		// Edge cases
		{"", false},
		{"nonexistent_register", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := IsDailyRegister(tt.key)
			if result != tt.expected {
				t.Errorf("IsDailyRegister(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

// TestIsMonthlyRegister tests the IsMonthlyRegister function
func TestIsMonthlyRegister(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		// Monthly registers (real Modbus registers)
		{"pv_month_energy", true},
		{"household_load_month_energy", true},
		{"backup_load_month_energy", true},

		// Computed monthly registers
		{"energy_consumption_month_energy", true},
		{"energy_fed_into_grid_month_energy", true},
		{"energy_imported_from_grid_month_energy", true},
		{"battery_discharge_month_energy", true},
		{"battery_charge_month_energy", true},
		{"month_grid_energy", true},

		// Non-monthly registers
		{"pv_voltage_1", false},
		{"today_energy_consumption", false},
		{"total_energy_consumption", false},
		{"energy_consumption_year_energy", false},

		// Edge cases
		{"", false},
		{"nonexistent_register", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := IsMonthlyRegister(tt.key)
			if result != tt.expected {
				t.Errorf("IsMonthlyRegister(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

// TestIsYearlyRegister tests the IsYearlyRegister function
func TestIsYearlyRegister(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		// Yearly registers (real Modbus registers)
		{"pv_year_energy", true},
		{"household_load_year_energy", true},
		{"backup_load_year_energy", true},

		// Computed yearly registers
		{"energy_consumption_year_energy", true},
		{"energy_fed_into_grid_year_energy", true},
		{"energy_imported_from_grid_year_energy", true},
		{"battery_discharge_year_energy", true},
		{"battery_charge_year_energy", true},
		{"year_grid_energy", true},

		// Non-yearly registers
		{"pv_voltage_1", false},
		{"today_energy_consumption", false},
		{"total_energy_consumption", false},
		{"energy_consumption_month_energy", false},

		// Edge cases
		{"", false},
		{"nonexistent_register", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := IsYearlyRegister(tt.key)
			if result != tt.expected {
				t.Errorf("IsYearlyRegister(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

// TestIsTotalRegister tests the IsTotalRegister function
func TestIsTotalRegister(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		// Total registers (real Modbus registers)
		{"pv_total_energy", true},
		{"total_battery_discharge_energy", true},
		{"total_battery_charge_energy", true},
		{"total_energy_imported_from_grid", true},
		{"total_energy_fed_into_grid", true},
		{"total_energy_consumption", true},
		{"household_load_total_energy", true},
		{"backup_load_total_energy", true},

		// Computed total register
		{"total_grid_energy", true},

		// Non-total registers
		{"pv_voltage_1", false},
		{"today_energy_consumption", false},
		{"pv_month_energy", false},
		{"energy_consumption_year_energy", false},

		// Edge cases
		{"", false},
		{"nonexistent_register", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := IsTotalRegister(tt.key)
			if result != tt.expected {
				t.Errorf("IsTotalRegister(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

// TestRegisterMapByKey tests that RegisterMapByKey contains all registers
func TestRegisterMapByKey(t *testing.T) {
	// Verify all registers are in the map
	for _, reg := range AllRegisters {
		if _, ok := RegisterMapByKey[reg.Key]; !ok {
			t.Errorf("Register %s not found in RegisterMapByKey", reg.Key)
		}
	}

	// Verify computed registers are in the map
	computedKeys := []string{
		"today_grid_energy",
		"total_grid_energy",
		"energy_consumption_month_energy",
		"energy_fed_into_grid_month_energy",
		"energy_imported_from_grid_month_energy",
		"battery_discharge_month_energy",
		"battery_charge_month_energy",
		"month_grid_energy",
		"energy_consumption_year_energy",
		"energy_fed_into_grid_year_energy",
		"energy_imported_from_grid_year_energy",
		"battery_discharge_year_energy",
		"battery_charge_year_energy",
		"year_grid_energy",
	}

	for _, key := range computedKeys {
		if _, ok := RegisterMapByKey[key]; !ok {
			t.Errorf("Computed register %s not found in RegisterMapByKey", key)
		}
	}
}

// TestComputedRegistersHaveZeroAddress tests that computed registers have Address=0
func TestComputedRegistersHaveZeroAddress(t *testing.T) {
	computedKeys := []string{
		"today_grid_energy",
		"total_grid_energy",
		"energy_consumption_month_energy",
		"energy_fed_into_grid_month_energy",
		"energy_imported_from_grid_month_energy",
		"battery_discharge_month_energy",
		"battery_charge_month_energy",
		"month_grid_energy",
		"energy_consumption_year_energy",
		"energy_fed_into_grid_year_energy",
		"energy_imported_from_grid_year_energy",
		"battery_discharge_year_energy",
		"battery_charge_year_energy",
		"year_grid_energy",
	}

	for _, key := range computedKeys {
		reg, ok := RegisterMapByKey[key]
		if !ok {
			t.Errorf("Computed register %s not found", key)
			continue
		}
		if reg.Address != 0 {
			t.Errorf("Computed register %s has Address=%d, want 0", key, reg.Address)
		}
		if reg.Count != 0 {
			t.Errorf("Computed register %s has Count=%d, want 0", key, reg.Count)
		}
	}
}

// TestComputedRegistersAreDynamic tests that computed registers have Dynamic stability
func TestComputedRegistersAreDynamic(t *testing.T) {
	computedKeys := []string{
		"today_grid_energy",
		"total_grid_energy",
		"energy_consumption_month_energy",
		"energy_fed_into_grid_month_energy",
		"energy_imported_from_grid_month_energy",
		"battery_discharge_month_energy",
		"battery_charge_month_energy",
		"month_grid_energy",
		"energy_consumption_year_energy",
		"energy_fed_into_grid_year_energy",
		"energy_imported_from_grid_year_energy",
		"battery_discharge_year_energy",
		"battery_charge_year_energy",
		"year_grid_energy",
	}

	for _, key := range computedKeys {
		reg, ok := RegisterMapByKey[key]
		if !ok {
			t.Errorf("Computed register %s not found", key)
			continue
		}
		if reg.Stability != Dynamic {
			t.Errorf("Computed register %s has Stability=%s, want Dynamic", key, reg.Stability)
		}
	}
}

// TestRegisterFilter tests the RegisterFilter functionality
func TestRegisterFilter(t *testing.T) {
	// Test with no disabled keys
	rf := NewRegisterFilter([]string{})
	if !rf.IsEnabled("pv_voltage_1") {
		t.Error("RegisterFilter with no disabled keys should enable all registers")
	}

	// Test with some disabled keys
	disabledKeys := []string{"pv_voltage_1", "battery_voltage"}
	rf = NewRegisterFilter(disabledKeys)

	if rf.IsEnabled("pv_voltage_1") {
		t.Error("RegisterFilter should disable pv_voltage_1")
	}
	if rf.IsEnabled("battery_voltage") {
		t.Error("RegisterFilter should disable battery_voltage")
	}
	if !rf.IsEnabled("pv_current_1") {
		t.Error("RegisterFilter should enable pv_current_1")
	}

	// Test with nil filter
	var nilFilter *RegisterFilter
	if !nilFilter.IsEnabled("pv_voltage_1") {
		t.Error("Nil RegisterFilter should enable all registers")
	}
}

// TestRegisterFilter_FilterRegisters tests the FilterRegisters method
func TestRegisterFilter_FilterRegisters(t *testing.T) {
	allRegs := []Register{
		{Key: "pv_voltage_1", Name: "PV Voltage 1"},
		{Key: "pv_current_1", Name: "PV Current 1"},
		{Key: "battery_voltage", Name: "Battery Voltage"},
	}

	disabledKeys := []string{"battery_voltage"}
	rf := NewRegisterFilter(disabledKeys)

	filtered := rf.FilterRegisters(allRegs)

	if len(filtered) != 2 {
		t.Errorf("FilterRegisters() returned %d registers, want 2", len(filtered))
	}

	// Check that battery_voltage was filtered out
	for _, reg := range filtered {
		if reg.Key == "battery_voltage" {
			t.Error("battery_voltage should have been filtered out")
		}
	}

	// Check that other registers are present
	foundPvVoltage := false
	foundPvCurrent := false
	for _, reg := range filtered {
		if reg.Key == "pv_voltage_1" {
			foundPvVoltage = true
		}
		if reg.Key == "pv_current_1" {
			foundPvCurrent = true
		}
	}
	if !foundPvVoltage || !foundPvCurrent {
		t.Error("Expected registers not found in filtered list")
	}
}

// TestRegisterFilter_FilterMapByKey tests the FilterMapByKey method
func TestRegisterFilter_FilterMapByKey(t *testing.T) {
	disabledKeys := []string{"pv_voltage_1"}
	rf := NewRegisterFilter(disabledKeys)

	filteredMap := rf.FilterMapByKey()

	// Check that pv_voltage_1 is not in the filtered map
	if _, ok := filteredMap["pv_voltage_1"]; ok {
		t.Error("pv_voltage_1 should have been filtered out")
	}

	// Check that other registers are still present (assuming they exist)
	if len(filteredMap) == 0 {
		t.Error("Filtered map should not be empty")
	}
}

// TestRegisterFilter_FilterKeys tests the FilterKeys method
func TestRegisterFilter_FilterKeys(t *testing.T) {
	disabledKeys := []string{"pv_voltage_1", "battery_voltage"}
	rf := NewRegisterFilter(disabledKeys)

	keys := rf.FilterKeys()

	// Check that disabled keys are not in the result
	for _, disabledKey := range disabledKeys {
		for _, key := range keys {
			if key == disabledKey {
				t.Errorf("Disabled key %s should have been filtered out", disabledKey)
			}
		}
	}

	// Check that at least some keys are present
	if len(keys) == 0 {
		t.Error("Filtered keys should not be empty")
	}
}

// TestRegisterMap tests that RegisterMap contains all registers with addresses
func TestRegisterMap(t *testing.T) {
	// Verify that all registers with non-zero addresses are in the map
	for _, reg := range AllRegisters {
		if reg.Address > 0 {
			if _, ok := RegisterMap[reg.Address]; !ok {
				t.Errorf("Register with address %d not found in RegisterMap", reg.Address)
			}
		}
	}
}

// TestAllRegistersNotEmpty tests that AllRegisters contains registers
func TestAllRegistersNotEmpty(t *testing.T) {
	if len(AllRegisters) == 0 {
		t.Error("AllRegisters should not be empty")
	}
}

// TestStabilityString tests the Stability.String() method
func TestStabilityString(t *testing.T) {
	tests := []struct {
		stability Stability
		want      string
	}{
		{Stable, "stable"},
		{Dynamic, "dynamic"},
		{Stability(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.stability.String(); got != tt.want {
				t.Errorf("Stability(%d).String() = %v, want %v", tt.stability, got, tt.want)
			}
		})
	}
}

// TestDataTypeString tests the DataType.String() method
func TestDataTypeString(t *testing.T) {
	tests := []struct {
		dataType DataType
		want     string
	}{
		{Uint16, "Uint16"},
		{Int16, "Int16"},
		{Uint32, "Uint32"},
		{Int32, "Int32"},
		{Float32, "Float32"},
		{String, "String"},
		{Bool, "Bool"},
		{DataType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.dataType.String(); got != tt.want {
				t.Errorf("DataType(%d).String() = %v, want %v", tt.dataType, got, tt.want)
			}
		})
	}
}

// TestNetGridEnergyRegistersExist tests that net grid energy registers exist
func TestNetGridEnergyRegistersExist(t *testing.T) {
	netKeys := []string{"today_grid_energy", "total_grid_energy", "month_grid_energy", "year_grid_energy"}

	for _, key := range netKeys {
		if _, ok := RegisterMapByKey[key]; !ok {
			t.Errorf("Net grid energy register %s not found", key)
		}
	}
}

// TestComputedMonthlyRegistersExist tests that computed monthly registers exist
func TestComputedMonthlyRegistersExist(t *testing.T) {
	keys := []string{
		"energy_consumption_month_energy",
		"energy_fed_into_grid_month_energy",
		"energy_imported_from_grid_month_energy",
		"battery_discharge_month_energy",
		"battery_charge_month_energy",
		"month_grid_energy",
	}

	for _, key := range keys {
		if _, ok := RegisterMapByKey[key]; !ok {
			t.Errorf("Computed monthly register %s not found", key)
		}
	}
}

// TestComputedYearlyRegistersExist tests that computed yearly registers exist
func TestComputedYearlyRegistersExist(t *testing.T) {
	keys := []string{
		"energy_consumption_year_energy",
		"energy_fed_into_grid_year_energy",
		"energy_imported_from_grid_year_energy",
		"battery_discharge_year_energy",
		"battery_charge_year_energy",
		"year_grid_energy",
	}

	for _, key := range keys {
		if _, ok := RegisterMapByKey[key]; !ok {
			t.Errorf("Computed yearly register %s not found", key)
		}
	}
}

// TestRegisterSetsConsistency tests that all real (non-computed) register sets are consistent
// Note: Computed registers like today_grid_energy, total_grid_energy, etc. are in the sets
// but not in the Keys arrays because they don't have real Modbus addresses
func TestRegisterSetsConsistency(t *testing.T) {
	// Check that all real daily register keys (with Address > 0) are in DailyRegisterKeys
	for key := range dailyRegisterSet {
		// Skip computed registers (Address = 0)
		reg, ok := RegisterMapByKey[key]
		if !ok {
			continue
		}
		if reg.Address == 0 {
			continue // Skip computed/virtual registers
		}
		found := false
		for _, k := range DailyRegisterKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Daily register set contains %s which is not in DailyRegisterKeys", key)
		}
	}

	// Check that all real monthly register keys (with Address > 0) are in MonthlyRegisterKeys
	for key := range monthlyRegisterSet {
		// Skip computed registers (Address = 0)
		reg, ok := RegisterMapByKey[key]
		if !ok {
			continue
		}
		if reg.Address == 0 {
			continue // Skip computed/virtual registers
		}
		found := false
		for _, k := range MonthlyRegisterKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Monthly register set contains %s which is not in MonthlyRegisterKeys", key)
		}
	}

	// Check that all real yearly register keys (with Address > 0) are in YearlyRegisterKeys
	for key := range yearlyRegisterSet {
		// Skip computed registers (Address = 0)
		reg, ok := RegisterMapByKey[key]
		if !ok {
			continue
		}
		if reg.Address == 0 {
			continue // Skip computed/virtual registers
		}
		found := false
		for _, k := range YearlyRegisterKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Yearly register set contains %s which is not in YearlyRegisterKeys", key)
		}
	}

	// Check that all real total register keys (with Address > 0) are in TotalRegisterKeys
	for key := range totalRegisterSet {
		// Skip computed registers (Address = 0)
		reg, ok := RegisterMapByKey[key]
		if !ok {
			continue
		}
		if reg.Address == 0 {
			continue // Skip computed/virtual registers
		}
		found := false
		for _, k := range TotalRegisterKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Total register set contains %s which is not in TotalRegisterKeys", key)
		}
	}
}
