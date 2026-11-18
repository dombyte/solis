package report

import (
	"time"
)

// LowImportBelow (kWh) is the daily grid import below which a day counts as a day with
// almost no grid import. The inverter draws 0.1-0.3 kWh from the grid even on days the
// house runs on solar and battery alone, so a threshold of zero would never match.
const LowImportBelow = 0.5

// DayValue is one day and a value of it.
type DayValue struct {
	Date  time.Time
	Value float64
}

// Patterns are day-level observations of the complete days of the range.
type Patterns struct {
	// Extremes; nil without complete days. Ties go to the earliest day.
	BestPV, LowestPV, MaxConsumption, MaxImport *DayValue
	// WeekdayConsumption / WeekendConsumption are the average consumption per complete
	// Monday-Friday / Saturday-Sunday; nil without such days.
	WeekdayConsumption, WeekendConsumption *float64
	WeekdayDays, WeekendDays               int
	// LowImportDays imported less than LowImportBelow from the grid.
	LowImportDays int
}

// patterns evaluates days (complete days, oldest first).
func patterns(days []Day) Patterns {
	p := Patterns{
		BestPV:         extreme(days, func(d Day) float64 { return d.PV }, true),
		LowestPV:       extreme(days, func(d Day) float64 { return d.PV }, false),
		MaxConsumption: extreme(days, func(d Day) float64 { return d.Consumption }, true),
		MaxImport:      extreme(days, func(d Day) float64 { return d.Import }, true),
	}
	var weekday, weekend float64
	for _, d := range days {
		if isWeekend(d.Date) {
			weekend += d.Consumption
			p.WeekendDays++
		} else {
			weekday += d.Consumption
			p.WeekdayDays++
		}
		if d.Import < LowImportBelow {
			p.LowImportDays++
		}
	}
	p.WeekdayConsumption = quotient(weekday, float64(p.WeekdayDays))
	p.WeekendConsumption = quotient(weekend, float64(p.WeekendDays))
	return p
}

// extreme returns the day with the highest (or lowest) value; the earliest wins ties.
func extreme(days []Day, value func(Day) float64, highest bool) *DayValue {
	var best *DayValue
	for _, d := range days {
		v := value(d)
		if best == nil || (highest && v > best.Value) || (!highest && v < best.Value) {
			best = &DayValue{Date: d.Date, Value: v}
		}
	}
	return best
}

// isWeekend reports whether day is a Saturday or Sunday.
func isWeekend(day time.Time) bool {
	wd := day.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}
