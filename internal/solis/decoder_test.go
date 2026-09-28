package solis

import (
	"math"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var at = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func newTestDecoder(t *testing.T) (*Decoder, *Registry) {
	t.Helper()
	r := newTestRegistry(t)
	return NewDecoder(r, zerolog.Nop()), r
}

func TestDecodeRaw(t *testing.T) {
	f := math.Float32bits(12.5)
	tests := []struct {
		name string
		dt   DataType
		raw  []uint16
		want float64
	}{
		{"uint16", Uint16, []uint16{65535}, 65535},
		{"int16 negative", Int16, []uint16{0xFFFE}, -2},
		{"uint32", Uint32, []uint16{0x0001, 0x0002}, 65538},
		{"int32 small negative (not an inverter bug)", Int32, []uint16{0xFFFF, 0xFFFE}, -2},
		{"float32", Float32, []uint16{uint16(f >> 16), uint16(f)}, 12.5},
		{"bool true", Bool, []uint16{7}, 1},
		{"bool false", Bool, []uint16{0}, 0},
		{"too short", Uint32, []uint16{1}, 0},
		{"empty", Uint16, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, decodeRaw(tt.dt, tt.raw), 1e-9)
		})
	}
}

func TestDecode_FullPrecisionNoRounding(t *testing.T) {
	d, r := newTestDecoder(t)
	reg, _ := r.ByKey("pv_energy_daily")
	v := d.Decode(reg, []uint16{12345}, at)
	assert.InDelta(t, 1234.5, v.DecodedValue, 1e-9)
	assert.InDelta(t, 12345.0, v.RawValue, 1e-9)
	assert.Equal(t, "kWh", v.Unit)
	assert.Equal(t, at, v.Timestamp)
	assert.Nil(t, v.StatusDecoded)

	reg.Scale = 0.001
	v = d.Decode(reg, []uint16{1}, at)
	assert.InDelta(t, 0.001, v.DecodedValue, 1e-12)
}

func TestDecodeBlock(t *testing.T) {
	d, r := newTestDecoder(t)
	b := r.Blocks()[0] // 33035..33058
	raw := make([]uint16, b.Count)
	raw[0] = 250               // pv_energy_daily 25.0 kWh
	raw[22], raw[23] = 0, 5230 // pv_total_power
	values := d.DecodeBlock(b, raw, at)
	require.Len(t, values, 2)
	assert.InDelta(t, 25.0, values["pv_energy_daily"].DecodedValue, 1e-9)
	assert.InDelta(t, 5230.0, values["pv_total_power"].DecodedValue, 1e-9)

	// Truncated block: the two-word register at the end cannot be decoded.
	short := d.DecodeBlock(b, raw[:23], at)
	assert.Len(t, short, 1)
}

func TestDecodeBlock_StatusAndSignedGrid(t *testing.T) {
	d, r := newTestDecoder(t)
	b := r.Blocks()[1] // 33095..33179
	raw := make([]uint16, b.Count)
	raw[0] = 0x0003                                     // solis_status Generating
	raw[33130-33095], raw[33131-33095] = 0xFFFF, 0xFFFE // grid_power -2 W
	values := d.DecodeBlock(b, raw, at)
	assert.Equal(t, map[string]string{
		"name":        "Generating",
		"description": "Initializing / Generating",
	}, values["solis_status"].StatusDecoded)
	assert.InDelta(t, -2.0, values["grid_power"].DecodedValue, 1e-9)
	assert.Nil(t, values["grid_fault_1"].StatusDecoded)
}

func TestDecodeStatus(t *testing.T) {
	d, _ := newTestDecoder(t)
	assert.Equal(t, []string{"No grid", "Grid undervoltage"}, d.DecodeStatus("grid_fault_1", 0b101))
	assert.Equal(t, []string{"Unknown bit 15"}, d.DecodeStatus("grid_fault_1", 1<<15))
	assert.Equal(t, []string{"Normal operation", "Unknown status bit 11"},
		d.DecodeStatus("operating_status", 1|1<<11))
	assert.Equal(t, []string{"Battery 2 internal fault"},
		d.DecodeStatus("battery_fault_2_bms", 1<<7))
	assert.Nil(t, d.DecodeStatus("device_fault_5", 0))
	assert.Equal(t, []string{"No bit map defined for register x"}, d.DecodeStatus("x", 1))
	assert.Equal(t, map[string]string{
		"name":        "Unknown Status (0x9999)",
		"description": "Unknown status code: 0x9999",
	}, d.DecodeStatus("solis_status", 0x9999))
}

func TestDerive_BatteryPowerSigned(t *testing.T) {
	d, _ := newTestDecoder(t)
	mk := func(dir float64) map[string]*Value {
		return map[string]*Value{
			KeyBatteryPower:     {RawValue: 1500, DecodedValue: 1500},
			KeyBatteryDirection: {RawValue: dir, DecodedValue: dir},
		}
	}

	charging := mk(0)
	d.Derive(charging, at)
	assert.InDelta(t, 1500.0, charging[KeyBatteryPowerSigned].DecodedValue, 1e-9)
	assert.Equal(t, "W", charging[KeyBatteryPowerSigned].Unit)

	discharging := mk(1)
	d.Derive(discharging, at)
	assert.InDelta(t, -1500.0, discharging[KeyBatteryPowerSigned].DecodedValue, 1e-9)

	unknown := mk(7)
	d.Derive(unknown, at)
	assert.NotContains(t, unknown, KeyBatteryPowerSigned)

	missing := map[string]*Value{KeyBatteryPower: {RawValue: 1}}
	d.Derive(missing, at)
	assert.NotContains(t, missing, KeyBatteryPowerSigned)
}
