// Package report is the pure energy report model: complete daily rows of the eight energy
// series go in, sums, ratios, buckets, patterns and key figures (with an optional
// comparison period) come out. No I/O, no clock, no logging; renderers (JSON, CSV, PDF)
// only format what Build returns.
//
// Every ratio is computed from the summed energy of a period, never as an average of
// daily ratios. Days without a row for every series are excluded from all figures and
// counted as missing, so the sums of the remaining days stay consistent with each other.
package report

import (
	"fmt"
	"sort"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// Energy holds the eight energy series of one day or a sum of days, in kWh.
type Energy struct {
	PV float64
	// Consumption is the inverter's counter of all loads. It is not exactly Household +
	// Backup: the inverter counts a small remainder it assigns to neither (OtherLoads).
	Consumption float64
	Household   float64
	Backup      float64
	Import      float64
	Export      float64
	Charge      float64
	Discharge   float64
}

// add returns the field-wise sum of e and o.
func (e Energy) add(o Energy) Energy {
	return Energy{
		PV: e.PV + o.PV, Consumption: e.Consumption + o.Consumption,
		Household: e.Household + o.Household, Backup: e.Backup + o.Backup,
		Import: e.Import + o.Import, Export: e.Export + o.Export,
		Charge: e.Charge + o.Charge, Discharge: e.Discharge + o.Discharge,
	}
}

// OtherLoads is the consumption the inverter assigns neither to the household loads nor
// to the backup output (Consumption - Household - Backup); the counters are separate, so
// it can be slightly negative.
func (e Energy) OtherLoads() float64 {
	return e.Consumption - e.Household - e.Backup
}

// scale returns every field of e multiplied by f.
func (e Energy) scale(f float64) Energy {
	return Energy{
		PV: e.PV * f, Consumption: e.Consumption * f, Household: e.Household * f,
		Backup: e.Backup * f, Import: e.Import * f, Export: e.Export * f,
		Charge: e.Charge * f, Discharge: e.Discharge * f,
	}
}

// Day is one complete day: every series has a stored daily row.
type Day struct {
	// Date is the local calendar day as midnight UTC (calendar math without DST).
	Date time.Time
	Energy
}

// series maps a daily register key to its Energy field.
type series struct {
	key   string
	field func(*Energy) *float64
}

// allSeries lists the eight daily energy registers a report reads.
func allSeries() []series {
	return []series{
		{"pv_energy_daily", func(e *Energy) *float64 { return &e.PV }},
		{"energy_consumption_daily", func(e *Energy) *float64 { return &e.Consumption }},
		{"household_energy_daily", func(e *Energy) *float64 { return &e.Household }},
		{"backup_energy_daily", func(e *Energy) *float64 { return &e.Backup }},
		{"grid_import_daily", func(e *Energy) *float64 { return &e.Import }},
		{"grid_export_daily", func(e *Energy) *float64 { return &e.Export }},
		{"battery_charge_daily", func(e *Energy) *float64 { return &e.Charge }},
		{"battery_discharge_daily", func(e *Energy) *float64 { return &e.Discharge }},
	}
}

// DailyKeys returns the daily register keys a report needs, one query each.
func DailyKeys() []string {
	ss := allSeries()
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.key
	}
	return out
}

// Assemble turns stored daily values (key -> YYYY-MM-DD -> kWh) into the complete days,
// oldest first. A day missing any series is left out.
func Assemble(values map[string]map[string]float64) ([]Day, error) {
	ss := allSeries()
	var out []Day
	for date := range values[ss[0].key] {
		d, ok := assembleDay(ss, values, date)
		if !ok {
			continue
		}
		t, err := time.Parse(period.DayLayout, date)
		if err != nil {
			return nil, fmt.Errorf("report: parse day %q: %w", date, err)
		}
		d.Date = t
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

// assembleDay collects date's value of every series; ok is false when one is missing.
func assembleDay(ss []series, values map[string]map[string]float64, date string) (Day, bool) {
	var d Day
	for _, s := range ss {
		v, ok := values[s.key][date]
		if !ok {
			return Day{}, false
		}
		*s.field(&d.Energy) = v
	}
	return d, true
}

// Input is everything Build needs.
type Input struct {
	// Range is the reported period.
	Range Range
	// Granularity groups the buckets.
	Granularity Granularity
	// Days are the complete days (days outside Range are ignored).
	Days []Day
	// Compare is the comparison period; nil = no comparison.
	Compare *CompareInput
	// TodayExcluded says the requested range ended today and Range was cut to end
	// yesterday (Range.Finished).
	TodayExcluded bool
}

// CompareInput is the comparison period and its complete days.
type CompareInput struct {
	Mode  CompareMode
	Range Range
	Days  []Day
}

// Report is the computed report.
type Report struct {
	Range       Range
	Granularity Granularity
	// Summary covers the whole range.
	Summary  Summary
	Patterns Patterns
	// Buckets are the range grouped by Granularity, oldest first, empty ones included.
	Buckets []Bucket
	// Comparison is nil without a comparison period.
	Comparison *Comparison
	// Figures are the key figures, with changes when there is a comparison.
	Figures []Figure
	// TodayExcluded is true when the requested range ended today: today is still in
	// progress and not part of the report, which ends yesterday.
	TodayExcluded bool
}

// Comparison is the comparison period.
type Comparison struct {
	Mode  CompareMode
	Range Range
	// Summary covers every complete day of the comparison period.
	Summary Summary
	// Matched is true when the periods cannot be compared as wholes: either has missing
	// days, or they differ in length (March against February, a leap year). The figures
	// then compare only the days both periods have data for (the same date a year
	// earlier, or the same position in the previous period), so both sides cover the
	// same days; see Figure.Compared and Figure.Base.
	Matched bool
	// MatchedDays is the number of such day pairs; 0 unless Matched.
	MatchedDays int
}

// sides are the two summaries the changes of a comparison are computed from.
type sides struct {
	cur, base Summary
}

// Build computes the report.
func Build(in Input) Report {
	days := within(in.Days, in.Range)
	r := Report{
		Range:         in.Range,
		Granularity:   in.Granularity,
		Summary:       summarize(days, in.Range.Days()),
		Patterns:      patterns(days),
		Buckets:       buckets(in.Range, in.Granularity, days),
		TodayExcluded: in.TodayExcluded,
	}
	var cmp *sides
	if in.Compare != nil {
		r.Comparison, cmp = compare(in.Compare, in.Range, days, r.Summary)
	}
	r.Figures = figures(r.Summary, cmp)
	return r
}

// compare evaluates the comparison period c of the range r (days and cur are r's
// complete days and their summary) and returns the two sides the changes are computed
// from: both whole periods when they are complete and equally long, otherwise only their
// matched days.
func compare(c *CompareInput, r Range, days []Day, cur Summary) (*Comparison, *sides) {
	baseDays := within(c.Days, c.Range)
	base := summarize(baseDays, c.Range.Days())
	out := &Comparison{Mode: c.Mode, Range: c.Range, Summary: base}
	if cur.Complete() && base.Complete() && cur.Days == base.Days {
		return out, &sides{cur, base}
	}
	a, b := matchDays(days, baseDays, func(day time.Time) (time.Time, bool) {
		return partner(day, c.Mode, r, c.Range)
	})
	out.Matched, out.MatchedDays = true, len(a)
	return out, &sides{summarize(a, len(a)), summarize(b, len(b))}
}

// matchDays returns the days of cur whose partner day is in base, and those partners,
// pairwise in the same order.
func matchDays(cur, base []Day, partner func(time.Time) (time.Time, bool)) (a, b []Day) {
	byDate := make(map[time.Time]Day, len(base))
	for _, d := range base {
		byDate[d.Date] = d
	}
	for _, d := range cur {
		p, ok := partner(d.Date)
		if !ok {
			continue
		}
		if o, found := byDate[p]; found {
			a, b = append(a, d), append(b, o)
		}
	}
	return a, b
}

// partner returns the day of the comparison range cmp that day (of r) is compared
// with: the same date a year earlier for CompareLastYear (29 February has none),
// otherwise the day at the same position in cmp (none past its end, e.g. the 31st of a
// month compared with a 30-day month).
func partner(day time.Time, m CompareMode, r, cmp Range) (time.Time, bool) {
	if m == CompareLastYear {
		p := yearBefore(day)
		return p, p.Day() == day.Day()
	}
	p := cmp.From.AddDate(0, 0, Range{From: r.From, To: day}.Days()-1)
	return p, !p.After(cmp.To)
}

// within returns the days of r, oldest first.
func within(days []Day, r Range) []Day {
	out := make([]Day, 0, len(days))
	for _, d := range days {
		if r.Contains(d.Date) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}
