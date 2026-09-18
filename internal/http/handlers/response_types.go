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

	tr.Start, err = parseTimeString(startStr, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		return TimeRange{}, err
	}
	tr.End, err = parseTimeString(endStr, time.Now())
	if err != nil {
		return TimeRange{}, err
	}

	return tr, nil
}

// parseTimeString parses a time string in various formats.
// Tries to parse as date (YYYY-MM-DD), month (YYYY-MM), or year (YYYY).
// Returns the default value if the input string is empty.
//

func parseTimeString(timeStr string, defaultTime time.Time) (time.Time, error) {
	if timeStr == "" {
		return defaultTime, nil
	}

	// Try to parse as date (YYYY-MM-DD)
	t, err := time.Parse(solis.DateFormat, timeStr)
	if err != nil {
		// Try to parse as month (YYYY-MM)
		t, err = time.Parse(solis.MonthFormat, timeStr)
		if err != nil {
			// Try to parse as year (YYYY)
			t, err = time.Parse(solis.YearFormat, timeStr)
			if err != nil {
				return time.Time{}, err
			}
		}
	}
	return t, nil
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
