// Package utils provides common utility functions for the Solis monitor application.
package utils

import "math"

// RoundTo2DecimalPlaces rounds a value to exactly 2 decimal places.
// This ensures consistent display format across all values.
func RoundTo2DecimalPlaces(value float64) float64 {
	return math.Round(value*100) / 100
}
