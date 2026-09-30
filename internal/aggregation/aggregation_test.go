package aggregation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
)

func registry(t *testing.T) *solis.Registry {
	t.Helper()
	r, err := solis.NewRegistry()
	require.NoError(t, err)
	return r
}

func TestApplyEdges(t *testing.T) {
	r := registry(t)
	sums := Values{"pv_energy_daily": 412.3, "grid_export_daily": 10}
	got := ApplyEdges(r.Edges(period.Monthly), sums)
	assert.InDelta(t, 412.3, got["pv_energy_monthly"], 1e-9)
	assert.InDelta(t, 10.0, got["grid_export_monthly"], 1e-9)
	// Gaps / no rows: zero, not absent.
	v, ok := got["backup_energy_monthly"]
	assert.True(t, ok)
	assert.Zero(t, v)
	assert.Len(t, got, 8)
	assert.Empty(t, ApplyEdges(nil, sums))
}

func TestApplyNet(t *testing.T) {
	t.Parallel()
	r := registry(t)
	tests := []struct {
		name   string
		values Values
		want   Values
	}{
		{
			"positive",
			Values{"grid_export_monthly": 12.5, "grid_import_monthly": 2.5},
			Values{"grid_energy_monthly": 10},
		},
		{
			"negative net",
			Values{"grid_export_monthly": 1, "grid_import_monthly": 4.1},
			Values{"grid_energy_monthly": -3.1},
		},
		{"missing import", Values{"grid_export_monthly": 1}, Values{}},
		{"empty", Values{}, Values{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ApplyNet(r.NetPairs(period.Monthly), tt.values)
			require.Len(t, got, len(tt.want))
			for k, v := range tt.want {
				assert.InDelta(t, v, got[k], 1e-9)
			}
		})
	}
}

func TestTotalsAndFold(t *testing.T) {
	r := registry(t)
	edges := r.Edges(period.Total)
	baseline := Values{"pv_energy_total": 5000}
	since := Values{"pv_energy_daily": 230, "grid_import_daily": 5}

	got := Totals(edges, baseline, since)
	assert.InDelta(t, 5230.0, got["pv_energy_total"], 1e-9)
	assert.InDelta(t, 5.0, got["grid_import_total"], 1e-9)
	assert.Zero(t, got["backup_energy_total"])

	folded := Fold(baseline, Values{"pv_energy_total": 230, "grid_import_total": 5})
	assert.InDelta(t, 5230.0, folded["pv_energy_total"], 1e-9)
	assert.InDelta(t, 5.0, folded["grid_import_total"], 1e-9)
	assert.InDelta(t, 5000.0, baseline["pv_energy_total"], 1e-9, "input untouched")

	// Composition: a year folded into the baseline gives the same total as summing it.
	after := Totals(edges, folded, Values{})
	assert.InDelta(t, got["pv_energy_total"], after["pv_energy_total"], 1e-9)
}

func TestSourceKeysAndMerge(t *testing.T) {
	r := registry(t)
	keys := SourceKeys(append(r.Edges(period.Monthly), r.Edges(period.Yearly)...))
	assert.Len(t, keys, 8)
	assert.Equal(t, "backup_energy_daily", keys[0])

	m := Merge(Values{"a": 1, "b": 2}, Values{"b": 3})
	assert.Equal(t, Values{"a": 1, "b": 3}, m)
}

func at(day string) period.Period {
	t, _ := time.Parse(period.DayLayout, day)
	return period.Of(t.Add(12 * time.Hour))
}

func TestBuildPlan_OpenPeriodsOnly(t *testing.T) {
	p, err := BuildPlan(at("2026-08-05"), Watermarks{
		FrozenMonth: "2026-07", FrozenYear: "2025", FrozenNetDay: "2026-08-04",
		ClosedThrough: "2026-08-04",
	})
	require.NoError(t, err)
	assert.Equal(t, []Job{{
		Level: period.Daily, Key: "2026-08-05", From: "2026-08-05",
		To: "2026-08-05",
	}}, p.Days)
	assert.Equal(t, []Job{{
		Level: period.Monthly, Key: "2026-08", From: "2026-08-01",
		To: "2026-08-05",
	}}, p.Months)
	assert.Equal(t, []Job{{
		Level: period.Yearly, Key: "2026", From: "2026-01-01",
		To: "2026-08-05",
	}}, p.Years)
}

func TestBuildPlan_MonthAndYearClose(t *testing.T) {
	// 00:30 on Jan 1st: every key closed Dec 31st -> December and 2026 freeze.
	p, err := BuildPlan(at("2027-01-01"), Watermarks{
		FrozenMonth: "2026-11", FrozenYear: "2025", FrozenNetDay: "2026-12-30",
		ClosedThrough: "2026-12-31",
	})
	require.NoError(t, err)
	require.Len(t, p.Months, 2)
	assert.Equal(t, Job{
		Level: period.Monthly, Key: "2026-12", From: "2026-12-01",
		To: "2026-12-31", Freeze: true,
	}, p.Months[0])
	assert.False(t, p.Months[1].Freeze)
	require.Len(t, p.Years, 2)
	assert.True(t, p.Years[0].Freeze)
	assert.Equal(t, "2026", p.Years[0].Key)
	assert.False(t, p.Years[1].Freeze)
	require.Len(t, p.Days, 2)
	assert.True(t, p.Days[0].Freeze)
	assert.False(t, p.Days[1].Freeze)
}

func TestBuildPlan_NotAllKeysClosedYet(t *testing.T) {
	// One key still writes to Dec 31st: nothing may freeze yet (catch-up later).
	p, err := BuildPlan(at("2027-01-01"), Watermarks{
		FrozenMonth: "2026-11", FrozenYear: "2025", FrozenNetDay: "2026-12-30",
		ClosedThrough: "2026-12-30",
	})
	require.NoError(t, err)
	assert.False(t, p.Months[0].Freeze)
	assert.False(t, p.Years[0].Freeze)
}

func TestBuildPlan_EmptyWatermarks(t *testing.T) {
	p, err := BuildPlan(at("2026-08-05"), Watermarks{})
	require.NoError(t, err)
	assert.Len(t, p.Months, 1)
	assert.Len(t, p.Years, 1)
	assert.Len(t, p.Days, MaxNetDays)
	for _, j := range append(append(p.Days, p.Months...), p.Years...) {
		assert.False(t, j.Freeze)
	}
}

func TestBuildPlan_InvalidWatermark(t *testing.T) {
	_, err := BuildPlan(at("2026-08-05"), Watermarks{FrozenMonth: "bad"})
	assert.ErrorIs(t, err, period.ErrInvalidKey)
	_, err = BuildPlan(at("2026-08-05"), Watermarks{FrozenYear: "bad", FrozenMonth: "2026-07"})
	assert.ErrorIs(t, err, period.ErrInvalidKey)
	_, err = periodJobs(at("2026-08-05"), Watermarks{}, period.Total, "")
	assert.Error(t, err)
}

// Net days between the frozen watermark and the MaxNetDays floor are reported, so the
// aggregator can log the gap instead of skipping it silently (review AGG-L1).
func TestBuildPlan_ReportsSkippedNetDays(t *testing.T) {
	plan, err := BuildPlan(at("2026-09-27"), Watermarks{FrozenNetDay: "2026-06-01"})
	require.NoError(t, err)
	assert.Equal(t, [2]string{"2026-06-02", "2026-07-27"}, plan.SkippedNetDays)
	assert.Equal(t, "2026-07-28", plan.Days[0].Key)

	plan, err = BuildPlan(at("2026-09-27"), Watermarks{FrozenNetDay: "2026-09-20"})
	require.NoError(t, err)
	assert.Empty(t, plan.SkippedNetDays[0])
}
