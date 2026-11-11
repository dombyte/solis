package httphandler

import (
	"time"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/report"
	"github.com/dombyte/solis/internal/util"
)

// percent turns a 0..1 share into percent (and a share difference into points).
const percent = 100

// Report JSON (docs/src/openapi.yaml): energy in kWh, shares in percent, changes of
// ratios in percentage points; values rounded to two decimals; null = no data.

// num is an optional rounded number (null in JSON).
type num = *util.Float64With2Decimals

// ReportResponse is the /api/report JSON body.
type ReportResponse struct {
	From        string             `json:"from"`
	To          string             `json:"to"`
	Granularity string             `json:"granularity"`
	Summary     SummaryJSON        `json:"summary"`
	Figures     []FigureJSON       `json:"figures"`
	Comparison  *ComparisonJSON    `json:"comparison"`
	Patterns    PatternsJSON       `json:"patterns"`
	Buckets     []ReportBucketJSON `json:"buckets"`
	// TodayExcluded: the requested range ended today; the report ends yesterday (To).
	TodayExcluded bool `json:"today_excluded"`
}

// EnergyJSON holds the eight energy series in kWh.
type EnergyJSON struct {
	PV               util.Float64With2Decimals `json:"pv"`
	Consumption      util.Float64With2Decimals `json:"consumption"`
	Household        util.Float64With2Decimals `json:"household"`
	Backup           util.Float64With2Decimals `json:"backup"`
	GridImport       util.Float64With2Decimals `json:"grid_import"`
	GridExport       util.Float64With2Decimals `json:"grid_export"`
	BatteryCharge    util.Float64With2Decimals `json:"battery_charge"`
	BatteryDischarge util.Float64With2Decimals `json:"battery_discharge"`
}

// SplitJSON is a three-part split in kWh and percent.
type SplitJSON struct {
	Solar      util.Float64With2Decimals `json:"solar"`
	Battery    util.Float64With2Decimals `json:"battery"`
	Grid       util.Float64With2Decimals `json:"grid"`
	SolarPct   num                       `json:"solar_pct"`
	BatteryPct num                       `json:"battery_pct"`
	GridPct    num                       `json:"grid_pct"`
}

// SummaryJSON is the evaluation of one period; values are null without complete days.
type SummaryJSON struct {
	Days               int         `json:"days"`
	CompleteDays       int         `json:"complete_days"`
	Total              *EnergyJSON `json:"total"`
	PerDay             *EnergyJSON `json:"per_day"`
	SelfSufficiencyPct num         `json:"self_sufficiency_pct"`
	SelfConsumptionPct num         `json:"self_consumption_pct"`
	PVDirectUse        num         `json:"pv_direct_use"`
	PVSplit            *SplitJSON  `json:"pv_split"`
	Sources            *SplitJSON  `json:"consumption_sources"`
	ChargeFromPV       num         `json:"battery_charge_from_pv"`
	GridChargeMin      num         `json:"battery_charge_from_grid_min"`
	BatteryRatioPct    num         `json:"battery_ratio_pct"`
	BalanceDiff        num         `json:"balance_difference"`
	BalanceDiffPct     num         `json:"balance_difference_pct"`
	GridChargingDays   []string    `json:"grid_charging_days"`
}

// FigureJSON is one key figure. Change is Compared - Base; with a matched comparison
// both cover only the matched days, otherwise Compared equals Value.
type FigureJSON struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Better    string `json:"better"`
	Value     num    `json:"value"`
	Compared  num    `json:"compared"`
	Base      num    `json:"base"`
	Change    num    `json:"change"`
	ChangePct num    `json:"change_pct"`
}

// ComparisonJSON is the comparison period.
type ComparisonJSON struct {
	Mode    string      `json:"mode"`
	From    string      `json:"from"`
	To      string      `json:"to"`
	Summary SummaryJSON `json:"summary"`
	// Matched: days are missing, so the figures compare MatchedDays pairs of days.
	Matched     bool `json:"matched"`
	MatchedDays int  `json:"matched_days"`
}

// DayValueJSON is a day and its value in kWh.
type DayValueJSON struct {
	Date  string                    `json:"date"`
	Value util.Float64With2Decimals `json:"value"`
}

// PatternsJSON are the day-level observations.
type PatternsJSON struct {
	BestPV             *DayValueJSON `json:"best_pv"`
	LowestPV           *DayValueJSON `json:"lowest_pv"`
	MaxConsumption     *DayValueJSON `json:"max_consumption"`
	MaxImport          *DayValueJSON `json:"max_import"`
	WeekdayConsumption num           `json:"weekday_consumption_per_day"`
	WeekendConsumption num           `json:"weekend_consumption_per_day"`
	WeekdayDays        int           `json:"weekday_days"`
	WeekendDays        int           `json:"weekend_days"`
	LowImportDays      int           `json:"low_import_days"`
}

// ReportBucketJSON is one day, month or year of the range.
type ReportBucketJSON struct {
	Key     string      `json:"key"`
	From    string      `json:"from"`
	To      string      `json:"to"`
	Partial bool        `json:"partial"`
	Summary SummaryJSON `json:"summary"`
}

// NewReportResponse renders a report.
func NewReportResponse(r report.Report) ReportResponse {
	out := ReportResponse{
		From: day(r.Range.From), To: day(r.Range.To), Granularity: r.Granularity.String(),
		Summary:       newSummaryJSON(r.Summary),
		Figures:       make([]FigureJSON, len(r.Figures)),
		Patterns:      newPatternsJSON(r.Patterns),
		Buckets:       make([]ReportBucketJSON, len(r.Buckets)),
		TodayExcluded: r.TodayExcluded,
	}
	for i, f := range r.Figures {
		out.Figures[i] = newFigureJSON(f)
	}
	for i, b := range r.Buckets {
		out.Buckets[i] = ReportBucketJSON{
			Key: b.Key, From: day(b.From), To: day(b.To), Partial: b.Partial,
			Summary: newSummaryJSON(b.Summary),
		}
	}
	if c := r.Comparison; c != nil {
		out.Comparison = &ComparisonJSON{
			Mode: c.Mode.String(), From: day(c.Range.From), To: day(c.Range.To),
			Summary: newSummaryJSON(c.Summary), Matched: c.Matched, MatchedDays: c.MatchedDays,
		}
	}
	return out
}

func newSummaryJSON(s report.Summary) SummaryJSON {
	out := SummaryJSON{
		Days: s.Days, CompleteDays: s.CompleteDays,
		SelfSufficiencyPct: optScaled(s.SelfSufficiency, percent),
		SelfConsumptionPct: optScaled(s.SelfConsumption, percent),
		BatteryRatioPct:    optScaled(s.BatteryRatio, percent),
		BalanceDiff:        optScaled(s.BalanceDiff, 1),
		BalanceDiffPct:     optScaled(s.BalanceDiffShare, percent),
		GridChargingDays:   make([]string, len(s.GridChargingDays)),
	}
	for i, d := range s.GridChargingDays {
		out.GridChargingDays[i] = day(d)
	}
	if s.CompleteDays == 0 {
		return out
	}
	total := newEnergyJSON(s.Total)
	out.Total, out.PerDay = &total, optEnergy(s.PerDay)
	out.PVDirectUse = optScaled(&s.DirectUse, 1)
	out.ChargeFromPV, out.GridChargeMin = optScaled(&s.ChargeFromPV, 1), optScaled(&s.GridCharge, 1)
	pv, src := newSplitJSON(s.PVSplit), newSplitJSON(s.Sources)
	out.PVSplit, out.Sources = &pv, &src
	return out
}

func newEnergyJSON(e report.Energy) EnergyJSON {
	return EnergyJSON{
		PV: round(e.PV), Consumption: round(e.Consumption), Household: round(e.Household),
		Backup: round(e.Backup), GridImport: round(e.Import), GridExport: round(e.Export),
		BatteryCharge: round(e.Charge), BatteryDischarge: round(e.Discharge),
	}
}

func optEnergy(e *report.Energy) *EnergyJSON {
	if e == nil {
		return nil
	}
	out := newEnergyJSON(*e)
	return &out
}

func newSplitJSON(s report.Split) SplitJSON {
	return SplitJSON{
		Solar: round(s.Solar), Battery: round(s.Battery), Grid: round(s.Grid),
		SolarPct:   optScaled(s.SolarShare, percent),
		BatteryPct: optScaled(s.BatteryShare, percent),
		GridPct:    optScaled(s.GridShare, percent),
	}
}

// newFigureJSON renders a figure: ratios and their changes in percent / points.
func newFigureJSON(f report.Figure) FigureJSON {
	scale := 1.0
	if f.Kind == report.KindRatio {
		scale = percent
	}
	return FigureJSON{
		ID: f.ID, Kind: f.Kind.String(), Better: f.Better.String(),
		Value: optScaled(f.Value, scale), Compared: optScaled(f.Compared, scale),
		Base:   optScaled(f.Base, scale),
		Change: optScaled(f.Change, scale), ChangePct: optScaled(f.ChangePct, percent),
	}
}

func newPatternsJSON(p report.Patterns) PatternsJSON {
	return PatternsJSON{
		BestPV: optDay(p.BestPV), LowestPV: optDay(p.LowestPV),
		MaxConsumption: optDay(p.MaxConsumption), MaxImport: optDay(p.MaxImport),
		WeekdayConsumption: optScaled(p.WeekdayConsumption, 1),
		WeekendConsumption: optScaled(p.WeekendConsumption, 1),
		WeekdayDays:        p.WeekdayDays, WeekendDays: p.WeekendDays,
		LowImportDays: p.LowImportDays,
	}
}

func optDay(d *report.DayValue) *DayValueJSON {
	if d == nil {
		return nil
	}
	return &DayValueJSON{Date: day(d.Date), Value: round(d.Value)}
}

// optScaled returns v * scale rounded, or nil.
func optScaled(v *float64, scale float64) num {
	if v == nil {
		return nil
	}
	r := round(*v * scale)
	return &r
}

func day(t time.Time) string { return t.Format(period.DayLayout) }
