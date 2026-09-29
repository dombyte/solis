package solis

import "github.com/dombyte/solis/internal/period"

const (
	unitKWh  = "kWh"
	unitW    = "W"
	scaleOne = 1.0
	scaleTen = 0.1
)

// energyKind names one energy series that exists at daily/monthly/yearly/total level.
type energyKind struct {
	prefix string // key prefix, e.g. "pv_energy"
	name   string // display prefix, e.g. "PV Energy"
	daily  uint16 // daily register address
	suffix string // name suffix of computed levels that were v2-computed
}

// energyKinds lists the eight polled daily series; every one gets computed
// monthly, yearly and total registers.
func energyKinds() []energyKind {
	const computed = " (Computed)"
	return []energyKind{
		{"pv_energy", "PV Energy", 33035, ""},
		{"battery_charge", "Battery Charge", 33163, computed},
		{"battery_discharge", "Battery Discharge", 33167, computed},
		{"grid_import", "Grid Import", 33171, computed},
		{"grid_export", "Grid Export", 33175, computed},
		{"energy_consumption", "Energy Consumption", 33179, computed},
		{"household_energy", "Household Energy", 33586, ""},
		{"backup_energy", "Backup Energy", 33596, ""},
	}
}

// levelSuffix maps a level to its key and display suffix.
func levelSuffix(l period.Level) (key, name string) {
	switch l {
	case period.Daily:
		return "_daily", " Daily"
	case period.Monthly:
		return "_monthly", " Monthly"
	case period.Yearly:
		return "_yearly", " Yearly"
	default:
		return "_total", " Total"
	}
}

// computedLevels are the levels the aggregator produces from daily rows.
func computedLevels() []period.Level {
	return []period.Level{period.Monthly, period.Yearly, period.Total}
}

// energyRegisters builds the daily (polled) and computed energy registers plus the
// daily → computed edges.
func energyRegisters() ([]Register, []Edge) {
	var regs []Register
	var edges []Edge
	for _, k := range energyKinds() {
		dKey, dName := levelSuffix(period.Daily)
		regs = append(regs, Register{
			Key: k.prefix + dKey, Name: k.name + dName, Address: k.daily,
			DataType: Uint16, Scale: scaleTen, Unit: unitKWh, Store: StoreDaily,
		})
		for _, l := range computedLevels() {
			key, name := levelSuffix(l)
			suffix := k.suffix
			if l == period.Total {
				suffix = ""
			}
			regs = append(regs, Register{
				Key: k.prefix + key, Name: k.name + name + suffix, DataType: Uint32,
				Scale: scaleOne, Unit: unitKWh, Store: StoreForLevel(l),
			})
			edges = append(edges, Edge{Level: l, Source: k.prefix + dKey, Target: k.prefix + key})
		}
	}
	return regs, edges
}

// netRegisters builds grid_energy_{daily,monthly,yearly,total} = export - import.
func netRegisters() ([]Register, []NetPair) {
	levels := []period.Level{period.Daily, period.Monthly, period.Yearly, period.Total}
	var regs []Register
	var pairs []NetPair
	for _, l := range levels {
		key, name := levelSuffix(l)
		dt := Uint32
		label := " (Net, Computed)"
		if l == period.Daily || l == period.Total {
			label = " (Net)"
		}
		if l == period.Daily {
			dt = Uint16
		}
		regs = append(regs, Register{
			Key: "grid_energy" + key, Name: "Grid Energy" + name + label, DataType: dt,
			Scale: scaleOne, Unit: unitKWh, Store: StoreForLevel(l), Net: true,
		})
		pairs = append(pairs, NetPair{
			Level: l, Export: "grid_export" + key, Import: "grid_import" + key,
			Target: "grid_energy" + key,
		})
	}
	return regs, pairs
}

// statusRegisters are written to error_data on change.
func statusRegisters() []Register {
	status := func(key, name string, addr uint16) Register {
		return Register{
			Key: key, Name: name, Address: addr, DataType: Uint16,
			Scale: scaleOne, Store: StoreStatus,
		}
	}
	return []Register{
		status("solis_status", "Solis Status", 33095),
		status("grid_fault_1", "Grid Fault 1 (Bitmask)", 33116),
		status("backup_fault_2", "Backup Fault 2 (Bitmask)", 33117),
		status("battery_fault_3", "Battery Fault 3 (Bitmask)", 33118),
		status("device_fault_4", "Device Fault 4 (Bitmask)", 33119),
		status("device_fault_5", "Device Fault 5 (Bitmask)", 33120),
		status("operating_status", "Solis Operating Status (Bitmask)", 33121),
		status("battery_fault_1_bms", "Battery Fault 1 (BMS)", 33145),
		status("battery_fault_2_bms", "Battery Fault 2 (BMS)", 33146),
	}
}

// Live power register keys used by DeriveValues and the dashboard flow diagram.
const (
	KeyBatteryPower       = "battery_power"
	KeyBatteryDirection   = "battery_current_direction"
	KeyBatteryPowerSigned = "battery_power_signed"
)

// liveRegisters are polled, cache-only values for the flow diagram. grid_power
// uses the alternate address 33130 which falls inside the fault/BMS block.
func liveRegisters() []Register {
	live := func(key, name string, addr uint16, dt DataType, unit string) Register {
		return Register{
			Key: key, Name: name, Address: addr, DataType: dt, Scale: scaleOne,
			Unit: unit, Store: StoreNone,
		}
	}
	return []Register{
		live("pv_total_power", "PV Total Power", 33057, Uint32, unitW),
		live("grid_power", "Grid Power", 33130, Int32, unitW),
		live(KeyBatteryDirection, "Battery Current Direction", 33135, Uint16, ""),
		live("battery_soc", "Battery SOC", 33139, Uint16, "%"),
		live("household_load_power", "Household Load Power", 33147, Uint16, unitW),
		live("backup_load_power", "Backup Load Power", 33148, Uint16, unitW),
		live(KeyBatteryPower, "Battery Power", 33149, Uint32, unitW),
		// Derived in Decoder.Derive: charge (+) / discharge (-).
		{
			Key: KeyBatteryPowerSigned, Name: "Battery Power (Signed)", DataType: Int32,
			Scale: scaleOne, Unit: unitW, Store: StoreNone,
		},
	}
}
