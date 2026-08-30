// Package solis provides Solis inverter register definitions, status maps,
// and utilities for decoding Modbus register data.
package solis

import (
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
		DecodedValue utils.Float64With2Decimals `json:"value"`
		*Alias
	}{
		Alias:        (*Alias)(&v),
		DecodedValue: utils.Float64With2Decimals(utils.RoundTo2DecimalPlaces(v.DecodedValue)),
	}
	return json.Marshal(aux)
}

// DecodeRegister decodes raw register values into a typed Value.
// This is the main entry point for decoding individual register data.
// raw contains the uint16 register values as returned directly from Modbus.
func DecodeRegister(reg *Register, raw []uint16) Value {
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
	return decodeStatusByKey(reg.Key, reg.Address, rawValue)
}

// decodeStatusByKey decodes status based on the register key
func decodeStatusByKey(key string, address uint16, rawValue uint16) interface{} {
	if handler, ok := statusDecoders[key]; ok {
		return handler(rawValue)
	}
	return DecodeFaultBits(address, rawValue)
}

// statusDecoders maps status register keys to their decoding functions
var statusDecoders = map[string]func(uint16) interface{}{
	"solis_status":        decodeSolisStatusHandler,
	"operating_status":    decodeOperatingStatusHandler,
	"grid_fault_1":        decodeGridFault01Handler,
	"backup_fault_2":      decodeBackupFault02Handler,
	"battery_fault_3":     decodeBatteryFault03Handler,
	"device_fault_4":      decodeDeviceFault04Handler,
	"device_fault_5":      decodeDeviceFault05Handler,
	"battery_fault_1_bms": decodeBatteryFault1BmsHandler,
	"battery_fault_2_bms": decodeBatteryFault2BmsHandler,
}

// Handler wrappers for status decoding
func decodeSolisStatusHandler(value uint16) interface{}     { return DecodeSolisStatus(value) }
func decodeOperatingStatusHandler(value uint16) interface{} { return DecodeOperatingStatus(value) }
func decodeGridFault01Handler(value uint16) interface{}     { return DecodeGridFaultStatus01(value) }
func decodeBackupFault02Handler(value uint16) interface{}   { return DecodeBackupFaultStatus02(value) }
func decodeBatteryFault03Handler(value uint16) interface{}  { return DecodeBatteryFaultStatus03(value) }
func decodeDeviceFault04Handler(value uint16) interface{}   { return DecodeDeviceFaultStatus04(value) }
func decodeDeviceFault05Handler(value uint16) interface{}   { return DecodeDeviceFaultStatus05(value) }
func decodeBatteryFault1BmsHandler(value uint16) interface{} {
	return DecodeBatteryFaultStatus1Bms(value)
}
func decodeBatteryFault2BmsHandler(value uint16) interface{} {
	return DecodeBatteryFaultStatus2Bms(value)
}

// decodeRaw converts raw uint16 register values to a float64 value based on the data type.
// The registers are in Modbus format (big-endian for multi-register values).
// For multi-register types (Uint32, Int32, Float32), the first uint16 is the high word.
func decodeRaw(dataType DataType, raw []uint16) float64 {
	if len(raw) == 0 {
		return 0
	}

	// Get the decoder function for this data type
	decoder, ok := rawDecoders[dataType]
	if !ok {
		return 0
	}
	return decoder(raw)
}

// rawDecoders maps data types to their decoding functions
var rawDecoders = map[DataType]func([]uint16) float64{
	Uint16:  decodeUint16,
	Int16:   decodeInt16,
	Uint32:  decodeUint32,
	Int32:   decodeInt32,
	Float32: decodeFloat32,
	Bool:    decodeBool,
}

// decodeUint16 decodes a Uint16 value
func decodeUint16(raw []uint16) float64 {
	return float64(raw[0])
}

// decodeInt16 decodes an Int16 value
func decodeInt16(raw []uint16) float64 {
	return float64(int16(raw[0])) // #nosec G115
}

// decodeUint32 decodes a Uint32 value from two uint16 registers
func decodeUint32(raw []uint16) float64 {
	if len(raw) < 2 {
		return 0
	}
	return float64(uint32(raw[0])<<16 | uint32(raw[1]))
}

// decodeInt32 decodes an Int32 value from two uint16 registers
func decodeInt32(raw []uint16) float64 {
	if len(raw) < 2 {
		return 0
	}
	return float64(int32(uint32(raw[0])<<16 | uint32(raw[1]))) // #nosec G115
}

// decodeFloat32 decodes a Float32 value from two uint16 registers
func decodeFloat32(raw []uint16) float64 {
	if len(raw) < 2 {
		return 0
	}
	bits := uint32(raw[0])<<16 | uint32(raw[1])
	return float64(math.Float32frombits(bits))
}

// decodeBool decodes a Bool value
func decodeBool(raw []uint16) float64 {
	if len(raw) >= 1 && raw[0] != 0 {
		return 1
	}
	return 0
}

// DecodeString decodes a string register from raw uint16 values.
// Each uint16 contains 2 ASCII characters (high byte and low byte).
// Example: raw = [0x4845, 0x4C4C, 0x4F20] -> "HELLO " -> "HELLO"
func DecodeString(raw []uint16) string {
	result := make([]byte, 0, len(raw)*2)

	for _, word := range raw {
		// Extract high and low bytes from each uint16
		high := byte(word >> 8)
		low := byte(word & 0xFF)

		// Only include printable ASCII characters (32-126)
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
// raw contains the uint16 register values as returned directly from Modbus.
// Returns a map of register keys to their decoded Values.
func DecodeRange(startAddr uint16, raw []uint16) map[string]Value {
	decoderLogger.Debug().Msgf("Decoding range starting at address %d, %d registers", startAddr, len(raw))

	result := make(map[string]Value)

	// Iterate through registers by index
	// Each register in the raw slice corresponds to a Modbus address: raw[i] = startAddr + i
	i := 0
	for i < len(raw) {
		addr := startAddr + uint16(i)

		// Check if there's a register defined at this address
		if reg, ok := RegisterMap[addr]; ok {
			// Check if we have enough registers for this data type
			regCount := int(reg.Count)

			if i+regCount <= len(raw) {
				regRaw := raw[i : i+regCount]

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
				i += regCount
				continue
			} else {
				decoderLogger.Warn().Msgf("Insufficient registers for %s at address %d (need %d, have %d)",
					reg.Key, addr, regCount, len(raw)-i)
			}
		}

		// No register defined at this address, skip 1 register (one Uint16)
		i += 1
	}

	decoderLogger.Debug().Msgf("Decoded %d values from range %d", len(result), startAddr)
	return result
}
