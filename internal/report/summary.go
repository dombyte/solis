package report

import (
	"time"
)

const (
	// minBalanceDays is the shortest period (complete days) with a battery ratio and a
	// balance difference: the battery level at the start and end is not stored, so over a
	// few days it dominates both values.
	minBalanceDays = 28
	// gridChargeTolerance (kWh) is two counter steps of 0.1 kWh: a day is grid-charged
	// only when the battery took in clearly more than the panels could have supplied.
	gridChargeTolerance = 0.2
	// floatSlack absorbs binary rounding of decimal counter values (4.2 - 4.0 = 0.2000…02).
	floatSlack = 1e-9
)

// Split divides one energy amount into its three parts (kWh) with their shares (0..1).
// PV split: Solar = used directly, Battery = charged from PV, Grid = exported; the parts
// add up to the PV yield. Consumption sources: Solar = from PV, Battery = discharged,
// Grid = imported; the parts add up to the consumption.
type Split struct {
	Solar, Battery, Grid                float64
	SolarShare, BatteryShare, GridShare *float64
}

// newSplit computes the shares of the three parts of their sum.
func newSplit(solar, battery, grid float64) Split {
	sum := solar + battery + grid
	return Split{
		Solar: solar, Battery: battery, Grid: grid,
		SolarShare: share(solar, sum), BatteryShare: share(battery, sum),
		GridShare: share(grid, sum),
	}
}

// Summary is the evaluation of the complete days of one period.
type Summary struct {
	// Days is the number of calendar days of the period, CompleteDays those with data.
	Days, CompleteDays int
	// Total sums the complete days.
	Total Energy
	// PerDay is Total / CompleteDays; nil without complete days.
	PerDay *Energy
	// SelfSufficiency (autarky) = (consumption - import) / consumption, 0..1; nil without
	// consumption.
	SelfSufficiency *float64
	// SelfConsumption (self-consumption rate) = (PV - export) / PV, 0..1; nil without PV.
	SelfConsumption *float64
	// DirectUse sums PV - export - charge from PV per day: the PV energy that was neither
	// exported nor stored. An estimate (daily totals); it also contains the part of the
	// balance difference on the PV side (e.g. inverter conversion losses).
	DirectUse float64
	// ChargeFromPV sums min(charge, PV - export) per day: the battery charge the panels
	// can have supplied. An estimate (daily totals).
	ChargeFromPV float64
	// GridCharge = charge - ChargeFromPV: battery charge that must have come from the
	// grid. A lower bound: grid charging on a day with enough PV cannot be seen.
	GridCharge float64
	// PVSplit: where the PV energy went; Sources: where the consumption came from.
	PVSplit, Sources Split
	// BatteryRatio = discharge / charge, the plain quotient of the two battery counters.
	// It is not an efficiency: the counters are not calibrated against each other (real
	// devices report more discharged than charged over months) and the battery level is
	// not stored. BalanceDiff = energy in - energy out (kWh): the energy the counters do
	// not account for (conversion losses, standby use, counter inaccuracy);
	// BalanceDiffShare = BalanceDiff / energy in. Nil below minBalanceDays complete days.
	BatteryRatio, BalanceDiff, BalanceDiffShare *float64
	// GridChargingDays are the days whose charge exceeded PV - export by more than the
	// counter tolerance.
	GridChargingDays []time.Time
}

// Complete reports whether every day of the period has data.
func (s Summary) Complete() bool {
	return s.CompleteDays == s.Days
}

// summarize evaluates days (complete days of a period with periodDays calendar days).
func summarize(days []Day, periodDays int) Summary {
	s := Summary{Days: periodDays, CompleteDays: len(days)}
	for _, d := range days {
		s.Total = s.Total.add(d.Energy)
		s.addDay(d)
	}
	if s.CompleteDays > 0 {
		perDay := s.Total.scale(1 / float64(s.CompleteDays))
		s.PerDay = &perDay
	}
	t := s.Total
	s.SelfSufficiency = share(t.Consumption-t.Import, t.Consumption)
	s.SelfConsumption = share(t.PV-t.Export, t.PV)
	s.PVSplit = newSplit(s.DirectUse, s.ChargeFromPV, t.Export)
	s.Sources = newSplit(max(0, t.Consumption-t.Import-t.Discharge), t.Discharge, t.Import)
	if s.CompleteDays >= minBalanceDays {
		s.balance()
	}
	return s
}

// addDay splits one day's PV into direct use, charge from PV and export.
func (s *Summary) addDay(d Day) {
	kept := max(0, d.PV-d.Export) // PV used on site (directly or stored)
	fromPV := min(d.Charge, kept)
	s.DirectUse += kept - fromPV
	s.ChargeFromPV += fromPV
	s.GridCharge += d.Charge - fromPV
	if d.Charge-fromPV > gridChargeTolerance+floatSlack {
		s.GridChargingDays = append(s.GridChargingDays, d.Date)
	}
}

// balance fills the battery ratio and the balance difference.
func (s *Summary) balance() {
	t := s.Total
	in := t.PV + t.Import + t.Discharge
	diff := in - (t.Consumption + t.Export + t.Charge)
	s.BalanceDiff = &diff
	s.BalanceDiffShare = quotient(diff, in)
	s.BatteryRatio = quotient(t.Discharge, t.Charge)
}

// quotient returns num / den, or nil when den is not positive.
func quotient(num, den float64) *float64 {
	if den <= 0 {
		return nil
	}
	q := num / den
	return &q
}

// share returns num / den clamped to 0..1, or nil when den is not positive.
func share(num, den float64) *float64 {
	q := quotient(num, den)
	if q != nil {
		*q = min(1, max(0, *q))
	}
	return q
}
