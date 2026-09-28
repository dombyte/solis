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

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util/clocktest"
)

var ctx = context.Background()

func testConfig(path string) Settings {
	return Settings{
		Path: path, DailyRetention: 365 * 24 * time.Hour, ErrorRetention: 30 * 24 * time.Hour,
		WalMode: true, Synchronous: "NORMAL", TempStore: "MEMORY",
	}
}

func newStore(t *testing.T, now time.Time) (*Storage, *clocktest.Clock, string) {
	t.Helper()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	clk := clocktest.New(now)
	path := filepath.Join(t.TempDir(), "solis.db")
	s, err := New(ctx, testConfig(path), reg, clk, zerolog.Nop())
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
	s2, err := New(ctx, testConfig(path), reg, clocktest.New(day("2026-10-01")), zerolog.Nop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	st2, err := s2.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, st, st2)
}

// The cutover month and year keep their inverter-reported value (review AGG-M2): the
// difference to the daily sum up to the cutover day is recorded as an offset; keys at or
// below their daily sum get none. A backfill of the period clears the offset.
func TestEnsureCutover_RecordsOffsetsForCutoverMonthAndYear(t *testing.T) {
	s, _, _ := newStore(t, day("2026-09-27"))
	require.NoError(t, s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
		{Key: "pv_energy_daily", Day: "2026-09-01", Value: 100},
		{Key: "pv_energy_daily", Day: "2026-09-27", Value: 20},
		{Key: "grid_export_daily", Day: "2026-09-02", Value: 50},
	}}))
	insertRows(t, s,
		`INSERT INTO monthly_values (month, register_key, value, raw_value) VALUES
			('2026-09', 'pv_energy_monthly', 400, 4000),
			('2026-09', 'grid_export_monthly', 30, 300)`,
		`INSERT INTO yearly_values (year, register_key, value, raw_value) VALUES
			('2026', 'pv_energy_yearly', 5000, 50000)`)
	_, _, err := s.EnsureCutover(ctx, period.Of(day("2026-09-27")))
	require.NoError(t, err)
	st, err := s.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{
		OffsetKey(period.Monthly, "2026-09", "pv_energy_monthly"): 280,  // 400 - 120
		OffsetKey(period.Yearly, "2026", "pv_energy_yearly"):      4880, // 5000 - 120
	}, st.Offsets, "grid_export is below its daily sum: no offset")

	require.NoError(t, s.Backfill(ctx, func(tx BackfillTx) error {
		return tx.PutPeriod(period.Monthly, "pv_energy_monthly", "2026-09", 120)
	}))
	st, err = s.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{
		OffsetKey(period.Yearly, "2026", "pv_energy_yearly"): 4880,
	}, st.Offsets, "backfill override clears the month's offset")
	require.NoError(t, s.loadMeta(ctx))
	st2, err := s.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, st.Offsets, st2.Offsets, "persisted")
}

// A cutover on the 1st still has the previous month's last day open (cutover − 2 days is
// in that month), so that month and, on 1 January, that year must stay open too; their
// inverter values are kept as offsets as well (review AGG-L3).
func TestEnsureCutover_OnFirstOfMonthKeepsPreviousPeriodOpen(t *testing.T) {
	for _, tt := range []struct{ day, frozenMonth, frozenYear string }{
		{"2026-10-01", "2026-08", "2025"},
		{"2027-01-01", "2026-11", "2025"},
		{"2026-09-27", "2026-08", "2025"}, // mid-month: unchanged
	} {
		t.Run(tt.day, func(t *testing.T) {
			s, _, _ := newStore(t, day(tt.day))
			_, _, err := s.EnsureCutover(ctx, period.Of(day(tt.day)))
			require.NoError(t, err)
			st, err := s.CloseState(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.frozenMonth, st.FrozenMonth)
			assert.Equal(t, tt.frozenYear, st.FrozenYear)
		})
	}

	s, _, _ := newStore(t, day("2026-10-01"))
	require.NoError(t, s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
		{Key: "pv_energy_daily", Day: "2026-09-30", Value: 10},
	}}))
	insertRows(t, s, `INSERT INTO monthly_values (month, register_key, value, raw_value)
		VALUES ('2026-09', 'pv_energy_monthly', 300, 3000)`)
	_, _, err := s.EnsureCutover(ctx, period.Of(day("2026-10-01")))
	require.NoError(t, err)
	st, err := s.CloseState(ctx)
	require.NoError(t, err)
	assert.InDelta(t, 290, st.Offsets[OffsetKey(period.Monthly, "2026-09", "pv_energy_monthly")], 0)
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
	w := ComputedWrite{
		Freezes: []Freeze{{Level: period.Yearly, Period: "2026"}},
		Folds:   []BaselineFold{fold},
	}
	require.NoError(t, s.WriteComputed(ctx, w))
	require.NoError(t, s.WriteComputed(ctx, w)) // duplicate close event

	st, err := s.CloseState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2026", st.BaselineYear)
	assert.Equal(t, "2026", st.FrozenYear)
	assert.InDelta(t, 4000.0, st.Baseline["pv_energy_total"], 1e-9)
}

// A rejected status row or day close is skipped like a rejected daily row: the valid
// rows of the same poll still commit (review AGG-M1), and a net key never gets a poller
// close watermark (AGG-L4).
func TestWritePoll_RejectedStatusAndCloseDoNotRollBack(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	now := day("2026-08-05")
	err := s.WritePoll(ctx, PollWrite{
		Daily: []DailyRow{{Key: "pv_energy_daily", Day: "2026-08-05", Value: 7, Raw: 70}},
		Status: []StatusRow{
			{Key: "pv_energy_daily", Raw: 1, At: now}, // not a status register
			{Key: "solis_status", Raw: 3, At: now},
		},
		Close: []DayClose{
			{Key: "zz", Day: "2026-08-04"},
			{Key: "grid_energy_daily", Day: "2026-08-04"}, // net: aggregator-owned
			{Key: "pv_energy_daily", Day: "2026-08-04"},
		},
	})
	require.Error(t, err)
	assert.True(t, IsRejection(err))
	assert.ErrorIs(t, err, ErrWriteDomain)
	assert.ErrorIs(t, err, ErrUnknownKey)

	assert.InDelta(t, 7.0, dailyValue(t, s, "pv_energy_daily", "2026-08-05"), 0)
	seed, err := s.Seed(ctx, nil)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, seed.Status["solis_status"], 0)
	assert.NotContains(t, seed.Status, "pv_energy_daily")
	assert.Equal(t, "2026-08-04", seed.Closed["pv_energy_daily"])
	assert.NotContains(t, seed.Closed, "grid_energy_daily")
	assert.NotContains(t, seed.Closed, "zz")
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

func TestBackfillTx_FirstDailyDayAndCutover(t *testing.T) {
	s, _, _ := newStore(t, day("2026-09-27"))
	require.NoError(t, s.Backfill(ctx, func(tx BackfillTx) error {
		first, err := tx.FirstDailyDay()
		require.NoError(t, err)
		assert.Empty(t, first)
		assert.Empty(t, tx.Cutover())
		return nil
	}))
	require.NoError(t, s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
		{Key: "pv_energy_daily", Day: "2025-06-01", Value: 1},
		{Key: "pv_energy_daily", Day: "2026-01-01", Value: 1},
	}}))
	_, _, err := s.EnsureCutover(ctx, period.Of(day("2026-09-01")))
	require.NoError(t, err)
	require.NoError(t, s.Backfill(ctx, func(tx BackfillTx) error {
		first, err := tx.FirstDailyDay()
		require.NoError(t, err)
		assert.Equal(t, "2025-06-01", first)
		assert.Equal(t, "2026-09-01", tx.Cutover())
		return nil
	}))
	cctx, cancel := context.WithCancel(ctx)
	require.NoError(t, s.Backfill(ctx, func(tx BackfillTx) error {
		cancel()
		b := tx.(*backfillTx)
		b.ctx = cctx
		_, err := tx.FirstDailyDay()
		assert.Error(t, err)
		return nil
	}))
}

// A corrupt baseline row fails loading loudly instead of starting with a zero baseline.
func TestLoadMeta_CorruptBaselineIsAnError(t *testing.T) {
	s, _, _ := newStore(t, day("2026-09-27"))
	insertRows(t, s, `INSERT INTO meta (key, value) VALUES ('baseline:pv_energy_total', 'x')`)
	assert.ErrorContains(t, s.loadMeta(ctx), "baseline:pv_energy_total")
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
}

// insertRows writes raw rows bypassing the write guards (history older than a cutover).
func insertRows(t *testing.T, s *Storage, stmts ...string) {
	t.Helper()
	for _, q := range stmts {
		_, err := s.db.Exec(q)
		require.NoError(t, err)
	}
}

func countRows(t *testing.T, s *Storage, table string) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

func TestCleanupAll_StatusRetention(t *testing.T) {
	s, clk, _ := newStore(t, day("2026-08-05"))
	require.NoError(t, s.WritePoll(ctx, PollWrite{Status: []StatusRow{
		{Key: "solis_status", Raw: 1, At: clk.Now().AddDate(0, -3, 0)},
		{Key: "solis_status", Raw: 2, At: clk.Now().AddDate(0, 0, -1)},
	}}))
	require.NoError(t, s.CleanupAll(ctx))
	hist, err := s.GetErrorHistory(ctx, "solis_status", time.Time{}, clk.Now())
	require.NoError(t, err)
	require.Len(t, hist, 1)
	assert.InDelta(t, 2.0, hist[0].RawValue, 0)
	require.NoError(t, s.CleanupAll(ctx)) // nothing deleted, no vacuum
	require.NoError(t, s.Ping(ctx))
}

func TestCleanupAll_NothingFrozenKeepsPeriods(t *testing.T) {
	s, _, _ := newStore(t, day("2026-08-05"))
	insertRows(t, s, `INSERT INTO daily_values (date, register_key, value, raw_value)
		VALUES ('2020-01-01', 'pv_energy_daily', 1, 1)`)
	require.NoError(t, s.CleanupAll(ctx))
	assert.Equal(t, 1, countRows(t, s, "daily_values"))
}

func TestCleanupAll_FreezeAware(t *testing.T) {
	tests := []struct {
		name      string
		retention time.Duration
		purged    string
		daily     int // rows left of 2024-01-01, 2025-07-15, 2025-09-01, 2026-08-01
		monthly   int // rows left of 2025-07, 2025-08
		yearly    int // rows left of 2024, 2025
	}{
		// wanted cutoff 2025-08-05 is before the first open day (2026-01-01).
		{"retention wins", 365 * 24 * time.Hour, "2025-08-05", 2, 1, 1},
		// wanted 2026-08-04 is clamped to 2026-01-01: open periods are never deleted.
		{"freeze clamps", 24 * time.Hour, "2026-01-01", 1, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := newStore(t, day("2026-08-05"))
			s.cfg.DailyRetention = tt.retention
			_, _, err := s.EnsureCutover(ctx, period.Of(day("2026-08-05")))
			require.NoError(t, err)
			insertRows(t, s,
				`INSERT INTO daily_values (date, register_key, value, raw_value) VALUES
				('2024-01-01', 'pv_energy_daily', 1, 1), ('2025-07-15', 'pv_energy_daily', 1, 1),
				('2025-09-01', 'pv_energy_daily', 1, 1), ('2026-08-01', 'pv_energy_daily', 1, 1)`,
				`INSERT INTO monthly_values (month, register_key, value, raw_value) VALUES
				('2025-07', 'pv_energy_monthly', 1, 1), ('2025-08', 'pv_energy_monthly', 1, 1)`,
				`INSERT INTO yearly_values (year, register_key, value, raw_value) VALUES
				('2024', 'pv_energy_yearly', 1, 1), ('2025', 'pv_energy_yearly', 1, 1)`)
			require.NoError(t, s.CleanupAll(ctx))
			assert.Equal(t, tt.daily, countRows(t, s, "daily_values"))
			assert.Equal(t, tt.monthly, countRows(t, s, "monthly_values"))
			assert.Equal(t, tt.yearly, countRows(t, s, "yearly_values"))
			require.NoError(t, s.Backfill(ctx, func(tx BackfillTx) error {
				assert.Equal(t, tt.purged, tx.PurgedBefore())
				return nil
			}))
			require.NoError(t, s.loadMeta(ctx)) // watermark persisted
			assert.Equal(t, tt.purged, s.meta.purgedBefore)
		})
	}
}

func TestHistoryFormats(t *testing.T) {
	s, clk, _ := newStore(t, day("2026-09-20"))
	require.NoError(t, s.WritePoll(ctx, PollWrite{
		Daily:  []DailyRow{{Key: "pv_energy_daily", Day: "2026-09-20", Value: 1}},
		Status: []StatusRow{{Key: "solis_status", Raw: 3, At: clk.Now()}},
	}))
	d, err := s.GetDailyHistory(ctx, "pv_energy_daily", day("2026-09-01"), day("2026-09-30"))
	require.NoError(t, err)
	require.Len(t, d, 1)
	assert.Equal(t, "2026-09-20", d[0].Date)

	e, err := s.GetErrorHistory(ctx, "solis_status", day("2026-09-01"), day("2026-09-30"))
	require.NoError(t, err)
	require.Len(t, e, 1)
	assert.Equal(t, clk.Now().UTC().Format("2006-01-02T15:04:05.000Z"), e[0].Timestamp)
}

func TestGetErrorHistory_KeepsNewestOverCap(t *testing.T) {
	s, clk, _ := newStore(t, day("2026-09-20"))
	base := clk.Now()
	rows := make([]StatusRow, 0, maxErrorRows+2)
	for i := range maxErrorRows + 2 {
		rows = append(rows, StatusRow{
			Key: "solis_status", Raw: float64(i),
			At: base.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(t, s.WritePoll(ctx, PollWrite{Status: rows}))
	e, err := s.GetErrorHistory(ctx, "solis_status", base.Add(-time.Hour),
		base.Add(time.Duration(maxErrorRows+10)*time.Second))
	require.NoError(t, err)
	require.Len(t, e, maxErrorRows)
	assert.InDelta(t, 2.0, e[0].RawValue, 0, "oldest rows dropped")
	assert.InDelta(t, float64(maxErrorRows+1), e[len(e)-1].RawValue, 0, "ascending order")
}

func TestNew_Errors(t *testing.T) {
	reg, _ := solis.NewRegistry()
	_, err := New(ctx, testConfig(t.TempDir()), reg, clocktest.New(time.Now()), zerolog.Nop())
	assert.Error(t, err, "a directory is not a database")
}
