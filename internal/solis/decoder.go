// Package solis provides Solis inverter register definitions, status maps,
// and utilities for decoding Modbus register data.
package solis

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"time"

	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/utils"
)

// decoderLogger is the package-level logger for decoder operations.
var decoderLogger = logging.NewComponentLogger("solis.decoder")

// Value represents a decoded register value with its metadata.
type Value struct {
	// Key is the register key (e.g., "pv_voltage_1", "status").
	Key string
	// Name is the human-readable register name.
	Name string
	// RawValue is the raw numeric value before scaling.
	RawValue float64
	// DecodedValue is the value after applying the scale factor.
	DecodedValue float64
	// StringValue holds the decoded string for String-type registers.
	StringValue string
	// Unit is the unit of measurement.
	Unit string
	// Timestamp is when the value was read.
	Timestamp time.Time `json:"timestamp"`
	// DataType is the type of the value (omitted from JSON output).
	DataType DataType `json:"-"`
	// Stability indicates if this is a stable or dynamic register (omitted from JSON output).
	Stability Stability `json:"-"`
	// StatusDecoded holds the decoded status information for status registers.
	// For solis_status: map[string]string with "name" and "description"
	// For bitmask status: []string with list of active fault/status names
	StatusDecoded interface{} `json:"status_decoded,omitempty"`
}

// MarshalJSON implements json.Marshaler for Value to ensure DecodedValue is rounded to 2 decimal places.
func (v Value) MarshalJSON() ([]byte, error) {
	// Create a copy with rounded DecodedValue
	type Alias Value
	aux := struct {
		DecodedValue float64 `json:"value"`
		*Alias
	}{
		Alias:       (*Alias)(&v),
		DecodedValue: utils.RoundTo2DecimalPlaces(v.DecodedValue),
	}
	return json.Marshal(aux)
}

// DecodeRegister decodes raw bytes from a register into a typed Value.
// This is the main entry point for decoding individual register data.
func DecodeRegister(reg *Register, raw []byte) Value {
	rawVal := decodeRaw(reg.DataType, raw)
	decoded := rawVal * reg.Scale
	
	// Round to exactly 2 decimal places for consistent display
	decoded = utils.RoundTo2DecimalPlaces(decoded)

	value := Value{
		Key:          reg.Key,
		Name:         reg.Name,
		RawValue:     rawVal,
		DecodedValue: decoded,
		Unit:         reg.Unit,
		DataType:     reg.DataType,
		Stability:    reg.Stability,
	}

	// Decode status information for status registers
	if reg.Status {
		value.StatusDecoded = decodeStatus(reg, uint16(rawVal))
	}

	return value
}

// decodeStatus decodes status/fault register values based on their key.
// Returns the decoded status information as either a map (for solis_status)
// or a list of strings (for bitmask fault registers).
func decodeStatus(reg *Register, rawValue uint16) interface{} {
	switch reg.Key {
	case "solis_status":
		return DecodeSolisStatus(rawValue)
	case "operating_status":
		return DecodeOperatingStatus(rawValue)
	case "grid_fault_status_01":
		return DecodeGridFaultStatus01(rawValue)
	case "backup_load_fault_status_02":
		return DecodeBackupFaultStatus02(rawValue)
	case "battery_fault_status_03":
		return DecodeBatteryFaultStatus03(rawValue)
	case "device_fault_status_04":
		return DecodeDeviceFaultStatus04(rawValue)
	case "device_fault_status_05":
		return DecodeDeviceFaultStatus05(rawValue)
	case "battery_fault_status_1_bms":
		return DecodeBatteryFaultStatus1Bms(rawValue)
	case "battery_fault_status_2_bms":
		return DecodeBatteryFaultStatus2Bms(rawValue)
	default:
		// For unknown status registers, try generic fault decoding
		return DecodeFaultBits(reg.Address, rawValue)
	}
}

// decodeRaw converts raw bytes to a float64 value based on the data type.
// The raw bytes are in Modbus format (big-endian for multi-register values).
func decodeRaw(dataType DataType, raw []byte) float64 {
	if len(raw) == 0 {
		return 0
	}

	switch dataType {
	case Uint16:
		return float64(binary.BigEndian.Uint16(raw))
	case Int16:
		return float64(int16(binary.BigEndian.Uint16(raw)))
	case Uint32:
		if len(raw) >= 4 {
			return float64(binary.BigEndian.Uint32(raw))
		}
		return 0
	case Int32:
		if len(raw) >= 4 {
			return float64(int32(binary.BigEndian.Uint32(raw)))
		}
		return 0
	case Float32:
		if len(raw) >= 4 {
			return float64(math.Float32frombits(binary.BigEndian.Uint32(raw)))
		}
		return 0
	case String:
		// For string type, return 0 and use DecodeString separately
		return 0
	case Bool:
		if len(raw) >= 2 {
			if binary.BigEndian.Uint16(raw) != 0 {
				return 1
			}
			return 0
		}
		return 0
	default:
		return 0
	}
}

// DecodeString decodes a string register from raw bytes.
// Each Uint16 in the raw bytes contains 2 ASCII characters (big-endian).
// Example: raw = [0x41, 0x42, 0x43, 0x44] -> "ABCD"
func DecodeString(raw []byte) string {
	result := make([]byte, 0, len(raw))

	for i := 0; i+1 < len(raw); i += 2 {
		// Each Uint16 contains 2 ASCII characters
		word := binary.BigEndian.Uint16(raw[i : i+2])
		// Extract high and low bytes
		high := byte(word >> 8)
		low := byte(word & 0xFF)

		// Only include printable ASCII characters (32-126) and space (32)
		if high >= 32 && high <= 126 {
			result = append(result, high)
		}
		if low >= 32 && low <= 126 {
			result = append(result, low)
		}
	}

	// Trim trailing spaces
	for len(result) > 0 && result[len(result)-1] == ' ' {
		result = result[:len(result)-1]
	}

	return string(result)
}

// DecodeRange decodes all registers within a single read range.
// startAddr is the starting address of the range.
// raw is the raw bytes returned from Modbus (length = Count * 2).
// Returns a map of register keys to their decoded Values.
func DecodeRange(startAddr uint16, raw []byte) map[string]Value {
	decoderLogger.Debug().Msgf("Decoding range starting at address %d, %d bytes", startAddr, len(raw))

	result := make(map[string]Value)

	// Each register is 2 bytes (Uint16)
	// We need to handle multi-register types (Uint32, Int32, Float32 take 2 registers = 4 bytes)

	i := 0
	for i < len(raw) {
		addr := startAddr + uint16(i/2)

		// Check if there's a register defined at this address
		if reg, ok := RegisterMap[addr]; ok {
			// Calculate how many bytes we need to read for this register
			byteCount := int(reg.Count) * 2

			if i+byteCount <= len(raw) {
				regRaw := raw[i : i+byteCount]

				// Special handling for string type
				if reg.DataType == String {
					strValue := DecodeString(regRaw)
					decoderLogger.Debug().Msgf("Decoded string register %s (%d): %q", reg.Key, addr, strValue)
					result[reg.Key] = Value{
						Key:          reg.Key,
						Name:         reg.Name,
						RawValue:     0,
						DecodedValue: 0,
						StringValue:  strValue,
						Unit:         reg.Unit,
						DataType:     reg.DataType,
						Stability:    reg.Stability,
					}
				} else {
					value := DecodeRegister(reg, regRaw)
					decoderLogger.Debug().Msgf("Decoded register %s (%d): raw=%.1f, decoded=%.1f %s",
						reg.Key, addr, value.RawValue, value.DecodedValue, value.Unit)
					result[reg.Key] = value
				}

				// Move past this register (and any additional registers it occupies)
				i += byteCount
				continue
			} else {
				decoderLogger.Warn().Msgf("Insufficient bytes for register %s at address %d (need %d, have %d)",
					reg.Key, addr, byteCount, len(raw)-i)
			}
		}

		// No register defined at this address, skip 2 bytes (one Uint16)
		i += 2
	}

	decoderLogger.Debug().Msgf("Decoded %d values from range %d", len(result), startAddr)
	return result
}
