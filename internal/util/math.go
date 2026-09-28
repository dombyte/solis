// Package util provides common utility functions for the Solis monitor application.
package util

import (
	"fmt"
	"math"
)

// twoDecimals is the scale for rounding to 2 decimal places.
const twoDecimals = 100

// RoundTo2DecimalPlaces rounds a value to exactly 2 decimal places.
// This ensures consistent display format across all values.
func RoundTo2DecimalPlaces(value float64) float64 {
	return math.Round(value*twoDecimals) / twoDecimals
}

// Float64With2Decimals is a float64 that marshals to JSON with exactly 2 decimal places.
// This ensures values like 50.1 are displayed as 50.10 and 30 as 30.00.
type Float64With2Decimals float64

// MarshalJSON implements json.Marshaler to format the float with exactly 2 decimal places.
func (f Float64With2Decimals) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.2f", f)), nil
}
