package solis

import (
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog"
)

// Battery direction flag values (register 33135).
const (
	directionCharging    = 0
	directionDischarging = 1
)

const (
	solisStatusKey     = "solis_status"
	operatingStatusKey = "operating_status"
	bitsPerRegister    = 16
	wordShift          = 16
)

// Lookup resolves registers by key and address.
type Lookup interface {
	ByKey(key string) (Register, bool)
	ByAddress(addr uint16) (Register, bool)
}

// Decoder turns raw Modbus words into Values and decodes status/fault registers.
type Decoder struct {
	regs   Lookup
	codes  map[uint16]statusCode
	bits   map[string][]string
	log    zerolog.Logger
	signed Register
}

// NewDecoder builds a decoder over the given register lookup.
func NewDecoder(regs Lookup, log zerolog.Logger) *Decoder {
	signed, _ := regs.ByKey(KeyBatteryPowerSigned)
	return &Decoder{regs: regs, codes: statusCodes(), bits: bitNames(), log: log, signed: signed}
}

// Decode decodes the raw words of exactly one register (full precision).
func (d *Decoder) Decode(reg Register, raw []uint16, at time.Time) *Value {
	rawVal := decodeRaw(reg.DataType, raw)
	v := &Value{
		Key:          reg.Key,
		Name:         reg.Name,
		RawValue:     rawVal,
		DecodedValue: rawVal * reg.Scale,
		Unit:         reg.Unit,
		Timestamp:    at,
	}
	if reg.Store == StoreStatus {
		v.StatusDecoded = d.DecodeStatus(reg.Key, uint16(rawVal))
	}
	return v
}

// DecodeBlock decodes every register that starts inside a read block.
func (d *Decoder) DecodeBlock(b Block, raw []uint16, at time.Time) map[string]*Value {
	out := make(map[string]*Value)
	for i := 0; i < len(raw); i++ {
		reg, ok := d.regs.ByAddress(b.Start + uint16(i)) // #nosec G115 -- i < 125
		if !ok {
			continue
		}
		n := int(reg.Count())
		if i+n > len(raw) {
			d.log.Warn().Str("key", reg.Key).Int("need", n).Int("have", len(raw)-i).
				Msg("insufficient registers in block")
			break
		}
		out[reg.Key] = d.Decode(reg, raw[i:i+n], at)
		i += n - 1
	}
	return out
}

// Derive adds derived live values after a full poll was decoded: battery_power_signed is
// +battery_power while charging and -battery_power while discharging. Missing inputs or
// an unknown direction leave the derived key absent.
func (d *Decoder) Derive(values map[string]*Value, at time.Time) {
	p, okP := values[KeyBatteryPower]
	dir, okD := values[KeyBatteryDirection]
	if !okP || !okD || d.signed.Key == "" {
		return
	}
	var sign float64
	switch dir.RawValue {
	case directionCharging:
		sign = 1
	case directionDischarging:
		sign = -1
	default:
		return
	}
	values[KeyBatteryPowerSigned] = &Value{
		Key: d.signed.Key, Name: d.signed.Name, Unit: d.signed.Unit, Timestamp: at,
		RawValue: sign * p.RawValue, DecodedValue: sign * p.DecodedValue,
	}
}

// DecodeStatus decodes a status/fault register value by key: solis_status yields a
// {"name","description"} map, bitmask registers the list of active bit names.
func (d *Decoder) DecodeStatus(key string, raw uint16) any {
	if key == solisStatusKey {
		return d.decodeSolisStatus(raw)
	}
	names, ok := d.bits[key]
	if !ok {
		return []string{"No bit map defined for register " + key}
	}
	unknown := "Unknown bit %d"
	if key == operatingStatusKey {
		unknown = "Unknown status bit %d"
	}
	var active []string
	for i := range bitsPerRegister {
		if raw&(1<<i) == 0 {
			continue
		}
		if i < len(names) && names[i] != "" {
			active = append(active, names[i])
		} else {
			active = append(active, fmt.Sprintf(unknown, i))
		}
	}
	return active
}

func (d *Decoder) decodeSolisStatus(raw uint16) map[string]string {
	c, ok := d.codes[raw]
	if !ok {
		d.log.Debug().Uint16("status_code", raw).Msg("unknown status code")
		return map[string]string{
			"name":        fmt.Sprintf("Unknown Status (0x%04X)", raw),
			"description": fmt.Sprintf("Unknown status code: 0x%04X", raw),
		}
	}
	return map[string]string{"name": c.name, "description": c.desc}
}

// decodeRaw converts big-endian Modbus words to a float64 (high word first).
func decodeRaw(dt DataType, raw []uint16) float64 {
	if len(raw) < int(dt.Count()) || len(raw) == 0 {
		return 0
	}
	switch dt {
	case Int16:
		return float64(int16(raw[0])) // #nosec G115 -- two's complement reinterpretation
	case Uint32:
		return float64(word32(raw))
	case Int32:
		return float64(int32(word32(raw))) // #nosec G115 -- two's complement reinterpretation
	case Float32:
		return float64(math.Float32frombits(word32(raw)))
	case Bool:
		return boolValue(raw[0])
	default:
		return float64(raw[0])
	}
}

func boolValue(w uint16) float64 {
	if w != 0 {
		return 1
	}
	return 0
}

func word32(raw []uint16) uint32 {
	return uint32(raw[0])<<wordShift | uint32(raw[1])
}
