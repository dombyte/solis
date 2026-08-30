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
		{"household_energy_daily", true},
		{"energy_consumption_daily", true},
		{"grid_export_daily", true},
		{"grid_import_daily", true},
		{"battery_discharge_daily", true},
		{"battery_charge_daily", true},
		{"pv_energy_daily", true},
		{"backup_energy_daily", true},
		{"grid_energy_daily", true},

		// Non-daily registers
		{"pv_voltage_1", false},
		{"battery_voltage", false},
		{"energy_consumption_total", false},
		{"grid_energy_total", false},
		{"energy_consumption_monthly", false},
		{"grid_energy_monthly", false},

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
		{"pv_energy_monthly", true},
		{"household_energy_monthly", true},
		{"backup_energy_monthly", true},

		// Computed monthly registers
		{"energy_consumption_monthly", true},
		{"grid_export_monthly", true},
		{"grid_import_monthly", true},
		{"battery_discharge_monthly", true},
		{"battery_charge_monthly", true},
		{"grid_energy_monthly", true},

		// Non-monthly registers
		{"pv_voltage_1", false},
		{"energy_consumption_daily", false},
		{"energy_consumption_total", false},
		{"energy_consumption_yearly", false},

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
		{"pv_energy_yearly", true},
		{"household_energy_yearly", true},
		{"backup_energy_yearly", true},

		// Computed yearly registers
		{"energy_consumption_yearly", true},
		{"grid_export_yearly", true},
		{"grid_import_yearly", true},
		{"battery_discharge_yearly", true},
		{"battery_charge_yearly", true},
		{"grid_energy_yearly", true},

		// Non-yearly registers
		{"pv_voltage_1", false},
		{"energy_consumption_daily", false},
		{"energy_consumption_total", false},
		{"energy_consumption_monthly", false},

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
		{"pv_energy_total", true},
		{"battery_discharge_total", true},
		{"battery_charge_total", true},
		{"grid_import_total", true},
		{"grid_export_total", true},
		{"energy_consumption_total", true},
		{"household_energy_total", true},
		{"backup_energy_total", true},

		// Computed total register
		{"grid_energy_total", true},

		// Non-total registers
		{"pv_voltage_1", false},
		{"energy_consumption_daily", false},
		{"pv_energy_monthly", false},
		{"energy_consumption_yearly", false},

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
		"grid_energy_daily",
		"grid_energy_total",
		"energy_consumption_monthly",
		"grid_export_monthly",
		"grid_import_monthly",
		"battery_discharge_monthly",
		"battery_charge_monthly",
		"grid_energy_monthly",
		"energy_consumption_yearly",
		"grid_export_yearly",
		"grid_import_yearly",
		"battery_discharge_yearly",
		"battery_charge_yearly",
		"grid_energy_yearly",
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
		"grid_energy_daily",
		"grid_energy_total",
		"energy_consumption_monthly",
		"grid_export_monthly",
		"grid_import_monthly",
		"battery_discharge_monthly",
		"battery_charge_monthly",
		"grid_energy_monthly",
		"energy_consumption_yearly",
		"grid_export_yearly",
		"grid_import_yearly",
		"battery_discharge_yearly",
		"battery_charge_yearly",
		"grid_energy_yearly",
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
		"grid_energy_daily",
		"grid_energy_total",
		"energy_consumption_monthly",
		"grid_export_monthly",
		"grid_import_monthly",
		"battery_discharge_monthly",
		"battery_charge_monthly",
		"grid_energy_monthly",
		"energy_consumption_yearly",
		"grid_export_yearly",
		"grid_import_yearly",
		"battery_discharge_yearly",
		"battery_charge_yearly",
		"grid_energy_yearly",
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
