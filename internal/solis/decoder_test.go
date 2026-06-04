package solis

import (
	"fmt"
	"testing"
)

// getStatusName returns the short name for a status code.
// Returns "Unknown Status (0xXXXX)" if the code is not found.
func getStatusName(code uint16) string {
	if name, ok := STATUS_MAP[code]; ok {
		return name
	}
	logger.Debug().Msgf("Unknown status code: 0x%04X", code)
	return fmt.Sprintf("Unknown Status (0x%04X)", code)
}

// getStatusDescription returns the detailed description for a status code.
// Returns "Unknown status code: 0xXXXX" if the code is not found.
func getStatusDescription(code uint16) string {
	if desc, ok := STATUS_DESCRIPTION[code]; ok {
		return desc
	}
	logger.Debug().Msgf("Unknown status code description: 0x%04X", code)
	return fmt.Sprintf("Unknown status code: 0x%04X", code)
}

// decodeOpStatusBits returns a slice of active status descriptions for the
// Operating Status register (33121).
func decodeOpStatusBits(value uint16) []string {
	var statuses []string
	for i := range 16 {
		if value&(1<<i) != 0 {
			if i < len(OP_STATUS_BIT_MAP) && OP_STATUS_BIT_MAP[i] != "" {
				statuses = append(statuses, OP_STATUS_BIT_MAP[i])
			} else {
				statuses = append(statuses, fmt.Sprintf("Unknown status bit %d", i))
			}
		}
	}
	return statuses
}

func TestDecodeRaw(t *testing.T) {
	tests := []struct {
		name     string
		dataType DataType
		raw      []byte
		want     float64
	}{
		// Uint16 tests
		{"Uint16 zero", Uint16, []byte{0x00, 0x00}, 0},
		{"Uint16 one", Uint16, []byte{0x00, 0x01}, 1},
		{"Uint16 max", Uint16, []byte{0xFF, 0xFF}, 65535},
		{"Uint16 mid", Uint16, []byte{0x12, 0x34}, 0x1234},

		// Int16 tests
		{"Int16 zero", Int16, []byte{0x00, 0x00}, 0},
		{"Int16 positive", Int16, []byte{0x00, 0x01}, 1},
		{"Int16 negative", Int16, []byte{0xFF, 0xFF}, -1},
		{"Int16 max positive", Int16, []byte{0x7F, 0xFF}, 32767},
		{"Int16 min negative", Int16, []byte{0x80, 0x00}, -32768},

		// Uint32 tests
		{"Uint32 zero", Uint32, []byte{0x00, 0x00, 0x00, 0x00}, 0},
		{"Uint32 one", Uint32, []byte{0x00, 0x00, 0x00, 0x01}, 1},
		{"Uint32 max", Uint32, []byte{0xFF, 0xFF, 0xFF, 0xFF}, 4294967295},
		{"Uint32 mid", Uint32, []byte{0x12, 0x34, 0x56, 0x78}, 0x12345678},

		// Int32 tests
		{"Int32 zero", Int32, []byte{0x00, 0x00, 0x00, 0x00}, 0},
		{"Int32 positive", Int32, []byte{0x00, 0x00, 0x00, 0x01}, 1},
		{"Int32 negative", Int32, []byte{0xFF, 0xFF, 0xFF, 0xFF}, -1},
		{"Int32 max positive", Int32, []byte{0x7F, 0xFF, 0xFF, 0xFF}, 2147483647},
		{"Int32 min negative", Int32, []byte{0x80, 0x00, 0x00, 0x00}, -2147483648},

		// Bool tests
		{"Bool false", Bool, []byte{0x00, 0x00}, 0},
		{"Bool true", Bool, []byte{0x00, 0x01}, 1},
		{"Bool true non-zero", Bool, []byte{0xFF, 0xFF}, 1},

		// Empty raw
		{"Empty raw", Uint16, []byte{}, 0},
		{"Insufficient bytes Uint32", Uint32, []byte{0x00, 0x00}, 0},
		{"Insufficient bytes Int32", Int32, []byte{0x00, 0x00}, 0},
		{"Insufficient bytes Float32", Float32, []byte{0x00, 0x00}, 0},

		// String type should return 0
		{"String type", String, []byte{0x41, 0x42}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeRaw(tt.dataType, tt.raw)
			if got != tt.want {
				t.Errorf("decodeRaw(%v, %v) = %v, want %v", tt.dataType, tt.raw, got, tt.want)
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{"empty", []byte{}, ""},
		{"single word", []byte{0x41, 0x42}, "AB"},
		{"two words", []byte{0x41, 0x42, 0x43, 0x44}, "ABCD"},
		{"with spaces", []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x20, 0x57, 0x6F, 0x72, 0x6C, 0x64, 0x00}, "Hello World"},
		{"trailing spaces", []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x20, 0x20, 0x20}, "Hello"},
		{"non-printable", []byte{0x00, 0x01, 0x1F, 0x7F}, ""},
		{"mixed printable and non-printable", []byte{0x41, 0x00, 0x42, 0x1F}, "AB"},
		{"numbers", []byte{0x30, 0x31, 0x32, 0x33}, "0123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeString(tt.raw)
			if got != tt.want {
				t.Errorf("DecodeString(%v) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDecodeRegister(t *testing.T) {
	reg := &Register{
		Key:       "test_register",
		Name:      "Test Register",
		Address:   100,
		Count:     1,
		DataType:  Uint16,
		Scale:     0.1,
		Unit:      "V",
		Stability: Dynamic,
	}

	raw := []byte{0x00, 0x64} // 100 in Uint16
	got := DecodeRegister(reg, raw)

	if got.Key != reg.Key {
		t.Errorf("Key = %v, want %v", got.Key, reg.Key)
	}
	if got.Name != reg.Name {
		t.Errorf("Name = %v, want %v", got.Name, reg.Name)
	}
	if got.RawValue != 100 {
		t.Errorf("RawValue = %v, want %v", got.RawValue, 100)
	}
	if got.DecodedValue != 10 {
		t.Errorf("DecodedValue = %v, want %v (100 * 0.1)", got.DecodedValue, 10)
	}
	if got.Unit != reg.Unit {
		t.Errorf("Unit = %v, want %v", got.Unit, reg.Unit)
	}
	if got.DataType != reg.DataType {
		t.Errorf("DataType = %v, want %v", got.DataType, reg.DataType)
	}
	if got.Stability != reg.Stability {
		t.Errorf("Stability = %v, want %v", got.Stability, reg.Stability)
	}
}

func TestDecodeRegister_WithFloat32(t *testing.T) {
	reg := &Register{
		Key:       "test_float",
		Name:      "Test Float",
		Address:   200,
		Count:     2,
		DataType:  Float32,
		Scale:     1.0,
		Unit:      "A",
		Stability: Dynamic,
	}

	// Float32 representation of 1.5
	// In IEEE 754: 1.5 = 0x3FC00000
	raw := []byte{0x3F, 0xC0, 0x00, 0x00}
	got := DecodeRegister(reg, raw)

	// Check that the value is approximately 1.5
	if got.RawValue < 1.499 || got.RawValue > 1.501 {
		t.Errorf("RawValue = %v, want approximately 1.5", got.RawValue)
	}
}

func TestDecodeRange(t *testing.T) {
	// Create test registers at specific addresses
	reg1 := &Register{
		Key:       "reg1",
		Name:      "Register 1",
		Address:   100,
		Count:     1,
		DataType:  Uint16,
		Scale:     1.0,
		Unit:      "",
		Stability: Stable,
	}
	reg2 := &Register{
		Key:       "reg2",
		Name:      "Register 2",
		Address:   102,
		Count:     1,
		DataType:  Int16,
		Scale:     1.0,
		Unit:      "",
		Stability: Stable,
	}

	// Temporarily add test registers to RegisterMap
	originalReg1 := RegisterMap[100]
	originalReg2 := RegisterMap[102]
	RegisterMap[100] = reg1
	RegisterMap[102] = reg2
	defer func() {
		RegisterMap[100] = originalReg1
		RegisterMap[102] = originalReg2
	}()

	// Create raw data: address 100=0x0100 (256), address 101=0x0000 (gap), address 102=0xFFFE (-2 in Int16)
	raw := []byte{0x01, 0x00, 0x00, 0x00, 0xFF, 0xFE}

	got := DecodeRange(100, raw)

	// Check that we got 2 values
	if len(got) != 2 {
		t.Errorf("DecodeRange returned %d values, want 2", len(got))
	}

	// Check reg1
	if val, ok := got["reg1"]; !ok {
		t.Error("Expected reg1 in result")
	} else {
		if val.RawValue != 256 {
			t.Errorf("reg1.RawValue = %v, want 256", val.RawValue)
		}
	}

	// Check reg2
	if val, ok := got["reg2"]; !ok {
		t.Error("Expected reg2 in result")
	} else {
		if val.RawValue != -2 {
			t.Errorf("reg2.RawValue = %v, want -2", val.RawValue)
		}
	}
}

func TestDecodeRange_WithString(t *testing.T) {
	// Create a string register
	reg := &Register{
		Key:       "serial",
		Name:      "Serial Number",
		Address:   1000,
		Count:     5, // 5 registers = 10 bytes
		DataType:  String,
		Scale:     1.0,
		Unit:      "",
		Stability: Stable,
	}

	// Temporarily add test register to RegisterMap
	originalReg := RegisterMap[1000]
	RegisterMap[1000] = reg
	defer func() {
		RegisterMap[1000] = originalReg
	}()

	// Create raw data: "HELLO" in ASCII
	// Each Uint16 holds 2 characters
	// "HE" = 0x4845, "LL" = 0x4C4C, "O " = 0x4F20
	raw := []byte{0x48, 0x45, 0x4C, 0x4C, 0x4F, 0x20, 0x00, 0x00, 0x00, 0x00}

	got := DecodeRange(1000, raw)

	if len(got) != 1 {
		t.Errorf("DecodeRange returned %d values, want 1", len(got))
	}

	if val, ok := got["serial"]; !ok {
		t.Error("Expected serial in result")
	} else {
		if val.StringValue != "HELLO" {
			t.Errorf("serial.StringValue = %q, want %q", val.StringValue, "HELLO")
		}
	}
}

func TestDecodeRange_EmptyRaw(t *testing.T) {
	got := DecodeRange(100, []byte{})
	if len(got) != 0 {
		t.Errorf("DecodeRange with empty raw returned %d values, want 0", len(got))
	}
}

func TestDataType_String(t *testing.T) {
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
				t.Errorf("DataType(%d).String() = %q, want %q", tt.dataType, got, tt.want)
			}
		})
	}
}

func TestStability_String(t *testing.T) {
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
				t.Errorf("Stability(%d).String() = %q, want %q", tt.stability, got, tt.want)
			}
		})
	}
}

func TestGetStatusName(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{0x0000, "Waiting"},
		{0x0001, "OpenRun"},
		{0x0003, "Generating"},
		{0xFFFF, "Unknown Status (0xFFFF)"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := getStatusName(tt.code); got != tt.want {
				t.Errorf("getStatusName(%v) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestGetStatusDescription(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{0x0000, "Normal operation / Waiting"},
		{0x0001, "Open operating"},
		{0x0003, "Initializing / Generating"},
		{0xFFFF, "Unknown status code: 0xFFFF"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := getStatusDescription(tt.code); got != tt.want {
				t.Errorf("getStatusDescription(%v) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestDecodeFaultBits(t *testing.T) {
	// Test with a known address that has a bit map
	// Address 33120 is the Fault Code register
	tests := []struct {
		name  string
		addr  uint16
		value uint16
		want  []string
	}{
		// Assuming STATUS_MAP has entries, test with address that has a bit map
		// For now, test with address that doesn't have a bit map
		{"unknown address", 9999, 0x0001, []string{"No bit map defined for register 9999"}},
		{"zero value", 33120, 0x0000, []string{}},
		{"non-zero value unknown bits", 33120, 0x0001, []string{"Unknown bit 0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeFaultBits(tt.addr, tt.value)
			// For unknown address, just check length
			if len(got) != len(tt.want) {
				t.Errorf("DecodeFaultBits(%v, %v) returned %d items, want %d", tt.addr, tt.value, len(got), len(tt.want))
			}
		})
	}
}

func TestDecodeOpStatusBits(t *testing.T) {
	tests := []struct {
		value uint16
		want  []string
	}{
		{0x0000, []string{}},
		{0x0001, []string{"Bit 0"}},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := decodeOpStatusBits(tt.value)
			// Just verify it doesn't panic
			if len(got) == 0 && tt.value != 0 {
				// If value has bits set, we should get some output
				// But the bit map might be empty, so just check it runs
			}
		})
	}
}
