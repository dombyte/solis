// Package handlers provides HTTP request handlers for the Solis monitor API.
package handlers

import (
	"time"

	"github.com/dombyte/solis/internal/solis"
)

// DataResponse represents a single value response for current or total registers.
// This is the minimal schema - only fields with data are included.
type DataResponse struct {
	// Key is the register key
	Key string `json:"key,omitempty"`
	// Name is the human-readable name of the register
	Name string `json:"name,omitempty"`
	// Unit is the unit of measurement
	Unit string `json:"unit,omitempty"`
	// Value is the decoded/scaled value
	Value float64 `json:"value,omitempty"`
	// RawValue is the raw value from the device
	RawValue float64 `json:"raw_value,omitempty"`
	// Timestamp is when the value was last updated
	Timestamp string `json:"timestamp,omitempty"`
	// StringValue is for string-type registers
	StringValue string `json:"string_value,omitempty"`
	// StatusDecoded is for status/fault registers
	StatusDecoded interface{} `json:"status_decoded,omitempty"`
}

// buildDataResponse creates a DataResponse from a solis.Value.
// It handles optional fields like StringValue and StatusDecoded.
func buildDataResponse(key string, value *solis.Value) DataResponse {
	resp := DataResponse{
		Key:       key,
		Name:      value.Name,
		Unit:      value.Unit,
		Value:     value.DecodedValue,
		RawValue:  value.RawValue,
		Timestamp: value.Timestamp.Format(time.RFC3339),
	}
	if value.StringValue != "" {
		resp.StringValue = value.StringValue
	}
	if value.StatusDecoded != nil {
		resp.StatusDecoded = value.StatusDecoded
	}
	return resp
}

// TimeRange represents a time range for historical queries.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// ParseTimeRange parses start and end query parameters into a TimeRange.
// Supports formats: YYYY-MM-DD for daily, YYYY-MM for monthly, YYYY for yearly.
func ParseTimeRange(startStr, endStr string) (TimeRange, error) {
	var tr TimeRange
	var err error

	if startStr != "" {
		// Try to parse as date (YYYY-MM-DD)
		tr.Start, err = time.Parse(solis.DateFormat, startStr)
		if err != nil {
			// Try to parse as month (YYYY-MM)
			tr.Start, err = time.Parse(solis.MonthFormat, startStr)
			if err != nil {
				// Try to parse as year (YYYY)
				tr.Start, err = time.Parse(solis.YearFormat, startStr)
				if err != nil {
					return TimeRange{}, err
				}
			}
		}
	} else {
		// Default start: 30 days ago for daily, 12 months ago for monthly, 10 years ago for yearly
		tr.Start = time.Now().Add(-30 * 24 * time.Hour)
	}

	if endStr != "" {
		// Try to parse as date (YYYY-MM-DD)
		tr.End, err = time.Parse(solis.DateFormat, endStr)
		if err != nil {
			// Try to parse as month (YYYY-MM)
			tr.End, err = time.Parse(solis.MonthFormat, endStr)
			if err != nil {
				// Try to parse as year (YYYY)
				tr.End, err = time.Parse(solis.YearFormat, endStr)
				if err != nil {
					return TimeRange{}, err
				}
			}
		}
	} else {
		tr.End = time.Now()
	}

	return tr, nil
}

// GetKeyType determines the type of a register key using the canonical
// register definitions from the solis package.
// Returns "daily", "monthly", "yearly", "total", or "current".
func GetKeyType(key string) string {
	// Check in order of specificity: total, yearly, monthly, daily
	if solis.IsTotalRegister(key) {
		return "total"
	}

	if solis.IsYearlyRegister(key) {
		return "yearly"
	}

	if solis.IsMonthlyRegister(key) {
		return "monthly"
	}

	if solis.IsDailyRegister(key) {
		return "daily"
	}

	// Default to current
	return "current"
}
