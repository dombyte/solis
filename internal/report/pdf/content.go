package pdf

import (
	"fmt"
	"math"
	"strings"

	"github.com/dombyte/solis/internal/report"
	"github.com/dombyte/solis/internal/util"
)

// maxListedDays caps the dates spelled out in a note.
const maxListedDays = 5

// figureLabel names a key figure.
func figureLabel(id string) string {
	labels := map[string]string{
		"pv": "PV yield", "consumption": "Consumption", "self_sufficiency": "Self-sufficiency",
		"self_consumption": "Self-consumption", "grid_import": "Grid import",
		"grid_export": "Grid export", "battery_charge": "Battery charged",
		"battery_discharge": "Battery discharged",
	}
	if l, ok := labels[id]; ok {
		return l
	}
	return id
}

// figureValue formats a figure value without unit.
func figureValue(v *float64, k report.FigureKind) string {
	switch {
	case v == nil:
		return noValue
	case k == report.KindRatio:
		return number(*v * percent)
	default:
		return number(*v)
	}
}

// figureUnit is "%" for ratios, "kWh" otherwise.
func figureUnit(k report.FigureKind) string {
	if k == report.KindRatio {
		return "%"
	}
	return "kWh"
}

// tile is one key figure tile.
type tile struct {
	label, value, unit string
	// delta and base are empty without a comparison.
	delta, base string
	deltaColor  uint32
}

// tiles builds the key figure tiles of r.
func tiles(r report.Report) []tile {
	out := make([]tile, len(r.Figures))
	for i, f := range r.Figures {
		t := tile{label: figureLabel(f.ID), value: figureValue(f.Value, f.Kind)}
		if f.Value != nil {
			t.unit = figureUnit(f.Kind)
		}
		if r.Comparison != nil && !noCommonDays(r.Comparison) {
			t.delta, t.deltaColor = change(f, true)
			t.base = tileBase(f, *r.Comparison)
		}
		out[i] = t
	}
	return out
}

// tileBase says what a tile's change refers to: the comparison value, or with a matched
// comparison the number of compared days (the tile shows the whole period, the change
// only those days; the header names them and the comparison table lists both sides).
// Kept short: it shares one tile line with the change.
func tileBase(f report.Figure, c report.Comparison) string {
	switch {
	case !c.Matched:
		return "vs " + withUnit(f.Base, f.Kind)
	case f.Change == nil:
		return ""
	default:
		return fmt.Sprintf("on %d %s", c.MatchedDays, plural(c.MatchedDays, "day", "days"))
	}
}

// withUnit formats an optional figure value with its unit.
func withUnit(v *float64, k report.FigureKind) string {
	if v == nil {
		return noValue
	}
	return figureValue(v, k) + " " + figureUnit(k)
}

// change formats a figure's change ("↑ 8.8 %", "↓ 0.7 pp"; with arrows=false "+8.8 %")
// and picks its color by the figure's better direction.
func change(f report.Figure, arrows bool) (string, uint32) {
	if f.Change == nil {
		return "no data", colorMuted
	}
	v, unit := changeValue(f)
	mag := number(math.Abs(v))
	if mag == "0.0" {
		return "±0.0" + unit, colorMuted
	}
	sign := map[bool]string{true: "+", false: minus}[v > 0]
	if arrows {
		sign = map[bool]string{true: "↑ ", false: "↓ "}[v > 0]
	}
	return sign + mag + unit, changeColor(f.Better, v > 0)
}

// changeValue returns the shown change and its unit: percentage points for ratios,
// percent for energy (kWh when the base is 0). Point and kWh changes
// are the difference of the two compared values as displayed (rounded), so 98.0 % ->
// 84.3 % reads 13.7 points, never 13.6.
func changeValue(f report.Figure) (float64, string) {
	switch {
	case f.Kind == report.KindRatio:
		return round1(*f.Compared*percent) - round1(*f.Base*percent), " points"
	case f.ChangePct != nil:
		return *f.ChangePct * percent, " %"
	default:
		return round1(*f.Compared) - round1(*f.Base), " kWh"
	}
}

// round1 rounds v to the one decimal the report shows, from the API's two decimals like
// the Reports page (see number).
func round1(v float64) float64 {
	const tenths = 10
	return math.Round(util.RoundTo2DecimalPlaces(v)*tenths) / tenths
}

// changeColor is green for an improvement, red for a deterioration.
func changeColor(better report.Direction, up bool) uint32 {
	switch {
	case better == report.Neutral:
		return colorBody
	case (better == report.HigherIsBetter) == up:
		return colorBetter
	default:
		return colorWorse
	}
}

// highlight is one label/value box on the summary page.
type highlight struct {
	label, value string
}

// highlights builds the summary page highlights.
func highlights(r report.Report) []highlight {
	s, p := r.Summary, r.Patterns
	avg, best := noValue, noValue
	if s.PerDay != nil {
		avg = number(s.PerDay.PV) + " kWh PV · " + number(s.PerDay.Consumption) + " kWh used"
	}
	if p.BestPV != nil {
		best = weekdayLabel(p.BestPV.Date) + " · " + number(p.BestPV.Value) + " kWh"
	}
	return []highlight{
		{"Average per day", avg},
		{"Best PV day", best},
		{lowImportLabel(), fmt.Sprintf("%d of %d days", p.LowImportDays, s.CompleteDays)},
	}
}

// lowImportLabel names the days that needed almost nothing from the grid.
func lowImportLabel() string {
	return "Days under " + number(report.LowImportBelow) + " kWh grid import"
}

// notes explains the estimates and limits behind the energy flow figures.
func notes(r report.Report) []string {
	s := r.Summary
	direct := "* Used directly ≈ PV − export − battery charge from PV, per day. It is what " +
		"remains of the PV yield, so it also contains the PV-side part of the balance " +
		"difference (e.g. the inverter's conversion losses)."
	fromSolar := "“From solar” ≈ consumption − import − “from battery”, measured on the " +
		"consumption side. It differs from “used directly” by the balance difference."
	if len(s.GridChargingDays) > 0 {
		direct += fmt.Sprintf(" On %s the battery took in more than the panels could "+
			"supply: at least %s kWh came from the grid. Grid charging on a day with "+
			"enough sun cannot be seen in daily counters, so the split is an estimate.",
			dayList(s), number(s.GridCharge))
	}
	if gridCharged(s) {
		fromSolar += " The battery energy charged from the grid counts once, as “from " +
			"grid”, so “from battery” ≈ discharge − charge from the grid."
	}
	out := []string{direct, fromSolar}
	if n := batteryNote(s); n != "" {
		out = append(out, n)
	}
	if !s.Complete() {
		out = append(out, dataNote(r))
	}
	if r.TodayExcluded {
		out = append(out, todayNote(r))
	}
	return out
}

// dataNote says how many days have data and, for months or years, explains the * on the
// grid chart's axis.
func dataNote(r report.Report) string {
	s := r.Summary
	out := fmt.Sprintf("Data for %d of %d days; missing days are left out of every figure.",
		s.CompleteDays, s.Days)
	if g := gapNote(r); r.Granularity != report.ByDay && g != "" {
		out += " " + g
	}
	return out
}

// gridCharged reports whether a visible amount (as displayed) of the battery charge came
// from the grid; "from battery" is then an estimate.
func gridCharged(s report.Summary) bool {
	return number(s.GridCharge) != number(0)
}

// todayNote says that the requested period ended today and the report ends the day
// before.
func todayNote(r report.Report) string {
	return "Today is still in progress and not part of this report: it ends on " +
		dayLabel(r.Range.To) + "."
}

// batteryNote explains a missing or implausible discharged ÷ charged value.
func batteryNote(s report.Summary) string {
	switch {
	case s.BatteryRatio == nil:
		return "Discharged ÷ charged and the balance difference need at least 28 " +
			"complete days: the battery level at the start and end of a period is not stored."
	case *s.BatteryRatio > 1:
		return "The counters show more energy discharged from the battery than charged " +
			"into it. No battery returns more than it takes in: the inverter's two battery " +
			"counters are not calibrated against each other, so discharged ÷ charged is " +
			"not an efficiency."
	default:
		return ""
	}
}

// dayList names the grid-charging days ("28 Oct", "3 Oct, 9 Oct and 2 more days").
func dayList(s report.Summary) string {
	days := s.GridChargingDays
	names := make([]string, 0, maxListedDays)
	for i, d := range days {
		if i == maxListedDays {
			break
		}
		names = append(names, d.Format(shortDay))
	}
	out := strings.Join(names, ", ")
	if rest := len(days) - len(names); rest > 0 {
		out += fmt.Sprintf(" and %d more %s", rest, plural(rest, "day", "days"))
	}
	return out
}

// plural picks the singular or plural form for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// noCommonDays reports a comparison without a single day both periods have data for: it
// has nothing to compare, so the report shows no changes instead of rows of "no data".
func noCommonDays(c *report.Comparison) bool {
	return c != nil && c.Matched && c.MatchedDays == 0
}

// matchedLabel names the compared days of a matched comparison (" · 10 matched days");
// empty for a comparison of whole periods.
func matchedLabel(c report.Comparison) string {
	switch {
	case !c.Matched:
		return ""
	case c.MatchedDays == 0:
		return " · no days to compare"
	default:
		return fmt.Sprintf(" · %d matched %s", c.MatchedDays, plural(c.MatchedDays, "day", "days"))
	}
}

// noCompareText says why a comparison without common days compares nothing.
func noCompareText(r report.Report) string {
	switch {
	case r.Comparison.Summary.CompleteDays == 0:
		return "The comparison period has no data, so nothing is compared."
	case r.Summary.CompleteDays == 0:
		return "This period has no data, so nothing is compared."
	default:
		return "The two periods have no days with data in common, so nothing is compared."
	}
}

// matchReason says why a comparison is matched: missing days, or complete periods of
// different lengths.
func matchReason(r report.Report) string {
	if r.Summary.Complete() && r.Comparison.Summary.Complete() {
		return "The periods differ in length"
	}
	return "Days are missing"
}

// comparisonCaption explains how the comparison table compares.
func comparisonCaption(r report.Report) string {
	c := r.Comparison
	days := fmt.Sprintf("(%d of %d days and %d of %d days)", r.Summary.CompleteDays,
		r.Summary.Days, c.Summary.CompleteDays, c.Summary.Days)
	if c.Matched {
		return fmt.Sprintf("%s %s, so both columns and the changes cover only "+
			"the %d %s both periods have data for; the totals of the whole period are on "+
			"page 1. Shares change in percentage points: the difference of the two "+
			"percentages.", matchReason(r), days, c.MatchedDays,
			plural(c.MatchedDays, "day", "days"))
	}
	return "Both periods are complete " + days + ", so totals are compared. Shares change " +
		"in percentage points: the difference of the two percentages."
}
