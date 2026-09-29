// Package solis provides the Solis inverter register model: one validated register table
// (Registry), read-block planning, decoding of raw Modbus words and derived live values.
// Nothing here is package-level state; tables are built by NewRegistry and NewDecoder.
package solis

import (
	"time"

	"github.com/dombyte/solis/internal/period"
)

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
	// Bool is a boolean value (1 register, 0=false, non-zero=true).
	Bool
)

// String returns the name used by /api/keys.
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
	case Bool:
		return "Bool"
	default:
		return "Unknown"
	}
}

// Register counts of the 16- and 32-bit data types.
const (
	singleWord uint16 = 1
	doubleWord uint16 = 2
)

// Count returns the number of 16-bit Modbus registers the type occupies.
func (d DataType) Count() uint16 {
	switch d {
	case Uint32, Int32, Float32:
		return doubleWord
	default:
		return singleWord
	}
}

// Store is the single destination enum of a register.
type Store int

const (
	// StoreNone is cache-only (live power, derived values), never persisted.
	StoreNone Store = iota
	// StoreDaily goes to daily_values.
	StoreDaily
	// StoreMonthly goes to monthly_values (computed).
	StoreMonthly
	// StoreYearly goes to yearly_values (computed).
	StoreYearly
	// StoreTotal goes to total_values (computed).
	StoreTotal
	// StoreStatus goes to error_data, written on change.
	StoreStatus
)

// String returns the store name ("daily", "status", ...).
func (s Store) String() string {
	switch s {
	case StoreNone:
		return "none"
	case StoreDaily:
		return "daily"
	case StoreMonthly:
		return "monthly"
	case StoreYearly:
		return "yearly"
	case StoreTotal:
		return "total"
	case StoreStatus:
		return "status"
	default:
		return "invalid"
	}
}

// Level maps an energy store to its period level; ok is false for none/status.
func (s Store) Level() (period.Level, bool) {
	switch s {
	case StoreDaily:
		return period.Daily, true
	case StoreMonthly:
		return period.Monthly, true
	case StoreYearly:
		return period.Yearly, true
	case StoreTotal:
		return period.Total, true
	default:
		return 0, false
	}
}

// StoreForLevel is the inverse of Store.Level.
func StoreForLevel(l period.Level) Store {
	switch l {
	case period.Daily:
		return StoreDaily
	case period.Monthly:
		return StoreMonthly
	case period.Yearly:
		return StoreYearly
	default:
		return StoreTotal
	}
}

// Register describes one register key. Address 0 means computed or derived: the
// register is never part of the poll plan.
type Register struct {
	// Key is the unique identifier (API, storage, WebSocket).
	Key string
	// Name is the human-readable name.
	Name string
	// Address is the Modbus input register address (0 = not polled).
	Address uint16
	// DataType is the raw value type; it determines the register count.
	DataType DataType
	// Scale is applied to the raw value (0.1 = divide by 10).
	Scale float64
	// Unit is the unit of measurement.
	Unit string
	// Store is the destination of the value.
	Store Store
	// Net marks export-import values (latest-value rule, aggregator-written).
	Net bool
}

// Count returns the number of Modbus registers the value occupies.
func (r Register) Count() uint16 { return r.DataType.Count() }

// Polled reports whether the register is read from the inverter.
func (r Register) Polled() bool { return r.Address != 0 }

// Computed reports whether the aggregator produces the value.
func (r Register) Computed() bool {
	switch r.Store {
	case StoreMonthly, StoreYearly, StoreTotal:
		return true
	default:
		return r.Net
	}
}

// needsAddress reports whether the register must be read from the inverter (status and
// non-net daily registers).
func (r Register) needsAddress() bool {
	return r.Store == StoreStatus || (r.Store == StoreDaily && !r.Net)
}

// polledDaily reports whether the register is a polled, non-net daily counter.
func (r Register) polledDaily() bool {
	return r.Store == StoreDaily && !r.Net && r.Polled()
}

// energy reports whether the store is one of the period energy stores.
func (s Store) energy() bool {
	_, ok := s.Level()
	return ok
}

// Edge is a daily → monthly/yearly/total sum definition.
type Edge struct {
	// Level is the target level.
	Level period.Level
	// Source is the daily key being summed.
	Source string
	// Target is the computed key.
	Target string
}

// NetPair defines Target = Export - Import at one level.
type NetPair struct {
	// Level is the period level of all three keys.
	Level period.Level
	// Export is the export key.
	Export string
	// Import is the import key.
	Import string
	// Target is the net key.
	Target string
}

// Block is one contiguous Modbus read.
type Block struct {
	// Start is the first register address.
	Start uint16
	// Count is the number of registers to read.
	Count uint16
}

// Value is a decoded register value. DecodedValue keeps full precision; rounding to
// two decimals happens only when serializing for clients.
type Value struct {
	// Key is the register key.
	Key string
	// Name is the human-readable register name.
	Name string
	// RawValue is the value before scaling.
	RawValue float64
	// DecodedValue is RawValue * Scale.
	DecodedValue float64
	// Unit is the unit of measurement.
	Unit string
	// Timestamp is the poll (or computation) instant.
	Timestamp time.Time
	// StatusDecoded is the decoded status: map[string]string for solis_status, []string
	// of active bits for bitmask registers, nil otherwise.
	StatusDecoded any
}
