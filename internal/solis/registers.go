// Package solis provides Solis inverter register definitions, status maps,
// and utilities for decoding Modbus register data.
package solis

import (
	"fmt"

	"github.com/dombyte/solis/internal/logging"
)

// logger is the package-level logger for solis.
var logger = logging.NewComponentLogger("solis")

// DataType represents the data type of a register value.
type DataType int

const (
	// Uint16 is an unsigned 16-bit integer (1 register).
	Uint16 DataType = iota
	// Int16 is a signed 16-bit integer (1 register).
	Int16
	// Uint32 is an unsigned 32-bit integer (2 registers, big-endian).
	Uint32
	// Int32 is a signed 32-bit integer (2 registers, big-endian).
	Int32
	// Float32 is a 32-bit floating point (2 registers, IEEE 754).
	Float32
	// String is a ASCII-encoded string (multiple registers).
	String
	// Bool is a boolean value (1 register, 0=false, non-zero=true).
	Bool
)

// String returns the string representation of the DataType.
func (d DataType) String() string {
	switch d {
	case Uint16:
		return "Uint16"
	case Int16:
		return "Int16"
	case Uint32:
		return "Uint32"
	case Int32:
		return "Int32"
	case Float32:
		return "Float32"
	case String:
		return "String"
	case Bool:
		return "Bool"
	default:
		return "Unknown"
	}
}

// Stability indicates whether a register value is stable (rarely changes)
// or dynamic (changes frequently).
type Stability int

const (
	// Stable registers contain configuration, serial numbers, versions.
	// Written once at startup or on first successful poll.
	Stable Stability = iota
	// Dynamic registers contain measurements like voltage, current, power.
	// Written on every poll if the value has changed.
	Dynamic
)

// String returns the string representation of the Stability.
func (s Stability) String() string {
	switch s {
	case Stable:
		return "stable"
	case Dynamic:
		return "dynamic"
	default:
		return "unknown"
	}
}

// Register represents a Solis inverter Modbus register.
type Register struct {
	// Key is the unique identifier for this register (used in API responses, storage).
	Key string
	// Name is the human-readable name of the register.
	Name string
	// Address is the Modbus register address (0-based or 1-based depending on device).
	Address uint16
	// Count is the number of consecutive registers this value occupies.
	// 1 for Uint16/Int16/Bool, 2 for Uint32/Int32/Float32, N for String.
	Count uint16
	// DataType is the type of value stored in this register.
	DataType DataType
	// Scale is the scaling factor applied to the raw value.
	// e.g., 0.1 means divide by 10, 1.0 means no scaling.
	Scale float64
	// Unit is the unit of measurement (e.g., "V", "A", "kWh", "C").
	Unit string
	// Stability indicates how often this value changes.
	Stability Stability
	// Status indicates this is a status/fault register (stored in status table, not raw_data).
	Status bool
}

// RegisterMap provides O(1) lookup of registers by their starting address.
// This is the primary way to find a register when processing raw Modbus data.
var RegisterMap = buildRegisterMap()

// AllRegisters is a slice of all defined registers.
// This is useful for iteration over all registers.
var AllRegisters = allRegisters()

// RegisterMapByKey provides O(1) lookup of registers by their key.
var RegisterMapByKey = buildRegisterMapByKey()

// DailyToMonthlyMap maps daily register keys to their corresponding monthly register keys.
// Used by the aggregator to compute monthly values from daily storage.
// These are COMPUTED registers (not directly polled) that should be aggregated from daily values.
var DailyToMonthlyMap = map[string]string{
	"energy_consumption_daily": "energy_consumption_monthly",
	"grid_export_daily":        "grid_export_monthly",
	"grid_import_daily":        "grid_import_monthly",
	"battery_discharge_daily":  "battery_discharge_monthly",
	"battery_charge_daily":     "battery_charge_monthly",
}

// BackfillDailyToMonthlyMap includes ALL daily-to-monthly mappings, including
// directly-polled registers. Used by the backfill to recompute ALL monthly values
// from daily data, overriding any polled values for the current year.
var BackfillDailyToMonthlyMap = map[string]string{
	"energy_consumption_daily": "energy_consumption_monthly",
	"grid_export_daily":        "grid_export_monthly",
	"grid_import_daily":        "grid_import_monthly",
	"battery_discharge_daily":  "battery_discharge_monthly",
	"battery_charge_daily":     "battery_charge_monthly",
	// Directly-polled energy registers that also have daily equivalents
	"pv_energy_daily":         "pv_energy_monthly",
	"household_energy_daily":  "household_energy_monthly",
	"backup_energy_daily":     "backup_energy_monthly",
}

// DailyToYearlyMap maps daily register keys to their corresponding yearly register keys.
// Used by the aggregator to compute yearly values from daily storage.
// These are COMPUTED registers (not directly polled) that should be aggregated from daily values.
var DailyToYearlyMap = map[string]string{
	"energy_consumption_daily": "energy_consumption_yearly",
	"grid_export_daily":        "grid_export_yearly",
	"grid_import_daily":        "grid_import_yearly",
	"battery_discharge_daily":  "battery_discharge_yearly",
	"battery_charge_daily":     "battery_charge_yearly",
}

// BackfillDailyToYearlyMap includes ALL daily-to-yearly mappings, including
// directly-polled registers. Used by the backfill to recompute ALL yearly values
// from daily data, overriding any polled values for the current year.
var BackfillDailyToYearlyMap = map[string]string{
	"energy_consumption_daily": "energy_consumption_yearly",
	"grid_export_daily":        "grid_export_yearly",
	"grid_import_daily":        "grid_import_yearly",
	"battery_discharge_daily":  "battery_discharge_yearly",
	"battery_charge_daily":     "battery_charge_yearly",
	// Directly-polled energy registers that also have daily equivalents
	"pv_energy_daily":         "pv_energy_yearly",
	"household_energy_daily":  "household_energy_yearly",
	"backup_energy_daily":     "backup_energy_yearly",
}

// DailyRegisterKeys are the register keys that should have daily aggregation.
// These are energy registers that accumulate during the day and reset at midnight.
var DailyRegisterKeys = []string{
	"household_energy_daily",
	"energy_consumption_daily",
	"grid_export_daily",
	"grid_import_daily",
	"battery_discharge_daily",
	"battery_charge_daily",
	"pv_energy_daily",
	"backup_energy_daily",
}

// dailyRegisterSet provides O(1) lookup for daily registers.
var dailyRegisterSet = map[string]bool{
	"household_energy_daily":   true,
	"energy_consumption_daily": true,
	"grid_export_daily":        true,
	"grid_import_daily":        true,
	"battery_discharge_daily":  true,
	"battery_charge_daily":     true,
	"grid_energy_daily":        true,
	"pv_energy_daily":          true,
	"backup_energy_daily":      true,
}

// IsDailyRegister returns true if the key is a daily energy register.
func IsDailyRegister(key string) bool {
	return dailyRegisterSet[key]
}

// MonthlyRegisterKeys are the register keys that should have monthly aggregation.
// These are energy registers that accumulate during the month and reset at the start of a new month.
var MonthlyRegisterKeys = []string{
	"pv_energy_monthly",
	"household_energy_monthly",
	"backup_energy_monthly",
	// Computed monthly registers
	"energy_consumption_monthly",
	"grid_export_monthly",
	"grid_import_monthly",
	"battery_discharge_monthly",
	"battery_charge_monthly",
	"grid_energy_monthly",
}

// monthlyRegisterSet provides O(1) lookup for monthly registers.
var monthlyRegisterSet = map[string]bool{
	"pv_energy_monthly":        true,
	"household_energy_monthly": true,
	"backup_energy_monthly":    true,
	// Computed monthly registers
	"energy_consumption_monthly": true,
	"grid_export_monthly":        true,
	"grid_import_monthly":        true,
	"battery_discharge_monthly":  true,
	"battery_charge_monthly":     true,
	"grid_energy_monthly":        true,
}

// IsMonthlyRegister returns true if the key is a monthly energy register.
func IsMonthlyRegister(key string) bool {
	return monthlyRegisterSet[key]
}

// YearlyRegisterKeys are the register keys that should have yearly aggregation.
// These are energy registers that accumulate during the year and reset at the start of a new year.
var YearlyRegisterKeys = []string{
	"pv_energy_yearly",
	"household_energy_yearly",
	"backup_energy_yearly",
	// Computed yearly registers
	"energy_consumption_yearly",
	"grid_export_yearly",
	"grid_import_yearly",
	"battery_discharge_yearly",
	"battery_charge_yearly",
	"grid_energy_yearly",
}

// yearlyRegisterSet provides O(1) lookup for yearly registers.
var yearlyRegisterSet = map[string]bool{
	"pv_energy_yearly":        true,
	"household_energy_yearly": true,
	"backup_energy_yearly":    true,
	// Computed yearly registers
	"energy_consumption_yearly": true,
	"grid_export_yearly":        true,
	"grid_import_yearly":        true,
	"battery_discharge_yearly":  true,
	"battery_charge_yearly":     true,
	"grid_energy_yearly":        true,
}

// IsYearlyRegister returns true if the key is a yearly energy register.
func IsYearlyRegister(key string) bool {
	return yearlyRegisterSet[key]
}

// ComputedRegisterSet provides O(1) lookup for registers whose values are
// computed by the aggregator (monthly, yearly, net values).
// These registers should NOT be updated by the poller from Modbus reads.
var ComputedRegisterSet = map[string]bool{
	// Monthly registers (computed from daily storage)
	"energy_consumption_monthly": true,
	"grid_export_monthly":        true,
	"grid_import_monthly":        true,
	"battery_discharge_monthly":  true,
	"battery_charge_monthly":     true,
	// Yearly registers (computed from daily storage)
	"energy_consumption_yearly": true,
	"grid_export_yearly":        true,
	"grid_import_yearly":        true,
	"battery_discharge_yearly":  true,
	"battery_charge_yearly":     true,
	// Net grid energy values (computed from totals)
	"grid_energy_total":   true,
	"grid_energy_daily":   true,
	"grid_energy_monthly": true,
	"grid_energy_yearly":  true,
}

// IsComputedRegister returns true if the key is a computed register
// (monthly, yearly, or net values that are computed by the aggregator).
func IsComputedRegister(key string) bool {
	return ComputedRegisterSet[key]
}

// TotalRegisterKeys are the register keys that should have total aggregation.
// These are energy registers that accumulate indefinitely (lifetime totals).
var TotalRegisterKeys = []string{
	"pv_energy_total",
	"battery_discharge_total",
	"battery_charge_total",
	"grid_import_total",
	"grid_export_total",
	"energy_consumption_total",
	"household_energy_total",
	"backup_energy_total",
}

// totalRegisterSet provides O(1) lookup for total registers.
var totalRegisterSet = map[string]bool{
	"pv_energy_total":          true,
	"battery_discharge_total":  true,
	"battery_charge_total":     true,
	"grid_import_total":        true,
	"grid_export_total":        true,
	"energy_consumption_total": true,
	"grid_energy_total":        true,
	"household_energy_total":   true,
	"backup_energy_total":      true,
}

// IsTotalRegister returns true if the key is a total energy register.
func IsTotalRegister(key string) bool {
	return totalRegisterSet[key]
}

// buildRegisterMap constructs the RegisterMap from all defined registers.
func buildRegisterMap() map[uint16]*Register {
	m := make(map[uint16]*Register)
	for _, r := range AllRegisters {
		m[r.Address] = &r
	}
	return m
}

// buildRegisterMapByKey constructs the RegisterMapByKey from all defined registers.
func buildRegisterMapByKey() map[string]*Register {
	m := make(map[string]*Register)
	for _, r := range AllRegisters {
		m[r.Key] = &r
	}
	return m
}

// allRegisters returns a slice of all defined registers.
// This is used to build the RegisterMap and for iteration.
func allRegisters() []Register {
	return []Register{
		// =====================================================================
		// INFORMATION REGISTERS (33000-33048)
		// Removed per v2 plan - these were stable registers that are no longer polled
		// =====================================================================

		// =====================================================================
		// ENERGY REGISTERS (33029-33048)
		// =====================================================================

		{
			Key:       "pv_energy_daily",
			Name:      "PV Energy Daily",
			Address:   33035,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "pv_energy_monthly",
			Name:      "PV Energy Monthly",
			Address:   33031,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "pv_energy_yearly",
			Name:      "PV Energy Yearly",
			Address:   33037,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "pv_energy_total",
			Name:      "PV Energy Total",
			Address:   33029,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		// =====================================================================
		// PV VOLTAGE/CURRENT/POWER REGISTERS (33049-33058)
		// Removed per v2 plan - keeping only energy registers
		// =====================================================================

		// =====================================================================
		// GRID VOLTAGE/CURRENT/POWER/FREQUENCY REGISTERS (33073-33094)
		// Removed per v2 plan - keeping only energy and status registers
		// =====================================================================

		// =====================================================================
		// STATUS REGISTERS (33093-33096)
		// =====================================================================

		{
			Key:       "solis_status",
			Name:      "Solis Status",
			Address:   33095,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},

		{
			Key:       "grid_fault_1",
			Name:      "Grid Fault 1 (Bitmask)",
			Address:   33116,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "backup_fault_2",
			Name:      "Backup Fault 2 (Bitmask)",
			Address:   33117,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "battery_fault_3",
			Name:      "Battery Fault 3 (Bitmask)",
			Address:   33118,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "device_fault_4",
			Name:      "Device Fault 4 (Bitmask)",
			Address:   33119,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "device_fault_5",
			Name:      "Device Fault 5 (Bitmask)",
			Address:   33120,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "operating_status",
			Name:      "Solis Operating Status (Bitmask)",
			Address:   33121,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},

		// =====================================================================
		// BATTERY/SOC/SOH REGISTERS (33133-33141)
		// Removed per v2 plan - keeping only energy and status registers
		// =====================================================================

		{
			Key:       "battery_fault_1_bms",
			Name:      "Battery Fault 1 (BMS)",
			Address:   33145,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "battery_fault_2_bms",
			Name:      "Battery Fault 2 (BMS)",
			Address:   33146,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		// =====================================================================
		// POWER REGISTERS (33147-33152)
		// Removed per v2 plan - keeping only energy registers
		// =====================================================================

		{
			Key:       "battery_charge_daily",
			Name:      "Battery Charge Daily",
			Address:   33163,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "battery_discharge_daily",
			Name:      "Battery Discharge Daily",
			Address:   33167,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "battery_discharge_total",
			Name:      "Battery Discharge Total",
			Address:   33165,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "battery_charge_total",
			Name:      "Battery Charge Total",
			Address:   33161,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "grid_import_daily",
			Name:      "Grid Import Daily",
			Address:   33171,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "grid_import_total",
			Name:      "Grid Import Total",
			Address:   33169,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "grid_export_total",
			Name:      "Grid Export Total",
			Address:   33173,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "grid_export_daily",
			Name:      "Grid Export Daily",
			Address:   33175,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "energy_consumption_daily",
			Name:      "Energy Consumption Daily",
			Address:   33179,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "energy_consumption_total",
			Name:      "Energy Consumption Total",
			Address:   33177,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		// Computed net grid energy registers (virtual - no Modbus address)
		{
			Key:       "grid_energy_total",
			Name:      "Grid Energy Total (Net)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "grid_energy_daily",
			Name:      "Grid Energy Daily (Net)",
			Address:   0,
			Count:     0,
			DataType:  Uint16,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		// Computed monthly and yearly energy registers (virtual - no Modbus address)
		// These sum daily values from storage
		{
			Key:       "energy_consumption_monthly",
			Name:      "Energy Consumption Monthly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "grid_export_monthly",
			Name:      "Grid Export Monthly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "grid_import_monthly",
			Name:      "Grid Import Monthly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_discharge_monthly",
			Name:      "Battery Discharge Monthly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_charge_monthly",
			Name:      "Battery Charge Monthly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "grid_energy_monthly",
			Name:      "Grid Energy Monthly (Net, Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "energy_consumption_yearly",
			Name:      "Energy Consumption Yearly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "grid_export_yearly",
			Name:      "Grid Export Yearly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "grid_import_yearly",
			Name:      "Grid Import Yearly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_discharge_yearly",
			Name:      "Battery Discharge Yearly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_charge_yearly",
			Name:      "Battery Charge Yearly (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "grid_energy_yearly",
			Name:      "Grid Energy Yearly (Net, Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_energy_daily",
			Name:      "Household Energy Daily",
			Address:   33586,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_energy_yearly",
			Name:      "Household Energy Yearly",
			Address:   33582,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_energy_monthly",
			Name:      "Household Energy Monthly",
			Address:   33584,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_energy_total",
			Name:      "Household Energy Total",
			Address:   33580,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_energy_daily",
			Name:      "Backup Energy Daily",
			Address:   33596,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_energy_total",
			Name:      "Backup Energy Total",
			Address:   33590,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_energy_yearly",
			Name:      "Backup Energy Yearly",
			Address:   33592,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_energy_monthly",
			Name:      "Backup Energy Monthly",
			Address:   33594,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		// =====================================================================
		// METER REGISTERS (33251-33264)
		// Removed per v2 plan - meter registers are no longer polled
		// =====================================================================
	}
}

// =============================================================================
// READ RANGES
// These are the contiguous blocks we read from the inverter in one poll cycle.
// Each range is read as a single Modbus ReadInputRegisters call.
// Optimized to use 5 ranges (previously 7) by merging household groups and BMS/battery groups.
// =============================================================================

// ReadRanges defines the ranges to read sequentially from the inverter.
// Each entry contains the starting address and number of registers to read.
// Optimized to minimize both the number of read operations and extra registers read.
// Merges: household groups (gap=3) and BMS/battery groups (gap=14).
var ReadRanges = [5]struct {
	StartAddr uint16
	Count     uint16
}{
	{StartAddr: 33029, Count: 10}, // 33029-33038: PV energy registers (total, monthly, daily, yearly)
	{StartAddr: 33095, Count: 1},  // 33095: Solis status
	{StartAddr: 33116, Count: 6},  // 33116-33121: Fault/status registers
	{StartAddr: 33145, Count: 35}, // 33145-33179: BMS fault and battery/grid/energy registers (merged)
	{StartAddr: 33580, Count: 17}, // 33580-33596: Household and backup energy registers (merged)
}

// STATUS_MAP maps raw status code values (from register 33095) to short status names.
var STATUS_MAP = map[uint16]string{
	// Normal operation states
	0x0000: "Waiting",
	0x0001: "OpenRun",
	0x0002: "SoftRun",
	0x0003: "Generating",
	0x0004: "Standby",
	0x0005: "StandbySynch",
	0x0006: "GridToLoad",
	0x000F: "Normal",
	// Fault states
	0x1004: "Grid Off",
	0x1010: "OV-G-V", // Grid overvoltage
	0x1011: "UN-G-V", // Grid undervoltage
	0x1012: "OV-G-F", // Grid overfrequency
	0x1013: "UN-G-F", // Grid underfrequency
	0x1014: "G-IMP/Reve-Grid",
	0x1015: "NO-Grid",
	0x1016: "G-PHASE",
	0x1017: "G-F-FLU",
	0x1018: "OV-G-I",
	0x1019: "IGFOL-F",
	0x1020: "OV-DC",
	0x1021: "OV-BUS",
	0x1022: "UNB-BUS",
	0x1023: "UN-BUS",
	0x1024: "UNB2-BUS",
	0x1025: "OV-DCA-I",
	0x1026: "OV-DCB-I",
	0x1027: "DC-INTF.",
	0x1028: "Reve-DC",
	0x1029: "PvMidIso",
	0x1030: "GRID-INTF.",
	0x1031: "INI-FAULT",
	0x1032: "OV-TEM",
	0x1033: "PV ISO-PRO",
	0x1034: "ILeak-PRO",
	0x1035: "RelayChk-FAIL",
	0x1036: "DSP-B-FAULT",
	0x1037: "DCInj-FAULT",
	0x1038: "12Power-FAULT",
	0x1039: "ILeak-Check",
	0x103A: "UN-TEM",
	0x1040: "AFCI-Check",
	0x1041: "ARC-FAULT",
	0x1042: "RAM-FAULT",
	0x1043: "FLASH-FAULT",
	0x1044: "PC-FAULT",
	0x1045: "REG-FAULT",
	0x1046: "GRID-INTF02",
	0x1047: "IG-AD",
	0x1048: "IGBT-OV-I",
	0x1050: "OV-IgTr",
	0x1051: "OV-Vbatt-H",
	0x1052: "OV-ILLC",
	0x1053: "OV-Vbatt",
	0x1054: "UN-Vbatt",
	0x1055: "NO-Battery",
	0x1056: "OV-VBackup",
	0x1057: "Over-Load",
	0x1058: "DspSelfChk",
	// Warning states
	0x2010: "Fail Safe",
	0x2011: "MET_Comm_FAIL",
	0x2012: "CAN_Comm_FAIL",
	0x2014: "DSP_Comm_FAIL",
	0x2015: "Alarm-BMS",
	0x2016: "BatName-FAIL",
	0x2017: "Alarm2-BMS",
	0x2018: "DRM_LINK_FAIL",
	0x2019: "MET_SEL_FAIL",
	0x2020: "HighTemp.AMB",
	0x2021: "LowTemp.AMB",
	// Alarm states
	0xF010: "Surge Alarm",
	0xF011: "Fan Alarm",
}

// STATUS_DESCRIPTION maps raw status code values to detailed human-readable descriptions.
// This provides more context than STATUS_MAP for API responses and logging.
var STATUS_DESCRIPTION = map[uint16]string{
	// Normal operation states
	0x0000: "Normal operation / Waiting",
	0x0001: "Open operating",
	0x0002: "Soft run / Waiting",
	0x0003: "Initializing / Generating",
	0x0004: "Standby",
	0x0005: "Standby synchronize",
	0x0006: "Grid to load",
	0x000F: "Normal running",
	// Fault states
	0x1004: "Grid off",
	0x1010: "Grid overvoltage fault",
	0x1011: "Grid undervoltage fault",
	0x1012: "Grid over-frequency fault",
	0x1013: "Grid under-frequency fault",
	0x1014: "Over grid impedance / Grid reverse current",
	0x1015: "No grid detected",
	0x1016: "Unbalanced grid (phase fault)",
	0x1017: "Grid frequency fluctuation",
	0x1018: "Grid overcurrent",
	0x1019: "Grid current sampling error",
	0x1020: "DC overvoltage",
	0x1021: "DC bus overvoltage",
	0x1022: "DC bus unbalanced voltage",
	0x1023: "DC bus undervoltage",
	0x1024: "DC bus unbalanced voltage 2",
	0x1025: "DC channel A overcurrent",
	0x1026: "DC channel B overcurrent",
	0x1027: "DC input interference",
	0x1028: "DC reverse connection",
	0x1029: "PV midpoint grounding fault",
	0x1030: "Grid interference protection",
	0x1031: "DSP initial protection",
	0x1032: "Over temperature protection",
	0x1033: "PV insulation fault",
	0x1034: "Leakage current protection",
	0x1035: "Relay check protection",
	0x1036: "DSP_B protection",
	0x1037: "DC injection protection",
	0x1038: "12V undervoltage fault",
	0x1039: "Leakage current self-check protection",
	0x103A: "Under temperature protection",
	0x1040: "AFCI check fault",
	0x1041: "AFCI arc fault",
	0x1042: "DSP SRAM fault",
	0x1043: "DSP FLASH fault",
	0x1044: "DSP PC pointer fault",
	0x1045: "DSP register fault",
	0x1046: "Grid interference 02 protection",
	0x1047: "Grid current sampling error (AD)",
	0x1048: "IGBT overcurrent",
	0x1050: "Grid transient overcurrent",
	0x1051: "Battery hardware overvoltage fault",
	0x1052: "LLC hardware overcurrent",
	0x1053: "Battery overvoltage",
	0x1054: "Battery undervoltage",
	0x1055: "Battery not connected",
	0x1056: "Backup overvoltage",
	0x1057: "Backup overload",
	0x1058: "DSP self-check error",
	// Warning states
	0x2010: "Fail safe activated",
	0x2011: "Meter communication fail",
	0x2012: "Battery (CAN) communication fail",
	0x2014: "DSP communication fail",
	0x2015: "BMS alarm",
	0x2016: "Battery model mismatch",
	0x2017: "BMS alarm 2",
	0x2018: "DRM connection fail",
	0x2019: "Meter selection fail",
	0x2020: "Lead-acid battery high ambient temperature",
	0x2021: "Lead-acid battery low ambient temperature",
	// Alarm states
	0xF010: "Grid surge warning",
	0xF011: "Fan fault warning",
}

// FAULT_REGISTER_NAMES maps fault register addresses to human-readable names.
var FAULT_REGISTER_NAMES = map[uint16]string{
	33116: "grid",
	33117: "backup",
	33118: "battery",
	33119: "device04",
	33120: "device05",
}

// FAULT_BIT_MAP maps fault register addresses to their bit field descriptions.
// Each bit position (0-15) corresponds to a specific fault condition.
// An empty string ("") means that bit position has no defined meaning.
var FAULT_BIT_MAP = map[uint16][]string{
	// Register 33116 - Grid fault status 01
	33116: {
		"No grid",                     // BIT00
		"Grid overvoltage",            // BIT01
		"Grid undervoltage",           // BIT02
		"Grid over-frequency",         // BIT03
		"Grid under-frequency",        // BIT04
		"Unbalanced grid",             // BIT05
		"Grid frequency fluctuation",  // BIT06
		"Grid reverse current",        // BIT07
		"Grid current tracking error", // BIT08
		"Meter COM fail",              // BIT09
		"Fail safe",                   // BIT10
		"", "", "", "", "",            // BIT11-15 reserved
	},
	// Register 33117 - Backup load fault status 02
	33117: {
		"Backup overvoltage fault",                                 // BIT00
		"Backup overload fault",                                    // BIT01
		"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", // BIT02-15 reserved
	},
	// Register 33118 - Battery fault status 03
	33118: {
		"Battery not connected",                                // BIT00
		"Battery overvoltage check",                            // BIT01
		"Battery undervoltage check",                           // BIT02
		"", "", "", "", "", "", "", "", "", "", "", "", "", "", // BIT03-15 reserved
	},
	// Register 33119 - Device fault status 04
	33119: {
		"DC overvoltage",              // BIT00
		"DC bus overvoltage",          // BIT01
		"DC bus unbalanced voltage",   // BIT02
		"DC bus undervoltage",         // BIT03
		"DC bus unbalanced voltage 2", // BIT04
		"DC overcurrent A circuit",    // BIT05
		"DC overcurrent B circuit",    // BIT06
		"DC input interference",       // BIT07
		"Grid overcurrent",            // BIT08
		"IGBT overcurrent",            // BIT09
		"Grid interference 02",        // BIT10
		"AFCI self-check",             // BIT11
		"Arc fault (reserved)",        // BIT12
		"Grid current sampling fault", // BIT13
		"DSP self-check error",        // BIT14
		"",                            // BIT15 reserved
	},
	// Register 33120 - Device fault status 05
	33120: {
		"Grid interference",                  // BIT00
		"Over DC components",                 // BIT01
		"Over temperature protection",        // BIT02
		"Relay check protection",             // BIT03
		"Under temperature protection",       // BIT04
		"PV insulation fault",                // BIT05
		"12V undervoltage protection",        // BIT06
		"Leak current protection",            // BIT07
		"Leak current self-check",            // BIT08
		"DSP initial protection",             // BIT09
		"DSP_B protection",                   // BIT10
		"Battery overvoltage hardware fault", // BIT11
		"LLC hardware overcurrent",           // BIT12
		"Grid transient overcurrent",         // BIT13
		"CAN COM fail",                       // BIT14
		"DSP COM fail",                       // BIT15
	},
	// Register 33145 - Battery fault status 1 (BMS)
	33145: {
		"Battery 1 overvoltage",           // BIT00
		"Battery 1 undervoltage",          // BIT01
		"Battery 1 overcurrent charge",    // BIT02
		"Battery 1 overcurrent discharge", // BIT03
		"Battery 1 overtemperature",       // BIT04
		"Battery 1 undertemperature",      // BIT05
		"Battery 1 communication fault",   // BIT06
		"Battery 1 internal fault",        // BIT07
		"", "", "", "", "", "", "", "",    // BIT08-15 reserved
	},
	// Register 33146 - Battery fault status 2 (BMS)
	33146: {
		"Battery 2 overvoltage",           // BIT00
		"Battery 2 undervoltage",          // BIT01
		"Battery 2 overcurrent charge",    // BIT02
		"Battery 2 overcurrent discharge", // BIT03
		"Battery 2 overtemperature",       // BIT04
		"Battery 2 undertemperature",      // BIT05
		"Battery 2 communication fault",   // BIT06
		"Battery 2 internal fault",        // BIT07
		"", "", "", "", "", "", "", "",    // BIT08-15 reserved
	},
}

// OP_STATUS_BIT_MAP maps bit positions in the Operating Status register (33121)
// to human-readable descriptions. An empty string ("") means reserved/unknown.
var OP_STATUS_BIT_MAP = []string{
	"Normal operation",              // BIT00
	"Initializing",                  // BIT01
	"Controlled turn-off",           // BIT02
	"Fault turn-off",                // BIT03
	"Stand-by",                      // BIT04
	"Limited operation (temp/freq)", // BIT05
	"Limited operation (external)",  // BIT06
	"Backup overload",               // BIT07
	"Load fault",                    // BIT08
	"Grid fault",                    // BIT09
	"Battery fault",                 // BIT10
	"",                              // BIT11 reserved
	"Grid surge warning",            // BIT12
	"Fan fault warning",             // BIT13
	"", "",                          // BIT14-15 reserved
}

// DecodeFaultBits returns a slice of active fault descriptions for a given
// fault register value. Each bit that is set (1) corresponds to a fault condition.
func DecodeFaultBits(addr uint16, value uint16) []string {
	bitMap, ok := FAULT_BIT_MAP[addr]
	if !ok {
		return []string{fmt.Sprintf("No bit map defined for register %d", addr)}
	}

	var faults []string
	for i := range 16 {
		if value&(1<<i) != 0 {
			if i < len(bitMap) && bitMap[i] != "" {
				faults = append(faults, bitMap[i])
			} else {
				faults = append(faults, fmt.Sprintf("Unknown bit %d", i))
			}
		}
	}
	return faults
}

// Register filtering removed per v2 plan - all registers are always enabled
