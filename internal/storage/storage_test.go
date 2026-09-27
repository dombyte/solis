package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/utils/clocktest"
)

var ctx = context.Background()

func testConfig(path string) *config.StorageSettings {
	return &config.StorageSettings{
		Path: path, DailyRetention: 365 * 24 * time.Hour, MonthlyRetention: 365 * 24 * time.Hour,
		YearlyRetention: 365 * 24 * time.Hour, ErrorRetention: 30 * 24 * time.Hour,
		WalMode: true, Synchronous: "NORMAL", TempStore: "MEMORY",
	}
}

func newStore(t *testing.T, now time.Time) (*Storage, *clocktest.Clock, string) {
	t.Helper()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	clk := clocktest.New(now)
	path := filepath.Join(t.TempDir(), "solis.db")
	s, err := New(testConfig(path), reg, clk, zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s, clk, path
}

func day(d string) time.Time {
	t, _ := time.ParseInLocation(period.DayLayout, d, time.Local)
	return t.Add(12 * time.Hour)
}

func dailyValue(t *testing.T, s *Storage, key, d string) float64 {
	t.Helper()
	pts, err := s.GetDailyHistory(ctx, key, day(d), day(d))
	require.NoError(t, err)
	require.Len(t, pts, 1)
	return pts[0].Value
}

func TestWritePoll_MaxWriteAndStatus(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	now := day("2026-08-05")
	require.NoError(t, s.WritePoll(ctx, PollWrite{
		Daily:  []DailyRow{{Key: "pv_energy_daily", Day: "2026-08-05", Value: 10.5, Raw: 105}},
		Status: []StatusRow{{Key: "solis_status", Raw: 3, At: now}},
	}))
	// Lower value never replaces the max of the open day.
	require.NoError(t, s.WritePoll(ctx, PollWrite{
		Daily: []DailyRow{{Key: "pv_energy_daily", Day: "2026-08-05", Value: 9, Raw: 90}},
	}))
	assert.InDelta(t, 10.5, dailyValue(t, s, "pv_energy_daily", "2026-08-05"), 1e-9)
	require.NoError(t, s.WritePoll(ctx, PollWrite{
		Daily: []DailyRow{{Key: "pv_energy_daily", Day: "2026-08-05", Value: 11, Raw: 110}},
	}))
	assert.InDelta(t, 11.0, dailyValue(t, s, "pv_energy_daily", "2026-08-05"), 1e-9)

	hist, err := s.GetErrorHistory(ctx, "solis_status", now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, hist, 1)
	assert.InDelta(t, 3.0, hist[0].RawValue, 0)
}

func TestWritePoll_ClosedDayRejected(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	require.NoError(t, s.WritePoll(ctx, PollWrite{
		Daily: []DailyRow{{Key: "pv_energy_daily", Day: "2026-08-05", Value: 20, Raw: 200}},
		Close: []DayClose{{Key: "pv_energy_daily", Day: "2026-08-05"}},
	}))

	err := s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
		{Key: "pv_energy_daily", Day: "2026-08-05", Value: 25, Raw: 250}, // late poll
		{Key: "grid_export_daily", Day: "2026-08-05", Value: 1, Raw: 10}, // other key open
		{Key: "pv_energy_daily", Day: "2026-08-06", Value: 0.1, Raw: 1},
	}})
	require.ErrorIs(t, err, ErrPeriodClosed)
	var pce *PeriodClosedError
	require.True(t, errors.As(err, &pce))
	assert.Equal(t, "2026-08-05", pce.Period)
	assert.Contains(t, pce.Error(), "closed")

	assert.InDelta(t, 20.0, dailyValue(t, s, "pv_energy_daily", "2026-08-05"), 1e-9)
	assert.InDelta(t, 1.0, dailyValue(t, s, "grid_export_daily", "2026-08-05"), 1e-9)
	assert.InDelta(t, 0.1, dailyValue(t, s, "pv_energy_daily", "2026-08-06"), 1e-9)
}

func TestWritePoll_DomainGuards(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	err := s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
		{Key: "grid_energy_daily", Day: "2026-08-05", Value: 1}, // net: aggregator only
		{Key: "pv_energy_monthly", Day: "2026-08-05", Value: 1}, // computed
		{Key: "nope", Day: "2026-08-05", Value: 1},
	}})
	assert.ErrorIs(t, err, ErrWriteDomain)
	assert.ErrorIs(t, err, ErrUnknownKey)
	var wde *WriteDomainError
	require.True(t, errors.As(err, &wde))
	assert.Contains(t, wde.Error(), "poller")

	assert.ErrorIs(t, s.WritePoll(ctx, PollWrite{Status: []StatusRow{{Key: "pv_energy_daily"}}}),
		ErrWriteDomain)
	assert.ErrorIs(t, s.WritePoll(ctx, PollWrite{Close: []DayClose{{Key: "zz"}}}), ErrUnknownKey)
}

func TestWriteComputed_DomainAndFreeze(t *testing.T) {
	s, clk, _ := newStore(t, day("2026-08-05"))
	at := clk.Now()
	rows := []PeriodRow{
		{Level: period.Monthly, Key: "pv_energy_monthly", Period: "2026-08", Value: 100.123},
		{Level: period.Yearly, Key: "pv_energy_yearly", Period: "2026", Value: 900},
		{Level: period.Total, Key: "pv_energy_total", Value: 5000},
		{Level: period.Daily, Key: "grid_energy_daily", Period: "2026-08-05", Value: -2.5},
	}
	require.NoError(t, s.WriteComputed(ctx, ComputedWrite{Rows: rows, At: at}))

	// REPLACE semantics: a lower recomputation replaces the open period.
	require.NoError(t, s.WriteComputed(ctx, ComputedWrite{Rows: []PeriodRow{
		{Level: period.Monthly, Key: "pv_energy_monthly", Period: "2026-08", Value: 99},
	}, Freezes: []Freeze{{Level: period.Monthly, Period: "2026-08"}}, At: at}))
	m, err := s.GetMonthlyHistory(ctx, "pv_energy_monthly", day("2026-08-01"), day("2026-08-01"))
	require.NoError(t, err)
	assert.InDelta(t, 99.0, m[0].Value, 1e-9)

	err = s.WriteComputed(ctx, ComputedWrite{Rows: []PeriodRow{
		{Level: period.Monthly, Key: "pv_energy_monthly", Period: "2026-08", Value: 1},
		{Level: period.Monthly, Key: "pv_energy_daily", Period: "2026-08", Value: 1},
		{Level: period.Daily, Key: "pv_energy_daily", Period: "2026-08-05", Value: 1},
	}, At: at})
	assert.ErrorIs(t, err, ErrPeriodClosed)
	assert.ErrorIs(t, err, ErrWriteDomain)

	total, err := s.GetTotalHistory(ctx, "pv_energy_total")
	require.NoError(t, err)
	assert.InDelta(t, 5000.0, total.Value, 1e-9)
	assert.Equal(t, at.Format(time.RFC3339), total.Timestamp)
	assert.InDelta(t, -2.5, dailyValue(t, s, "grid_energy_daily", "2026-08-05"), 1e-9)

	missing, err := s.GetTotalHistory(ctx, "grid_energy_total")
	require.NoError(t, err)
	assert.Nil(t, missing)

	assert.Error(t, s.WriteComputed(ctx, ComputedWrite{Freezes: []Freeze{{Level: period.Total}}}))
}

func TestEnsureCutover_FreezesPreCutoverHistory(t *testing.T) {
	s, _, path := newStore(t, day("2026-09-27"))
	p := period.Of(day("2026-09-27"))
	cut, created, err := s.EnsureCutover(ctx, p)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "2026-09-27", cut)

	st, err := s.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2026-08", st.FrozenMonth)
	assert.Equal(t, "2025", st.FrozenYear)
	assert.Equal(t, "2025", st.BaselineYear)
	assert.Equal(t, "2026-09-25", st.FrozenNetDay)
	assert.Equal(t, "2026-09-25", st.ClosedThrough)

	// Pre-cutover rows are immutable for the aggregator.
	err = s.WriteComputed(ctx, ComputedWrite{Rows: []PeriodRow{
		{Level: period.Monthly, Key: "pv_energy_monthly", Period: "2026-08", Value: 1},
		{Level: period.Yearly, Key: "pv_energy_yearly", Period: "2025", Value: 1},
	}})
	assert.ErrorIs(t, err, ErrPeriodClosed)

	// Second call is a no-op; state survives reopening.
	_, created, err = s.EnsureCutover(ctx, period.Of(day("2026-10-01")))
	require.NoError(t, err)
	assert.False(t, created)
	require.NoError(t, s.Close())

	reg, _ := solis.NewRegistry()
	s2, err := New(testConfig(path), reg, clocktest.New(day("2026-10-01")), zerolog.Nop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	st2, err := s2.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, st, st2)
}

func TestSumDaily_ExplicitBounds(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	var rows []DailyRow
	for _, d := range []string{"2026-07-31", "2026-08-01", "2026-08-04", "2026-08-05", "2026-08-09"} {
		rows = append(rows, DailyRow{Key: "pv_energy_daily", Day: d, Value: 10, Raw: 100})
	}
	require.NoError(t, s.WritePoll(ctx, PollWrite{Daily: rows}))

	sum, err := s.SumDaily(ctx, "pv_energy_daily", "2026-08-01", "2026-08-05")
	require.NoError(t, err)
	assert.InDelta(t, 30.0, sum, 1e-9, "future-dated 2026-08-09 excluded by the bound")

	all, err := s.SumDaily(ctx, "pv_energy_daily", "", "2026-12-31")
	require.NoError(t, err)
	assert.InDelta(t, 50.0, all, 1e-9)

	none, err := s.SumDaily(ctx, "grid_import_daily", "2026-08-01", "2026-08-31")
	require.NoError(t, err)
	assert.Zero(t, none)

	has, err := s.HasDailyData(ctx)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestBaselineFold_Idempotent(t *testing.T) {
	s, _, _ := newStore(t, day("2027-01-01"))
	fold := BaselineFold{Year: "2026", Add: map[string]float64{"pv_energy_total": 4000}}
	w := ComputedWrite{Freezes: []Freeze{{Level: period.Yearly, Period: "2026"}},
		Folds: []BaselineFold{fold}}
	require.NoError(t, s.WriteComputed(ctx, w))
	require.NoError(t, s.WriteComputed(ctx, w)) // duplicate close event

	st, err := s.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2026", st.BaselineYear)
	assert.Equal(t, "2026", st.FrozenYear)
	assert.InDelta(t, 4000.0, st.Baseline["pv_energy_total"], 1e-9)
}

func TestSeed(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	now := day("2026-08-05")
	require.NoError(t, s.WritePoll(ctx, PollWrite{
		Daily: []DailyRow{
			{Key: "pv_energy_daily", Day: "2026-08-04", Value: 30, Raw: 300},
			{Key: "pv_energy_daily", Day: "2026-08-05", Value: 2, Raw: 20},
			{Key: "grid_export_daily", Day: "2026-08-01", Value: 9, Raw: 90},
		},
		Status: []StatusRow{
			{Key: "solis_status", Raw: 3, At: now},
			{Key: "grid_fault_1", Raw: 1, At: now},
		},
		Close: []DayClose{{Key: "pv_energy_daily", Day: "2026-08-04"}},
	}))
	require.NoError(t, s.WritePoll(ctx, PollWrite{
		Status: []StatusRow{{Key: "solis_status", Raw: 15, At: now.Add(time.Second)}},
	}))

	seed, err := s.Seed(ctx, []string{"2026-08-04", "2026-08-05"})
	require.NoError(t, err)
	assert.InDelta(t, 30.0, seed.Daily["2026-08-04"]["pv_energy_daily"], 0)
	assert.InDelta(t, 2.0, seed.Daily["2026-08-05"]["pv_energy_daily"], 0)
	assert.NotContains(t, seed.Daily["2026-08-05"], "grid_export_daily")
	assert.InDelta(t, 15.0, seed.Status["solis_status"], 0)
	assert.InDelta(t, 1.0, seed.Status["grid_fault_1"], 0)
	assert.Equal(t, "2026-08-04", seed.Closed["pv_energy_daily"])

	empty, err := s.Seed(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty.Daily)
}

func TestClosedThroughNeedsEveryKey(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	reg, _ := solis.NewRegistry()
	var closes []DayClose
	for _, k := range reg.DailyKeys() {
		closes = append(closes, DayClose{Key: k, Day: "2026-08-05"})
	}
	require.NoError(t, s.WritePoll(ctx, PollWrite{Close: closes[1:]}))
	st, _ := s.CloseState(ctx)
	assert.Empty(t, st.ClosedThrough)

	closes[0].Day = "2026-08-04"
	require.NoError(t, s.WritePoll(ctx, PollWrite{Close: closes[:1]}))
	st, _ = s.CloseState(ctx)
	assert.Equal(t, "2026-08-04", st.ClosedThrough)
}

func TestBackfill(t *testing.T) {
	s, _, _ := newStore(t, day("2026-09-27"))
	_, _, err := s.EnsureCutover(ctx, period.Of(day("2026-09-27")))
	require.NoError(t, err)
	require.NoError(t, s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
		{Key: "pv_energy_daily", Day: "2026-09-26", Value: 5, Raw: 50},
	}}))

	err = s.Backfill(ctx, func(tx BackfillTx) error {
		sum, err := tx.SumDaily("pv_energy_daily", "2026-09-01", "2026-09-30")
		require.NoError(t, err)
		assert.InDelta(t, 5.0, sum, 0)
		_, ok, err := tx.PeriodValue(period.Monthly, "pv_energy_monthly", "2026-08")
		require.NoError(t, err)
		assert.False(t, ok)
		// Frozen month is writable inside the backfill transaction.
		require.NoError(t, tx.PutPeriod(period.Monthly, "pv_energy_monthly", "2026-08", 42))
		v, ok, err := tx.PeriodValue(period.Monthly, "pv_energy_monthly", "2026-08")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.InDelta(t, 42.0, v, 0)
		year, base := tx.Baseline()
		assert.Equal(t, "2025", year)
		assert.Empty(t, base)
		require.NoError(t, tx.PutBaseline(map[string]float64{"pv_energy_total": 7}))
		assert.Error(t, tx.PutPeriod(period.Total, "pv_energy_total", "", 1))
		assert.ErrorIs(t, tx.PutPeriod(period.Monthly, "pv_energy_daily", "2026-08", 1),
			ErrWriteDomain)
		_, _, err = tx.PeriodValue(period.Total, "x", "")
		assert.Error(t, err)
		return nil
	})
	require.NoError(t, err)
	st, _ := s.CloseState(ctx)
	assert.InDelta(t, 7.0, st.Baseline["pv_energy_total"], 0)

	// A failing job rolls everything back.
	boom := errors.New("boom")
	err = s.Backfill(ctx, func(tx BackfillTx) error {
		require.NoError(t, tx.PutPeriod(period.Monthly, "pv_energy_monthly", "2026-08", 1))
		return boom
	})
	assert.ErrorIs(t, err, boom)
	m, _ := s.GetMonthlyHistory(ctx, "pv_energy_monthly", day("2026-08-01"), day("2026-08-01"))
	assert.InDelta(t, 42.0, m[0].Value, 0)
}

func TestHistoryAndJSONRounding(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	require.NoError(t, s.WriteComputed(ctx, ComputedWrite{Rows: []PeriodRow{
		{Level: period.Yearly, Key: "pv_energy_yearly", Period: "2026", Value: 1.23456},
	}}))
	y, err := s.GetYearlyHistory(ctx, "pv_energy_yearly", day("2025-01-01"), day("2026-01-01"))
	require.NoError(t, err)
	require.Len(t, y, 1)
	assert.InDelta(t, 1.23456, y[0].Value, 1e-12, "stored at full precision")

	b, err := json.Marshal(y[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"year":"2026","value":1.23,"raw_value":1.23}`, string(b))

	b, _ = json.Marshal(DailyDataPoint{Date: "2026-08-05", Value: 1.005, RawValue: 10})
	assert.JSONEq(t, `{"date":"2026-08-05","value":1.0,"raw_value":10}`, string(b))
	b, _ = json.Marshal(MonthlyDataPoint{Month: "2026-08", Value: 2.499})
	assert.JSONEq(t, `{"month":"2026-08","value":2.5,"raw_value":0}`, string(b))
	b, _ = json.Marshal(TotalDataPoint{Value: 3.333, Timestamp: "t"})
	assert.JSONEq(t, `{"value":3.33,"raw_value":0,"timestamp":"t"}`, string(b))
	b, _ = json.Marshal(ErrorDataPoint{Timestamp: "t", RawValue: 4})
	assert.JSONEq(t, `{"timestamp":"t","raw_value":4}`, string(b))
}

func TestCleanupAll(t *testing.T) {
	s, clk, _ := newStore(t, day("2026-08-05"))
	require.NoError(t, s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
		{Key: "pv_energy_daily", Day: "2024-01-01", Value: 1},
		{Key: "pv_energy_daily", Day: "2026-08-01", Value: 1},
	}, Status: []StatusRow{{Key: "solis_status", Raw: 1, At: clk.Now().AddDate(0, -3, 0)}}}))
	require.NoError(t, s.CleanupAll(ctx))
	sum, err := s.SumDaily(ctx, "pv_energy_daily", "", "2030-01-01")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, sum, 0)
	hist, err := s.GetErrorHistory(ctx, "solis_status", time.Time{}, clk.Now())
	require.NoError(t, err)
	assert.Empty(t, hist)
	require.NoError(t, s.CleanupAll(ctx)) // nothing deleted, no vacuum
	require.NoError(t, s.Ping(ctx))
}

func TestNew_Errors(t *testing.T) {
	reg, _ := solis.NewRegistry()
	_, err := New(testConfig(t.TempDir()), reg, clocktest.New(time.Now()), zerolog.Nop())
	assert.Error(t, err, "a directory is not a database")
}
