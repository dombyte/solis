package metrics

import (
	"testing"
)

func TestSanitizeMetricName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pv_voltage", "solis_pv_voltage"},
		{"pv-voltage", "solis_pv_voltage"},
		{"pv.voltage", "solis_pv_voltage"},
		{"inverter_temp", "solis_inverter_temp"},
		{"grid-power_active", "solis_grid_power_active"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMetricName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeMetricName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
