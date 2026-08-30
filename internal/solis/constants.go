// Package solis provides Solis inverter register definitions, status maps,
// and utilities for decoding Modbus register data.
package solis

import "time"

// Time format constants for consistent date/time formatting across the codebase.
const (
	// DateFormat is the standard date format (YYYY-MM-DD) used for daily data.
	DateFormat = "2006-01-02"
	// MonthFormat is the standard month format (YYYY-MM) used for monthly data.
	MonthFormat = "2006-01"
	// YearFormat is the standard year format (YYYY) used for yearly data.
	YearFormat = "2006"
	// TimeFormat is the RFC3339 format used for timestamps.
	TimeFormat = time.RFC3339
	// TimeFormatNano is the RFC3339 format with nanoseconds for high precision.
	TimeFormatNano = time.RFC3339Nano
)
