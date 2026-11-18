package report

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
)

const delta = 1e-9

func day(s string) time.Time {
	t, err := time.Parse(period.DayLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

func rng(from, to string) Range { return Range{From: day(from), To: day(to)} }

// october is the example month of the report design (docs: PDF mockup): day of month,
// PV, consumption, household, backup, import, export, charge, discharge.
func october() []Day {
	rows := [][9]float64{
		{1, 13.5, 9.4, 8.8, 0.6, 0.8, 4.3, 2.2, 2.0},
		{2, 11.7, 10.4, 10.1, 0.3, 1.1, 1.5, 3.0, 2.5},
		{3, 17.8, 12.9, 12.5, 0.4, 0.4, 4.6, 2.5, 2.3},
		{4, 18.7, 13.3, 13.0, 0.3, 0.0, 5.6, 3.9, 3.3},
		{5, 18.5, 11.3, 10.9, 0.4, 0.9, 7.2, 3.3, 3.0},
		{6, 16.0, 9.2, 8.8, 0.4, 0.7, 6.6, 3.5, 3.1},
		{7, 22.6, 10.3, 9.9, 0.4, 0.8, 12.2, 3.7, 3.4},
		{8, 21.5, 11.5, 10.8, 0.7, 0.0, 10.5, 2.6, 2.5},
		{9, 23.7, 9.4, 8.9, 0.5, 0.4, 13.8, 2.1, 1.9},
		{10, 23.9, 11.5, 10.9, 0.6, 0.8, 12.2, 3.3, 3.0},
		{11, 23.2, 11.9, 11.3, 0.6, 0.7, 11.1, 2.1, 1.9},
		{12, 19.8, 9.8, 9.3, 0.5, 0.0, 9.5, 3.5, 2.9},
		{13, 8.9, 9.2, 8.5, 0.7, 1.3, 0.4, 2.3, 2.0},
		{14, 16.4, 9.2, 8.7, 0.5, 0.3, 6.8, 3.2, 3.0},
		{15, 14.2, 9.8, 9.3, 0.5, 0.3, 4.0, 2.8, 2.6},
		{16, 3.4, 9.5, 9.1, 0.4, 6.4, 0.1, 1.2, 1.1},
		{17, 3.2, 10.6, 10.1, 0.5, 7.6, 0, 1.1, 1.0},
		{18, 7.3, 12.0, 11.4, 0.6, 5.3, 0, 2.6, 2.2},
		{19, 7.6, 11.4, 10.7, 0.7, 4.8, 0.4, 2.7, 2.4},
		{20, 5.3, 9.2, 8.9, 0.3, 4.6, 0.2, 1.9, 1.6},
		{21, 3.1, 9.0, 8.6, 0.4, 6.3, 0.2, 1.1, 1.0},
		{22, 8.9, 10.7, 10.3, 0.4, 2.9, 0.4, 2.6, 2.2},
		{23, 3.1, 11.4, 10.6, 0.8, 8.7, 0.2, 1.1, 1.0},
		{24, 3.1, 11.6, 11.2, 0.4, 9.0, 0.2, 1.1, 0.9},
		{25, 13.3, 12.1, 11.7, 0.4, 0.8, 1.0, 3.2, 2.6},
		{26, 15.2, 11.4, 10.8, 0.6, 1.2, 4.2, 2.6, 2.3},
		{27, 14.2, 10.5, 9.8, 0.7, 0.4, 3.3, 2.7, 2.3},
		{28, 2.1, 11.4, 10.7, 0.7, 11.3, 0.0, 4.0, 3.5},
		{29, 11.7, 9.1, 8.8, 0.3, 0.5, 2.3, 2.6, 2.2},
		{30, 20.2, 10.3, 9.5, 0.8, 0.0, 9.8, 4.2, 4.0},
		{31, 11.6, 11.2, 10.8, 0.4, 0.4, 0.2, 3.4, 3.2},
	}
	out := make([]Day, len(rows))
	for i, r := range rows {
		out[i] = Day{
			Date: time.Date(2026, time.October, int(r[0]), 0, 0, 0, 0, time.UTC),
			Energy: Energy{
				PV: r[1], Consumption: r[2], Household: r[3], Backup: r[4],
				Import: r[5], Export: r[6], Charge: r[7], Discharge: r[8],
			},
		}
	}
	return out
}

func TestBuild_ExampleMonth(t *testing.T) {
	t.Parallel()
	r := Build(Input{Range: rng("2026-10-01", "2026-10-31"), Granularity: ByDay, Days: october()})
	s := r.Summary
	assert.Equal(t, 31, s.Days)
	assert.True(t, s.Complete())
	assert.InDelta(t, 403.7, s.Total.PV, 1e-6)
	assert.InDelta(t, 330.5, s.Total.Consumption, 1e-6)
	assert.InDelta(t, 78.7, s.Total.Import, 1e-6)
	assert.InDelta(t, 132.8, s.Total.Export, 1e-6)
	assert.InDelta(t, 82.1, s.Total.Charge, 1e-6)
	assert.InDelta(t, 72.9, s.Total.Discharge, 1e-6)
	assert.InDelta(t, 1-78.7/330.5, *s.SelfSufficiency, 1e-6)
	assert.InDelta(t, (403.7-132.8)/403.7, *s.SelfConsumption, 1e-6)
	assert.InDelta(t, 403.7/31, s.PerDay.PV, 1e-6)

	// Day 28 charged 4.0 kWh from 2.1 kWh PV: clamped to 0 and flagged.
	// Day 28 charged 4.0 kWh from 2.1 kWh PV: 2.1 from PV, at least 1.9 from the grid.
	assert.InDelta(t, 190.7, s.DirectUse, 1e-6)
	assert.InDelta(t, 82.1-1.9, s.ChargeFromPV, 1e-6)
	assert.InDelta(t, 1.9, s.GridCharge, 1e-6)
	assert.Equal(t, []time.Time{day("2026-10-28")}, s.GridChargingDays)
	// The PV parts add up to the PV yield, and direct + stored is the self-consumption rate.
	assert.InDelta(t, 403.7, s.PVSplit.Solar+s.PVSplit.Battery+s.PVSplit.Grid, 1e-6)
	assert.InDelta(t, 190.7/403.7, *s.PVSplit.SolarShare, 1e-6)
	assert.InDelta(t, 80.2/403.7, *s.PVSplit.BatteryShare, 1e-6)
	assert.InDelta(t, *s.SelfConsumption, *s.PVSplit.SolarShare+*s.PVSplit.BatteryShare, 1e-9)
	// The sources add up to the consumption, and the grid share is 1 - self-sufficiency.
	assert.InDelta(t, 330.5, s.Sources.Solar+s.Sources.Battery+s.Sources.Grid, 1e-6)
	assert.InDelta(t, 1-*s.SelfSufficiency, *s.Sources.GridShare, 1e-9)
	// The 1.9 kWh charged from the grid count as grid, not again as battery.
	assert.InDelta(t, 72.9-1.9, s.Sources.Battery, 1e-6)
	assert.InDelta(t, 178.9+1.9, s.Sources.Solar, 1e-6)
	assert.InDelta(t, 78.7/330.5, *s.Sources.GridShare, 1e-6)
	// So "from solar" and "used directly" differ by exactly the balance difference.
	assert.InDelta(t, *s.BalanceDiff, s.DirectUse-s.Sources.Solar, 1e-6)

	assert.InDelta(t, 72.9/82.1, *s.BatteryRatio, 1e-6)
	assert.InDelta(t, 9.9, *s.BalanceDiff, 1e-6)
	assert.InDelta(t, 9.9/(403.7+78.7+72.9), *s.BalanceDiffShare, 1e-6)
	assert.Len(t, r.Buckets, 31)
}

func TestSummarize_SourcesAddUpToConsumption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		e                    Energy
		solar, battery, grid float64
	}{
		// A night of grid charging: the import fed the loads and the battery, the battery
		// then fed the loads. Counting import and discharge both would give 20 of 10 kWh.
		{"grid charging", Energy{Consumption: 10, Import: 11, Charge: 10, Discharge: 9}, 0, 0, 10},
		{
			"partly grid charged",
			Energy{PV: 2, Consumption: 10, Import: 5, Charge: 4, Discharge: 6},
			1, 4, 5,
		},
		{
			"no grid charging",
			Energy{PV: 8, Consumption: 10, Import: 3, Charge: 2, Discharge: 4},
			3, 4, 3,
		},
		{"import above consumption", Energy{Consumption: 2, Import: 5, Charge: 3}, 0, 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := summarize([]Day{{Date: day("2026-10-28"), Energy: tt.e}}, 1)
			assert.InDelta(t, tt.solar, s.Sources.Solar, delta, "solar")
			assert.InDelta(t, tt.battery, s.Sources.Battery, delta, "battery")
			assert.InDelta(t, tt.grid, s.Sources.Grid, delta, "grid")
			assert.InDelta(t, tt.e.Consumption,
				s.Sources.Solar+s.Sources.Battery+s.Sources.Grid, delta)
			assert.InDelta(t, 1-*s.SelfSufficiency, *s.Sources.GridShare, delta)
		})
	}
}

func TestBuild_ExamplePatterns(t *testing.T) {
	t.Parallel()
	p := Build(Input{Range: rng("2026-10-01", "2026-10-31"), Days: october()}).Patterns
	assert.Equal(t, DayValue{day("2026-10-10"), 23.9}, *p.BestPV)
	assert.Equal(t, DayValue{day("2026-10-28"), 2.1}, *p.LowestPV)
	assert.Equal(t, DayValue{day("2026-10-04"), 13.3}, *p.MaxConsumption)
	assert.Equal(t, DayValue{day("2026-10-28"), 11.3}, *p.MaxImport)
	assert.Equal(t, 22, p.WeekdayDays)
	assert.Equal(t, 9, p.WeekendDays)
	assert.InDelta(t, 223.4/22, *p.WeekdayConsumption, 1e-6)
	assert.InDelta(t, 107.1/9, *p.WeekendConsumption, 1e-6)
	assert.Equal(t, 10, p.LowImportDays, "import below 0.5 kWh; 0.5 itself does not count")
}

func TestBuild_CompareLastYear(t *testing.T) {
	t.Parallel()
	cur := october()
	base := make([]Day, len(cur))
	for i, d := range cur {
		e := d.Energy
		e.PV, e.Import = e.PV/2, e.Import*2
		base[i] = Day{Date: d.Date.AddDate(-1, 0, 0), Energy: e}
	}
	r := Build(Input{
		Range: rng("2026-10-01", "2026-10-31"), Days: cur,
		Compare: &CompareInput{
			Mode: CompareLastYear, Range: rng("2025-10-01", "2025-10-31"), Days: base,
		},
	})
	require.NotNil(t, r.Comparison)
	assert.False(t, r.Comparison.Matched)
	assert.Zero(t, r.Comparison.MatchedDays)
	pv := figure(t, r, "pv")
	assert.Equal(t, pv.Value, pv.Compared, "complete periods compare their totals")
	assert.InDelta(t, 403.7/2, *pv.Base, 1e-6)
	assert.InDelta(t, 403.7/2, *pv.Change, 1e-6)
	assert.InDelta(t, 1.0, *pv.ChangePct, 1e-6)
	imp := figure(t, r, "grid_import")
	assert.InDelta(t, -0.5, *imp.ChangePct, 1e-6)
	ss := figure(t, r, "self_sufficiency")
	assert.InDelta(t, (1-78.7/330.5)-(1-157.4/330.5), *ss.Change, 1e-6)
	assert.Nil(t, ss.ChangePct, "ratios change in percentage points only")
	assert.Equal(t, []string{
		"pv", "consumption", "self_sufficiency", "self_consumption", "grid_import",
		"grid_export", "battery_charge", "battery_discharge",
	}, figureIDs(r))
}

func TestBuild_CompareMatchedDaysWhenIncomplete(t *testing.T) {
	t.Parallel()
	cur := october()[:10] // 1-10 Oct of 31 days
	base := october()[4:30]
	for i := range base {
		base[i].Date = base[i].Date.AddDate(0, -1, 0) // 5-30 Sep; 1-4 Sep are missing
	}
	r := Build(Input{
		Range: rng("2026-10-01", "2026-10-31"), Days: cur,
		Compare: &CompareInput{
			Mode: ComparePrevious, Range: rng("2026-09-01", "2026-09-30"), Days: base,
		},
	})
	assert.Equal(t, 10, r.Summary.CompleteDays)
	require.True(t, r.Comparison.Matched)
	assert.Equal(t, 6, r.Comparison.MatchedDays, "5-10 Oct against 5-10 Sep")
	assert.Equal(t, 26, r.Comparison.Summary.CompleteDays, "the summary keeps every day")
	var total, matched float64
	for i, d := range october()[:10] {
		total += d.PV
		if i >= 4 {
			matched += d.PV
		}
	}
	pv := figure(t, r, "pv")
	assert.InDelta(t, total, *pv.Value, 1e-6, "value stays the total of the period")
	assert.InDelta(t, matched, *pv.Compared, 1e-6)
	assert.InDelta(t, matched, *pv.Base, 1e-6, "same energy on the same days")
	assert.InDelta(t, 0, *pv.Change, 1e-6)
	assert.InDelta(t, 0, *pv.ChangePct, 1e-6)
	ss := figure(t, r, "self_sufficiency")
	assert.InDelta(t, 0, *ss.Change, 1e-6, "ratios compare the matched days too")
}

func TestBuild_CompareMatchedWithoutCommonDays(t *testing.T) {
	t.Parallel()
	base := october()[20:]
	for i := range base {
		base[i].Date = base[i].Date.AddDate(-1, 0, 0)
	}
	r := Build(Input{
		Range: rng("2026-10-01", "2026-10-31"), Days: october()[:10],
		Compare: &CompareInput{
			Mode: CompareLastYear, Range: rng("2025-10-01", "2025-10-31"), Days: base,
		},
	})
	require.True(t, r.Comparison.Matched)
	assert.Zero(t, r.Comparison.MatchedDays)
	pv := figure(t, r, "pv")
	assert.NotNil(t, pv.Value)
	assert.Nil(t, pv.Compared)
	assert.Nil(t, pv.Base)
	assert.Nil(t, pv.Change)
}

func TestPartner(t *testing.T) {
	t.Parallel()
	feb, lastFeb := rng("2028-02-01", "2028-02-29"), rng("2027-02-01", "2027-02-28")
	p, ok := partner(day("2028-02-28"), CompareLastYear, feb, lastFeb)
	assert.True(t, ok)
	assert.Equal(t, day("2027-02-28"), p)
	_, ok = partner(day("2028-02-29"), CompareLastYear, feb, lastFeb)
	assert.False(t, ok, "29 February has no day a year earlier")

	oct, sep := rng("2026-10-01", "2026-10-31"), rng("2026-09-01", "2026-09-30")
	p, ok = partner(day("2026-10-30"), ComparePrevious, oct, sep)
	assert.True(t, ok)
	assert.Equal(t, day("2026-09-30"), p)
	_, ok = partner(day("2026-10-31"), ComparePrevious, oct, sep)
	assert.False(t, ok, "September has no 31st day")

	week, before := rng("2026-10-08", "2026-10-14"), rng("2026-10-01", "2026-10-07")
	p, ok = partner(day("2026-10-10"), ComparePrevious, week, before)
	assert.True(t, ok)
	assert.Equal(t, day("2026-10-03"), p)
}

func TestOtherLoads(t *testing.T) {
	t.Parallel()
	e := Energy{Consumption: 981.2, Household: 348.0, Backup: 602.4}
	assert.InDelta(t, 30.8, e.OtherLoads(), 1e-9)
}

func TestBuild_NoData(t *testing.T) {
	t.Parallel()
	r := Build(Input{
		Range: rng("2026-10-01", "2026-10-05"),
		Compare: &CompareInput{
			Mode: CompareLastYear, Range: rng("2025-10-01", "2025-10-05"), Days: october(),
		},
	})
	s := r.Summary
	assert.Equal(t, 0, s.CompleteDays)
	assert.Nil(t, s.PerDay)
	assert.Nil(t, s.SelfSufficiency)
	assert.Nil(t, s.SelfConsumption)
	assert.Nil(t, s.PVSplit.SolarShare)
	assert.Nil(t, r.Patterns.BestPV)
	assert.Nil(t, r.Patterns.WeekdayConsumption)
	for _, f := range r.Figures {
		assert.Nil(t, f.Value, f.ID)
		assert.Nil(t, f.Base, f.ID, "October 2026 days are outside the 2025 range")
		assert.Nil(t, f.Change, f.ID)
	}
	assert.Len(t, r.Buckets, 5)
}

func TestBuild_TodayExcluded(t *testing.T) {
	t.Parallel()
	r := rng("2026-10-01", "2026-10-30")
	assert.True(t, Build(Input{Range: r, TodayExcluded: true}).TodayExcluded)
	assert.False(t, Build(Input{Range: r}).TodayExcluded)
}

func TestBuild_CompareMatchedWhenLengthsDiffer(t *testing.T) {
	t.Parallel()
	base := october()[:30]
	for i := range base {
		base[i].Date = base[i].Date.AddDate(0, -1, 0) // a complete September
	}
	r := Build(Input{
		Range: rng("2026-10-01", "2026-10-31"), Days: october(),
		Compare: &CompareInput{
			Mode: ComparePrevious, Range: rng("2026-09-01", "2026-09-30"), Days: base,
		},
	})
	require.True(t, r.Summary.Complete())
	require.True(t, r.Comparison.Summary.Complete())
	assert.True(t, r.Comparison.Matched, "31 days against 30")
	assert.Equal(t, 30, r.Comparison.MatchedDays)
	pv := figure(t, r, "pv")
	assert.InDelta(t, 403.7, *pv.Value, 1e-6)
	assert.InDelta(t, 403.7-11.6, *pv.Compared, 1e-6, "without 31 Oct")
	assert.InDelta(t, 0, *pv.Change, 1e-6, "the extra day is not a change")
}

func TestBuild_NoComparison(t *testing.T) {
	t.Parallel()
	r := Build(Input{Range: rng("2026-10-01", "2026-10-31"), Days: october()})
	assert.Nil(t, r.Comparison)
	require.Len(t, r.Figures, 8)
	for _, f := range r.Figures {
		assert.NotNil(t, f.Value, f.ID)
		assert.Nil(t, f.Base, f.ID)
		assert.Nil(t, f.Change, f.ID)
	}
}

func TestBuild_ZeroBaseHasNoPercent(t *testing.T) {
	t.Parallel()
	cur := october()[:1]
	base := []Day{{Date: day("2025-10-01"), Energy: Energy{Consumption: 5, Import: 5}}}
	r := Build(Input{
		Range: rng("2026-10-01", "2026-10-01"), Days: cur,
		Compare: &CompareInput{
			Mode: CompareLastYear, Range: rng("2025-10-01", "2025-10-01"), Days: base,
		},
	})
	pv := figure(t, r, "pv")
	assert.InDelta(t, 13.5, *pv.Change, delta)
	assert.Nil(t, pv.ChangePct)
	sc := figure(t, r, "self_consumption")
	assert.Nil(t, sc.Base, "no PV, no self-consumption")
	assert.Nil(t, sc.Change)
}

func TestSummarize_ShortPeriodHasNoBalance(t *testing.T) {
	t.Parallel()
	s := summarize(october()[:minBalanceDays-1], minBalanceDays-1)
	assert.Nil(t, s.BatteryRatio)
	assert.Nil(t, s.BalanceDiff)
	assert.Nil(t, s.BalanceDiffShare)
	s = summarize(october()[:minBalanceDays], minBalanceDays)
	assert.NotNil(t, s.BatteryRatio)
}

func TestSummarize_EdgeCases(t *testing.T) {
	t.Parallel()
	d := day("2026-10-01")
	tests := []struct {
		name     string
		e        Energy
		gridChrg bool
		ss       float64
		fromGrid float64
	}{
		{
			"charge within tolerance",
			Energy{PV: 5, Export: 1, Charge: 4.2, Consumption: 2},
			false, 1, 0.2,
		},
		{
			"charge above tolerance",
			Energy{PV: 5, Export: 1, Charge: 4.3, Consumption: 2},
			true, 1, 0.3,
		},
		{"import above consumption", Energy{Consumption: 2, Import: 3, Charge: 1}, true, 0, 1},
		{
			"export above PV (counter glitch)",
			Energy{PV: 1, Export: 1.5, Consumption: 1},
			false, 1, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := summarize([]Day{{Date: d, Energy: tt.e}}, 1)
			assert.Equal(t, tt.gridChrg, len(s.GridChargingDays) == 1)
			assert.InDelta(t, tt.ss, *s.SelfSufficiency, delta)
			assert.GreaterOrEqual(t, s.DirectUse, 0.0)
			assert.InDelta(t, tt.fromGrid, s.GridCharge, 1e-9)
		})
	}
}

func TestBuild_IgnoresDaysOutsideRangeAndSorts(t *testing.T) {
	t.Parallel()
	days := october()
	days[0], days[30] = days[30], days[0] // unsorted input
	r := Build(Input{Range: rng("2026-10-02", "2026-10-30"), Days: days})
	assert.Equal(t, 29, r.Summary.CompleteDays)
	assert.Equal(t, "2026-10-02", r.Buckets[0].Key)
	assert.Equal(t, 1, r.Buckets[0].Summary.CompleteDays)
}

func TestBuckets_MonthPartialEdges(t *testing.T) {
	t.Parallel()
	r := rng("2026-03-15", "2026-06-10")
	days := []Day{{Date: day("2026-03-20")}, {Date: day("2026-04-01")}, {Date: day("2026-06-10")}}
	got := buckets(r, ByMonth, days)
	require.Len(t, got, 4)
	want := []struct {
		key           string
		from, to      string
		partial       bool
		days, present int
	}{
		{"2026-03", "2026-03-15", "2026-03-31", true, 17, 1},
		{"2026-04", "2026-04-01", "2026-04-30", false, 30, 1},
		{"2026-05", "2026-05-01", "2026-05-31", false, 31, 0},
		{"2026-06", "2026-06-01", "2026-06-10", true, 10, 1},
	}
	for i, w := range want {
		b := got[i]
		assert.Equal(t, w.key, b.Key)
		assert.Equal(t, day(w.from), b.From)
		assert.Equal(t, day(w.to), b.To)
		assert.Equal(t, w.partial, b.Partial, w.key)
		assert.Equal(t, w.days, b.Summary.Days, w.key)
		assert.Equal(t, w.present, b.Summary.CompleteDays, w.key)
	}
}

func TestBuckets_Year(t *testing.T) {
	t.Parallel()
	got := buckets(rng("2024-01-01", "2026-10-29"), ByYear, nil)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"2024", "2025", "2026"}, []string{got[0].Key, got[1].Key, got[2].Key})
	assert.Equal(t, 366, got[0].Summary.Days, "leap year")
	assert.False(t, got[1].Partial)
	assert.True(t, got[2].Partial)
}

func TestAssemble(t *testing.T) {
	t.Parallel()
	values := map[string]map[string]float64{}
	for i, k := range DailyKeys() {
		values[k] = map[string]float64{"2026-10-02": float64(i + 1), "2026-10-01": 1}
	}
	delete(values["grid_export_daily"], "2026-10-01") // incomplete day
	values["pv_energy_daily"]["2026-10-03"] = 9       // only PV
	days, err := Assemble(values)
	require.NoError(t, err)
	require.Len(t, days, 1)
	assert.Equal(t, day("2026-10-02"), days[0].Date)
	assert.Equal(t, Energy{
		PV: 1, Consumption: 2, Household: 3, Backup: 4, Import: 5, Export: 6, Charge: 7,
		Discharge: 8,
	}, days[0].Energy)

	values = map[string]map[string]float64{}
	for _, k := range DailyKeys() {
		values[k] = map[string]float64{"bad": 1}
	}
	_, err = Assemble(values)
	require.Error(t, err)
}

func TestDailyKeysAreDailyRegisters(t *testing.T) {
	t.Parallel()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	keys := DailyKeys()
	assert.Len(t, keys, 8)
	for _, k := range keys {
		r, ok := reg.ByKey(k)
		require.True(t, ok, k)
		assert.Equal(t, solis.StoreDaily, r.Store, k)
		assert.False(t, r.Net, k)
	}
}

func figureIDs(r Report) []string {
	out := make([]string, len(r.Figures))
	for i, f := range r.Figures {
		out[i] = f.ID
	}
	return out
}

func figure(t *testing.T, r Report, id string) Figure {
	t.Helper()
	for _, f := range r.Figures {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("figure %s missing", id)
	return Figure{}
}
