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

// DailyRegisterKeys are the register keys that should have daily aggregation.
// These are energy registers that accumulate during the day and reset at midnight.
var DailyRegisterKeys = []string{
	"household_load_today_energy",
	"today_energy_consumption",
	"today_energy_fed_into_grid",
	"today_energy_imported_from_grid",
	"today_battery_discharge_energy",
	"today_battery_charge_energy",
	"pv_today_energy",
	"backup_load_today_energy",
}

// dailyRegisterSet provides O(1) lookup for daily registers.
var dailyRegisterSet = map[string]bool{
	"household_load_today_energy":     true,
	"today_energy_consumption":        true,
	"today_energy_fed_into_grid":      true,
	"today_energy_imported_from_grid": true,
	"today_battery_discharge_energy":  true,
	"today_battery_charge_energy":     true,
	"today_grid_energy":               true,
	"pv_today_energy":                 true,
	"backup_load_today_energy":        true,
}

// IsDailyRegister returns true if the key is a daily energy register.
func IsDailyRegister(key string) bool {
	return dailyRegisterSet[key]
}

// MonthlyRegisterKeys are the register keys that should have monthly aggregation.
// These are energy registers that accumulate during the month and reset at the start of a new month.
var MonthlyRegisterKeys = []string{
	"pv_month_energy",
	"household_load_month_energy",
	"backup_load_month_energy",
	// Computed monthly registers
	"energy_consumption_month_energy",
	"energy_fed_into_grid_month_energy",
	"energy_imported_from_grid_month_energy",
	"battery_discharge_month_energy",
	"battery_charge_month_energy",
	"month_grid_energy",
}

// monthlyRegisterSet provides O(1) lookup for monthly registers.
var monthlyRegisterSet = map[string]bool{
	"pv_month_energy":                 true,
	"household_load_month_energy":     true,
	"backup_load_month_energy":        true,
	// Computed monthly registers
	"energy_consumption_month_energy":     true,
	"energy_fed_into_grid_month_energy":    true,
	"energy_imported_from_grid_month_energy": true,
	"battery_discharge_month_energy":      true,
	"battery_charge_month_energy":         true,
	"month_grid_energy":                    true,
}

// IsMonthlyRegister returns true if the key is a monthly energy register.
func IsMonthlyRegister(key string) bool {
	return monthlyRegisterSet[key]
}

// YearlyRegisterKeys are the register keys that should have yearly aggregation.
// These are energy registers that accumulate during the year and reset at the start of a new year.
var YearlyRegisterKeys = []string{
	"pv_year_energy",
	"household_load_year_energy",
	"backup_load_year_energy",
	// Computed yearly registers
	"energy_consumption_year_energy",
	"energy_fed_into_grid_year_energy",
	"energy_imported_from_grid_year_energy",
	"battery_discharge_year_energy",
	"battery_charge_year_energy",
	"year_grid_energy",
}

// yearlyRegisterSet provides O(1) lookup for yearly registers.
var yearlyRegisterSet = map[string]bool{
	"pv_year_energy":                 true,
	"household_load_year_energy":     true,
	"backup_load_year_energy":        true,
	// Computed yearly registers
	"energy_consumption_year_energy":     true,
	"energy_fed_into_grid_year_energy":    true,
	"energy_imported_from_grid_year_energy": true,
	"battery_discharge_year_energy":      true,
	"battery_charge_year_energy":         true,
	"year_grid_energy":                   true,
}

// IsYearlyRegister returns true if the key is a yearly energy register.
func IsYearlyRegister(key string) bool {
	return yearlyRegisterSet[key]
}

// TotalRegisterKeys are the register keys that should have total aggregation.
// These are energy registers that accumulate indefinitely (lifetime totals).
var TotalRegisterKeys = []string{
	"pv_total_energy",
	"total_battery_discharge_energy",
	"total_battery_charge_energy",
	"total_energy_imported_from_grid",
	"total_energy_fed_into_grid",
	"total_energy_consumption",
	"household_load_total_energy",
	"backup_load_total_energy",
}

// totalRegisterSet provides O(1) lookup for total registers.
var totalRegisterSet = map[string]bool{
	"pv_total_energy":                 true,
	"total_battery_discharge_energy":  true,
	"total_battery_charge_energy":     true,
	"total_energy_imported_from_grid": true,
	"total_energy_fed_into_grid":      true,
	"total_energy_consumption":        true,
	"total_grid_energy":               true,
	"household_load_total_energy":     true,
	"backup_load_total_energy":        true,
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
		// Stable: Written once at startup
		// =====================================================================

		{
			Key:       "solis_model_no",
			Name:      "Solis Model No",
			Address:   33000,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Stable,
		},
		{
			Key:       "solis_dsp_version",
			Name:      "Solis DSP Version",
			Address:   33001,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Stable,
		},
		{
			Key:       "solis_hmi_version",
			Name:      "Solis HMI Version",
			Address:   33002,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Stable,
		},
		{
			Key:       "solis_protocol_version",
			Name:      "Solis Protocol Version",
			Address:   33003,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Stable,
		},
		{
			Key:       "solis_serial_number",
			Name:      "Solis Serial Number",
			Address:   33004,
			Count:     16, // 16 registers = 32 bytes for ASCII string
			DataType:  String,
			Scale:     1.0,
			Unit:      "",
			Stability: Stable,
		},

		// =====================================================================
		// ENERGY REGISTERS (33029-33048)
		// =====================================================================

		{
			Key:       "pv_today_energy",
			Name:      "Solis PV Today Energy Generation",
			Address:   33035,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "pv_month_energy",
			Name:      "Solis PV Current Month Energy Generation",
			Address:   33031,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "pv_year_energy",
			Name:      "Solis PV This Year Energy Generation",
			Address:   33037,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "pv_total_energy",
			Name:      "Solis PV Total Energy Generation",
			Address:   33029,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		// =====================================================================
		// PV VOLTAGE/CURRENT REGISTERS (33049-33056)
		// =====================================================================

		{
			Key:       "pv_voltage_1",
			Name:      "Solis PV Voltage 1",
			Address:   33049,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "pv_current_1",
			Name:      "Solis PV Current 1",
			Address:   33050,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "pv_voltage_2",
			Name:      "Solis PV Voltage 2",
			Address:   33051,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "pv_current_2",
			Name:      "Solis PV Current 2",
			Address:   33052,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "pv_voltage_3",
			Name:      "Solis PV Voltage 3",
			Address:   33053,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "pv_current_3",
			Name:      "Solis PV Current 3",
			Address:   33054,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "pv_voltage_4",
			Name:      "Solis PV Voltage 4",
			Address:   33055,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "pv_current_4",
			Name:      "Solis PV Current 4",
			Address:   33056,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},

		// =====================================================================
		// PV POWER AND BUS REGISTERS (33057-33072)
		// =====================================================================

		{
			Key:       "total_pv_power",
			Name:      "Solis Total PV Power",
			Address:   33057,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},

		// =====================================================================
		// GRID VOLTAGE/CURRENT REGISTERS (33073-33084)
		// =====================================================================

		{
			Key:       "a_phase_voltage",
			Name:      "Solis A Phase Voltage",
			Address:   33073,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "b_phase_voltage",
			Name:      "Solis B Phase Voltage",
			Address:   33074,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "c_phase_voltage",
			Name:      "Solis C Phase Voltage",
			Address:   33075,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "a_phase_current",
			Name:      "Solis A Phase Current",
			Address:   33076,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "b_phase_current",
			Name:      "Solis B Phase Current",
			Address:   33077,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "c_phase_current",
			Name:      "Solis C Phase Current",
			Address:   33078,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "active_power",
			Name:      "Solis Active Power",
			Address:   33079,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},
		{
			Key:       "reactive_power",
			Name:      "Solis Reactive Power",
			Address:   33081,
			Count:     2,
			DataType:  Int32,
			Scale:     0.1,
			Unit:      "VAR",
			Stability: Dynamic,
		},
		{
			Key:       "apparent_power",
			Name:      "Solis Apparent Power",
			Address:   33083,
			Count:     2,
			DataType:  Uint32,
			Scale:     0.1,
			Unit:      "VA",
			Stability: Dynamic,
		},
		{
			Key:       "grid_frequency",
			Name:      "Solis Grid Frequency",
			Address:   33094,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.01,
			Unit:      "HZ",
			Stability: Dynamic,
		},

		// =====================================================================
		// TEMPERATURE AND STATUS REGISTERS (33093-33096)
		// =====================================================================

		{
			Key:       "temperature",
			Name:      "Solis Temperature",
			Address:   33093,
			Count:     1,
			DataType:  Int16,
			Scale:     0.1,
			Unit:      "C",
			Stability: Dynamic,
		},
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
			Key:       "grid_fault_status_01",
			Name:      "Solis Grid Fault Status 01 (Bitmask)",
			Address:   33116,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "backup_load_fault_status_02",
			Name:      "Solis Backup Load Fault Status 02 (Bitmask)",
			Address:   33117,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "battery_fault_status_03",
			Name:      "Solis Battery Fault Status 03 (Bitmask)",
			Address:   33118,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "device_fault_status_04",
			Name:      "Solis Device Fault Status 04 (Bitmask)",
			Address:   33119,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "device_fault_status_05",
			Name:      "Solis Device Fault Status 05 (Bitmask)",
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

		{
			Key:       "battery_voltage",
			Name:      "Solis Battery Voltage",
			Address:   33133,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "battery_current",
			Name:      "Solis Battery Current",
			Address:   33134,
			Count:     1,
			DataType:  Int16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "battery_current_direction",
			Name:      "Solis Battery Current Direction",
			Address:   33135,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
		},

		{
			Key:       "backup_ac_voltage_phase_a",
			Name:      "Solis Backup AC Voltage Phase A",
			Address:   33137,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "backup_ac_current_phase_a",
			Name:      "Solis Backup AC Current Phase A",
			Address:   33138,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "battery_soc",
			Name:      "Solis Battery SOC",
			Address:   33139,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "%",
			Stability: Dynamic,
		},
		{
			Key:       "battery_soh",
			Name:      "Solis Battery SOH",
			Address:   33140,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "%",
			Stability: Dynamic,
		},
		{
			Key:       "battery_voltage_bms",
			Name:      "Solis Battery Voltage (BMS)",
			Address:   33141,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.01,
			Unit:      "V",
			Stability: Dynamic,
		},

		{
			Key:       "battery_fault_status_1_bms",
			Name:      "Solis Battery Fault Status 1 (BMS)",
			Address:   33145,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "battery_fault_status_2_bms",
			Name:      "Solis Battery Fault Status 2 (BMS)",
			Address:   33146,
			Count:     1,
			DataType:  Uint16,
			Scale:     1.0,
			Unit:      "",
			Stability: Dynamic,
			Status:    true,
		},
		{
			Key:       "household_load_power",
			Name:      "Solis Household Load Power",
			Address:   33147,
			Count:     1,
			DataType:  Uint16,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},
		{
			Key:       "backup_load_power",
			Name:      "Solis Backup Load Power",
			Address:   33148,
			Count:     1,
			DataType:  Uint16,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},
		{
			Key:       "battery_power",
			Name:      "Solis Battery Power",
			Address:   33149,
			Count:     2,
			DataType:  Int32,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},
		{
			Key:       "ac_grid_port_power",
			Name:      "Solis AC Grid Port Power",
			Address:   33151,
			Count:     2,
			DataType:  Int32,
			Scale:     0.1,
			Unit:      "W",
			Stability: Dynamic,
		},

		{
			Key:       "today_battery_charge_energy",
			Name:      "Solis Today Battery Charge Energy",
			Address:   33163,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "today_battery_discharge_energy",
			Name:      "Solis Today Battery Discharge Energy",
			Address:   33167,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "total_battery_discharge_energy",
			Name:      "Solis Total Battery Discharge Energy",
			Address:   33165,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "total_battery_charge_energy",
			Name:      "Solis Total Battery Charge Energy",
			Address:   33161,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "today_energy_imported_from_grid",
			Name:      "Solis Today Energy Imported From Grid",
			Address:   33171,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "total_energy_imported_from_grid",
			Name:      "Solis Total Energy Imported From Grid",
			Address:   33169,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "total_energy_fed_into_grid",
			Name:      "Solis Total Energy Fed Into Grid",
			Address:   33173,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "today_energy_fed_into_grid",
			Name:      "Solis Today Energy Fed Into Grid",
			Address:   33175,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "today_energy_consumption",
			Name:      "Solis Today Energy Consumption",
			Address:   33179,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "total_energy_consumption",
			Name:      "Solis Total Energy Consumption",
			Address:   33177,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		// Computed net grid energy registers (virtual - no Modbus address)
		{
			Key:       "total_grid_energy",
			Name:      "Total Grid Energy (Net)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "today_grid_energy",
			Name:      "Today Grid Energy (Net)",
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
			Key:       "energy_consumption_month_energy",
			Name:      "Energy Consumption Month Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "energy_fed_into_grid_month_energy",
			Name:      "Energy Fed Into Grid Month Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "energy_imported_from_grid_month_energy",
			Name:      "Energy Imported From Grid Month Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_discharge_month_energy",
			Name:      "Battery Discharge Month Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_charge_month_energy",
			Name:      "Battery Charge Month Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "month_grid_energy",
			Name:      "Month Grid Energy (Net, Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "energy_consumption_year_energy",
			Name:      "Energy Consumption Year Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "energy_fed_into_grid_year_energy",
			Name:      "Energy Fed Into Grid Year Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "energy_imported_from_grid_year_energy",
			Name:      "Energy Imported From Grid Year Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_discharge_year_energy",
			Name:      "Battery Discharge Year Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "battery_charge_year_energy",
			Name:      "Battery Charge Year Energy (Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},
		{
			Key:       "year_grid_energy",
			Name:      "Year Grid Energy (Net, Computed)",
			Address:   0,
			Count:     0,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_load_today_energy",
			Name:      "Solis Household Load Today Energy",
			Address:   33586,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_load_year_energy",
			Name:      "Solis Household Load Year Energy",
			Address:   33582,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_load_month_energy",
			Name:      "Solis Household Load Month Energy",
			Address:   33584,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "household_load_total_energy",
			Name:      "Solis Household Load Total Energy",
			Address:   33580,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_load_today_energy",
			Name:      "Solis Backup Load Today Energy",
			Address:   33596,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_load_total_energy",
			Name:      "Solis Backup Load Total Energy",
			Address:   33590,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_load_year_energy",
			Name:      "Solis Backup Load Year Energy",
			Address:   33592,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "backup_load_month_energy",
			Name:      "Solis Backup Load Month Energy",
			Address:   33594,
			Count:     2,
			DataType:  Uint32,
			Scale:     1,
			Unit:      "kWh",
			Stability: Dynamic,
		},

		{
			Key:       "meter_ac_voltage_a",
			Name:      "Solis Meter AC Voltage A",
			Address:   33251,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "meter_ac_current_a",
			Name:      "Solis Meter AC Current A",
			Address:   33252,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.01,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "meter_ac_voltage_b",
			Name:      "Solis Meter AC Voltage B",
			Address:   33253,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "meter_ac_current_b",
			Name:      "Solis Meter AC Current B",
			Address:   33254,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.01,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "meter_ac_voltage_c",
			Name:      "Solis Meter AC Voltage C",
			Address:   33255,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.1,
			Unit:      "V",
			Stability: Dynamic,
		},
		{
			Key:       "meter_ac_current_c",
			Name:      "Solis Meter AC Current C",
			Address:   33256,
			Count:     1,
			DataType:  Uint16,
			Scale:     0.01,
			Unit:      "A",
			Stability: Dynamic,
		},
		{
			Key:       "meter_active_power_a",
			Name:      "Solis Meter Active Power A",
			Address:   33257,
			Count:     2,
			DataType:  Int32,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},
		{
			Key:       "meter_active_power_b",
			Name:      "Solis Meter Active Power B",
			Address:   33259,
			Count:     2,
			DataType:  Int32,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},
		{
			Key:       "meter_active_power_c",
			Name:      "Solis Meter Active Power C",
			Address:   33261,
			Count:     2,
			DataType:  Int32,
			Scale:     1,
			Unit:      "W",
			Stability: Dynamic,
		},
		{
			Key:       "meter_total_active_power",
			Name:      "Solis Meter Total Active Power",
			Address:   33263,
			Count:     2,
			DataType:  Int32,
			Scale:     1.0,
			Unit:      "W",
			Stability: Dynamic,
		},
	}
}

// =============================================================================
// READ RANGES
// These are the 4 contiguous blocks we read from the inverter in one poll cycle.
// Each range is read as a single Modbus ReadInputRegisters call.
// =============================================================================

// ReadRanges defines the 4 ranges to read sequentially from the inverter.
// Each entry contains the starting address and number of registers to read.
var ReadRanges = [4]struct {
	StartAddr uint16
	Count     uint16
}{
	{StartAddr: 33000, Count: 97}, // 33000-33096 (inclusive) - includes model, versions, serial
	{StartAddr: 33116, Count: 65}, // 33116-33180 (inclusive)
	{StartAddr: 33251, Count: 14}, // 33251-33264 (inclusive)
	{StartAddr: 33580, Count: 17}, // 33580-33596 (inclusive)
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

// ==========================================================================================================================================================
// REGISTER FILTER
// Helper for filtering registers by disabled keys from configuration
// =============================================================================

// RegisterFilter provides helper functions for filtering registers by disabled keys.
// It uses a map for O(1) lookup performance.
type RegisterFilter struct {
	// disabledSet is a map of disabled register keys for O(1) lookup
	disabledSet map[string]bool
}

// NewRegisterFilter creates a new RegisterFilter with the given disabled keys.
// If disabledKeys is nil or empty, all registers are considered enabled.
func NewRegisterFilter(disabledKeys []string) *RegisterFilter {
	disabledSet := make(map[string]bool, len(disabledKeys))
	for _, k := range disabledKeys {
		disabledSet[k] = true
	}
	return &RegisterFilter{disabledSet: disabledSet}
}

// IsEnabled returns true if the register key is not disabled.
// Returns true if the filter has no disabled keys or if the key is not in the disabled set.
func (rf *RegisterFilter) IsEnabled(key string) bool {
	if rf == nil {
		return true
	}
	return !rf.disabledSet[key]
}

// FilterRegisters returns a slice of registers with disabled keys removed.
func (rf *RegisterFilter) FilterRegisters(registers []Register) []Register {
	if rf == nil {
		return registers
	}
	result := make([]Register, 0, len(registers))
	for _, reg := range registers {
		if rf.IsEnabled(reg.Key) {
			result = append(result, reg)
		}
	}
	return result
}

// FilterMapByKey returns a filtered copy of RegisterMapByKey with disabled keys removed.
func (rf *RegisterFilter) FilterMapByKey() map[string]*Register {
	if rf == nil {
		return RegisterMapByKey
	}
	result := make(map[string]*Register)
	for k, reg := range RegisterMapByKey {
		if rf.IsEnabled(k) {
			result[k] = reg
		}
	}
	return result
}

// FilterKeys returns all register keys with disabled keys removed.
func (rf *RegisterFilter) FilterKeys() []string {
	if rf == nil {
		keys := make([]string, 0, len(RegisterMapByKey))
		for k := range RegisterMapByKey {
			keys = append(keys, k)
		}
		return keys
	}
	keys := make([]string, 0, len(RegisterMapByKey))
	for k := range RegisterMapByKey {
		if rf.IsEnabled(k) {
			keys = append(keys, k)
		}
	}
	return keys
}
