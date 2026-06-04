// Package solis provides Solis inverter register definitions, status maps,
// and utilities for decoding Modbus register data.
package solis

import "fmt"

// =============================================================================
// INDIVIDUAL STATUS DECODERS
// Each status/fault register has its own dedicated decoder function.
// These use the maps defined in registers.go for consistency.
// =============================================================================

// DecodeSolisStatus decodes the solis_status register (33095).
func DecodeSolisStatus(value uint16) map[string]string {
	name, ok := STATUS_MAP[value]
	if !ok {
		logger.Debug().Msgf("Unknown status code: 0x%04X", value)
		name = fmt.Sprintf("Unknown Status (0x%04X)", value)
	}
	desc, ok := STATUS_DESCRIPTION[value]
	if !ok {
		logger.Debug().Msgf("Unknown status code description: 0x%04X", value)
		desc = fmt.Sprintf("Unknown status code: 0x%04X", value)
	}
	return map[string]string{"name": name, "description": desc}
}

// DecodeOperatingStatus decodes the operating_status register (33121).
func DecodeOperatingStatus(value uint16) []string {
	var statuses []string
	for i := range 16 {
		if value&(1<<i) != 0 {
			if i < len(OP_STATUS_BIT_MAP) && OP_STATUS_BIT_MAP[i] != "" {
				statuses = append(statuses, OP_STATUS_BIT_MAP[i])
			} else if i < len(OP_STATUS_BIT_MAP) {
				statuses = append(statuses, fmt.Sprintf("Unknown status bit %d", i))
			}
		}
	}
	return statuses
}

// DecodeGridFaultStatus01 decodes the grid_fault_status_01 register (33116).
func DecodeGridFaultStatus01(value uint16) []string {
	bitMap := FAULT_BIT_MAP[33116]
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

// DecodeBackupFaultStatus02 decodes the backup_load_fault_status_02 register (33117).
func DecodeBackupFaultStatus02(value uint16) []string {
	bitMap := FAULT_BIT_MAP[33117]
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

// DecodeBatteryFaultStatus03 decodes the battery_fault_status_03 register (33118).
func DecodeBatteryFaultStatus03(value uint16) []string {
	bitMap := FAULT_BIT_MAP[33118]
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

// DecodeDeviceFaultStatus04 decodes the device_fault_status_04 register (33119).
func DecodeDeviceFaultStatus04(value uint16) []string {
	bitMap := FAULT_BIT_MAP[33119]
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

// DecodeDeviceFaultStatus05 decodes the device_fault_status_05 register (33120).
func DecodeDeviceFaultStatus05(value uint16) []string {
	bitMap := FAULT_BIT_MAP[33120]
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

// DecodeBatteryFaultStatus1Bms decodes the battery_fault_status_1_bms register (33145).
func DecodeBatteryFaultStatus1Bms(value uint16) []string {
	var faults []string
	for i := range 16 {
		if value&(1<<i) != 0 {
			faults = append(faults, fmt.Sprintf("BMS fault bit %d", i))
		}
	}
	return faults
}

// DecodeBatteryFaultStatus2Bms decodes the battery_fault_status_2_bms register (33146).
func DecodeBatteryFaultStatus2Bms(value uint16) []string {
	var faults []string
	for i := range 16 {
		if value&(1<<i) != 0 {
			faults = append(faults, fmt.Sprintf("BMS fault bit %d", i))
		}
	}
	return faults
}
