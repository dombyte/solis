// Package history holds the read-model rows of the history tables (daily, monthly,
// yearly, total, status) shared by storage, the read service and the HTTP handlers.
package history

import (
	"encoding/json"

	"github.com/dombyte/solis/internal/util"
)

// Values are stored at full precision; these JSON encoders round to two decimals for
// clients (spec §5).

// ErrorDataPoint represents a single error/fault data point.
type ErrorDataPoint struct {
	Timestamp   string  `json:"timestamp"`
	RawValue    float64 `json:"raw_value"`
	StringValue string  `json:"string_value,omitempty"`
}

// DailyDataPoint represents a daily value.
type DailyDataPoint struct {
	Date     string  `json:"date"`
	Value    float64 `json:"value"`
	RawValue float64 `json:"raw_value"`
}

// MonthlyDataPoint represents a monthly value.
type MonthlyDataPoint struct {
	Month    string  `json:"month"`
	Value    float64 `json:"value"`
	RawValue float64 `json:"raw_value"`
}

// YearlyDataPoint represents a yearly value.
type YearlyDataPoint struct {
	Year     string  `json:"year"`
	Value    float64 `json:"value"`
	RawValue float64 `json:"raw_value"`
}

// TotalDataPoint represents a total (lifetime) value.
type TotalDataPoint struct {
	Value     float64 `json:"value"`
	RawValue  float64 `json:"raw_value"`
	Timestamp string  `json:"timestamp"`
}

// roundedPoint is the JSON shape of a period point with values rounded to 2 decimals.
type roundedPoint struct {
	Date      string                    `json:"date,omitempty"`
	Month     string                    `json:"month,omitempty"`
	Year      string                    `json:"year,omitempty"`
	Value     util.Float64With2Decimals `json:"value"`
	RawValue  util.Float64With2Decimals `json:"raw_value"`
	Timestamp string                    `json:"timestamp,omitempty"`
}

func round(v float64) util.Float64With2Decimals {
	return util.Float64With2Decimals(util.RoundTo2DecimalPlaces(v))
}

// MarshalJSON rounds values to two decimals.
func (d DailyDataPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(roundedPoint{
		Date: d.Date, Value: round(d.Value),
		RawValue: round(d.RawValue),
	})
}

// MarshalJSON rounds values to two decimals.
func (m MonthlyDataPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(roundedPoint{
		Month: m.Month, Value: round(m.Value),
		RawValue: round(m.RawValue),
	})
}

// MarshalJSON rounds values to two decimals.
func (y YearlyDataPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(roundedPoint{
		Year: y.Year, Value: round(y.Value),
		RawValue: round(y.RawValue),
	})
}

// MarshalJSON rounds values to two decimals.
func (t TotalDataPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Value     util.Float64With2Decimals `json:"value"`
		RawValue  util.Float64With2Decimals `json:"raw_value"`
		Timestamp string                    `json:"timestamp"`
	}{round(t.Value), round(t.RawValue), t.Timestamp})
}

// MarshalJSON rounds the raw value to two decimals.
func (e ErrorDataPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Timestamp   string                    `json:"timestamp"`
		RawValue    util.Float64With2Decimals `json:"raw_value"`
		StringValue string                    `json:"string_value,omitempty"`
	}{e.Timestamp, round(e.RawValue), e.StringValue})
}
