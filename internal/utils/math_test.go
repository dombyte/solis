package utils

import (
	"encoding/json"
	"testing"
)

func TestFloat64With2Decimals_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"integer", 30.0, "30.00"},
		{"one decimal", 50.1, "50.10"},
		{"two decimals", 50.12, "50.12"},
		{"three decimals", 50.123, "50.12"},
		{"rounds down", 50.124, "50.12"},
		{"zero", 0, "0.00"},
		{"negative", -10.5, "-10.50"},
		{"large number", 12345.6, "12345.60"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Float64With2Decimals(RoundTo2DecimalPlaces(tt.value))
			data, err := json.Marshal(f)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			result := string(data)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
