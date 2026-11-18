// Package csv writes a report's buckets as CSV: one row per day, month or year with the
// eight energy series and the derived columns (estimates end in _est_kwh). UTF-8, comma
// separator, dot decimals, ISO
// dates, values rounded to two decimals. A bucket without complete days has empty value
// cells (no data is not 0 kWh).
package csv

import (
	stdcsv "encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/report"
	"github.com/dombyte/solis/internal/util"
)

// percent turns a 0..1 share into percent.
const percent = 100

// column is one CSV column: a header and the value of a bucket with data.
type column struct {
	header string
	value  func(report.Summary) *float64
}

// energyColumn reads a total of the bucket.
func energyColumn(header string, field func(report.Energy) float64) column {
	return column{header, func(s report.Summary) *float64 {
		v := field(s.Total)
		return &v
	}}
}

// ratioColumn reads a share of the bucket in percent.
func ratioColumn(header string, field func(report.Summary) *float64) column {
	return column{header, func(s report.Summary) *float64 {
		v := field(s)
		if v == nil {
			return nil
		}
		p := *v * percent
		return &p
	}}
}

// valueColumns lists the value columns after the bucket columns.
func valueColumns() []column {
	return []column{
		energyColumn("pv_kwh", func(e report.Energy) float64 { return e.PV }),
		energyColumn("consumption_kwh", func(e report.Energy) float64 { return e.Consumption }),
		energyColumn("household_kwh", func(e report.Energy) float64 { return e.Household }),
		energyColumn("backup_kwh", func(e report.Energy) float64 { return e.Backup }),
		energyColumn("grid_import_kwh", func(e report.Energy) float64 { return e.Import }),
		energyColumn("grid_export_kwh", func(e report.Energy) float64 { return e.Export }),
		energyColumn("battery_charge_kwh", func(e report.Energy) float64 { return e.Charge }),
		energyColumn("battery_discharge_kwh",
			func(e report.Energy) float64 { return e.Discharge }),
		// _est: estimated from daily totals (see the PDF's last page).
		{"pv_direct_use_est_kwh", func(s report.Summary) *float64 { return &s.DirectUse }},
		{"battery_charge_from_pv_est_kwh", func(s report.Summary) *float64 {
			return &s.ChargeFromPV
		}},
		ratioColumn("self_sufficiency_pct",
			func(s report.Summary) *float64 { return s.SelfSufficiency }),
		ratioColumn("self_consumption_pct",
			func(s report.Summary) *float64 { return s.SelfConsumption }),
	}
}

// Write writes the buckets of r to w.
func Write(w io.Writer, r report.Report) error {
	cols := valueColumns()
	cw := stdcsv.NewWriter(w)
	header := []string{"period", "from", "to", "partial", "days", "complete_days"}
	for _, c := range cols {
		header = append(header, c.header)
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("csv: write header: %w", err)
	}
	for _, b := range r.Buckets {
		if err := cw.Write(row(b, cols)); err != nil {
			return fmt.Errorf("csv: write %s: %w", b.Key, err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("csv: flush: %w", err)
	}
	return nil
}

// row renders one bucket.
func row(b report.Bucket, cols []column) []string {
	s := b.Summary
	out := []string{
		b.Key, b.From.Format(period.DayLayout), b.To.Format(period.DayLayout),
		strconv.FormatBool(b.Partial), strconv.Itoa(s.Days), strconv.Itoa(s.CompleteDays),
	}
	for _, c := range cols {
		out = append(out, cell(s, c))
	}
	return out
}

// cell formats one value; empty without data.
func cell(s report.Summary, c column) string {
	if s.CompleteDays == 0 {
		return ""
	}
	v := c.value(s)
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(util.RoundTo2DecimalPlaces(*v), 'f', 2, 64)
}
