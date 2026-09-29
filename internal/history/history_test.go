package history

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalJSON_RoundsToTwoDecimals(t *testing.T) {
	tests := map[string]struct {
		point any
		want  string
	}{
		"daily": {
			DailyDataPoint{Date: "2026-08-05", Value: 1.005, RawValue: 10},
			`{"date":"2026-08-05","value":1.0,"raw_value":10}`,
		},
		"monthly": {
			MonthlyDataPoint{Month: "2026-08", Value: 2.499},
			`{"month":"2026-08","value":2.5,"raw_value":0}`,
		},
		"yearly": {
			YearlyDataPoint{Year: "2026", Value: 1.23456, RawValue: 1.23456},
			`{"year":"2026","value":1.23,"raw_value":1.23}`,
		},
		"total": {
			TotalDataPoint{Value: 3.333, Timestamp: "t"},
			`{"value":3.33,"raw_value":0,"timestamp":"t"}`,
		},
		"status": {
			ErrorDataPoint{Timestamp: "t", RawValue: 4},
			`{"timestamp":"t","raw_value":4}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			b, err := json.Marshal(tt.point)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(b))
		})
	}
}
