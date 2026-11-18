package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/solis"
)

func TestParseTimeRange(t *testing.T) {
	tr, err := ParseTimeRange("", "", t0)
	require.NoError(t, err)
	assert.Equal(t, "2026-07-06", tr.Start.Format(time.DateOnly))
	assert.Equal(t, "2026-08-05", tr.End.Format(time.DateOnly))
	_, err = ParseTimeRange("2026-08-01", "bad", t0)
	assert.ErrorIs(t, err, ErrInvalidRange)
	// start after end is rejected instead of silently returning [] (review HTTP-L4).
	_, err = ParseTimeRange("2026-08-05", "2026-08-01", t0)
	assert.ErrorIs(t, err, ErrInvalidRange)
	assert.ErrorContains(t, err, "start 2026-08-05 is after end 2026-08-01")

	// Month/year ends are inclusive of the whole period.
	tests := []struct{ start, end, wantStart, wantEnd string }{
		{"2026", "2026", "2026-01-01", "2026-12-31"},
		{"2026-02", "2026-02", "2026-02-01", "2026-02-28"},
		{"2024-02", "2024-02", "2024-02-01", "2024-02-29"},
		{"2026-08-01", "2026-08-03", "2026-08-01", "2026-08-03"},
	}
	for _, tt := range tests {
		tr, err := ParseTimeRange(tt.start, tt.end, t0)
		require.NoError(t, err)
		assert.Equal(t, tt.wantStart, tr.Start.Format(time.DateOnly), tt.start)
		assert.Equal(t, tt.wantEnd, tr.End.Format(time.DateOnly), tt.end)
	}
}

// Defaults are calendar days in now's location: east of UTC shortly after midnight, or
// with an end exactly defaultHistoryDays ago, an omitted bound must not compare as a
// different instant on the same day (it used to fail with "start is after end").
func TestParseTimeRange_DefaultsAreLocalDays(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	afterMidnight := time.Date(2026, 11, 16, 0, 30, 0, 0, berlin)
	tr, err := ParseTimeRange("2026-11-16", "", afterMidnight)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-16", tr.End.Format(time.DateOnly))

	afternoon := time.Date(2026, 11, 16, 14, 0, 0, 0, berlin)
	tr, err = ParseTimeRange("", "2026-10-17", afternoon)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-17", tr.Start.Format(time.DateOnly))
}

func TestHistoryCapable(t *testing.T) {
	for s, want := range map[solis.Store]bool{
		solis.StoreDaily: true, solis.StoreMonthly: true, solis.StoreYearly: true,
		solis.StoreStatus: true, solis.StoreTotal: false, solis.StoreNone: false,
	} {
		assert.Equal(t, want, HistoryCapable(s), s.String())
	}
}

// Data owns the /api/data rules (review HTTP-L11): what a key returns depends on its
// store, and a range only applies to daily/monthly/yearly keys.
func TestData_DispatchByStore(t *testing.T) {
	f := newFixture(t)
	q := func(key, start, end string) DataQuery {
		return DataQuery{Key: key, Start: start, End: end, Now: t0}
	}

	f.cache.EXPECT().Get("grid_power").Return(&solis.Value{Key: "grid_power"}).Once()
	res, err := f.svc.Data(ctx, q("grid_power", "", ""))
	require.NoError(t, err)
	assert.Equal(t, "grid_power", res.Current.Key)
	assert.Equal(t, "grid_power", res.Register.Key)

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	rows := []*history.DailyDataPoint{{Date: "2026-08-01", Value: 1}}
	f.store.EXPECT().GetDailyHistory(mock.Anything, "pv_energy_daily", start, end).
		Return(rows, nil).Once()
	res, err = f.svc.Data(ctx, q("pv_energy_daily", "2026-08-01", "2026-08-03"))
	require.NoError(t, err)
	assert.Equal(t, rows, res.Rows)

	f.store.EXPECT().GetMonthlyHistory(mock.Anything, "pv_energy_monthly", mock.Anything,
		calendarDay(t0)).
		Return(nil, nil).Once()
	_, err = f.svc.Data(ctx, q("pv_energy_monthly", "2026-01", ""))
	require.NoError(t, err)

	f.store.EXPECT().GetYearlyHistory(mock.Anything, "pv_energy_yearly",
		calendarDay(t0).AddDate(0, 0, -defaultHistoryDays),
		time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)).
		Return(nil, nil).Once()
	_, err = f.svc.Data(ctx, q("pv_energy_yearly", "", "2027"))
	require.NoError(t, err)

	f.store.EXPECT().GetTotalHistory(mock.Anything, "pv_energy_total").
		Return(&history.TotalDataPoint{Value: 5}, nil).Once()
	res, err = f.svc.Data(ctx, q("pv_energy_total", "", ""))
	require.NoError(t, err)
	assert.InDelta(t, 5.0, res.Total.Value, 0)

	f.store.EXPECT().GetErrorHistory(mock.Anything, "grid_fault_1", mock.Anything,
		mock.Anything).Return(nil, nil).Once()
	f.cache.EXPECT().Get("grid_fault_1").Return(nil).Once()
	res, err = f.svc.Data(ctx, q("grid_fault_1", "", ""))
	require.NoError(t, err)
	require.NotNil(t, res.Status)
	assert.Equal(t, "grid_fault_1", res.Status.Key)
}

func TestData_Errors(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Data(ctx, DataQuery{Key: "nope", Now: t0})
	assert.ErrorIs(t, err, ErrUnknownKey)

	for _, key := range []string{"grid_power", "pv_energy_total"} {
		_, err = f.svc.Data(ctx, DataQuery{Key: key, Start: "2026-08-01", Now: t0})
		assert.ErrorIs(t, err, ErrWrongKind, key)
		assert.ErrorContains(t, err, "only supported for daily, monthly, yearly and status", key)
	}

	for _, key := range []string{"pv_energy_daily", "solis_status"} {
		_, err = f.svc.Data(ctx, DataQuery{Key: key, Start: "yesterday", Now: t0})
		assert.ErrorIs(t, err, ErrInvalidRange, key)
	}
}

// A status range covers whole local days: from local midnight of the start day to the
// last stored millisecond of the end day, including a 25-hour DST day (Europe/Berlin).
func TestData_StatusRangeIsLocalDays(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, berlin)
	f := newFixture(t)

	start := time.Date(2026, 10, 25, 0, 0, 0, 0, berlin) // DST ends this day
	end := time.Date(2026, 10, 26, 0, 0, 0, 0, berlin).Add(-time.Millisecond)
	f.store.EXPECT().GetErrorHistory(mock.Anything, "solis_status", start, end).
		Return([]*history.ErrorDataPoint{{Timestamp: "2026-10-25T10:00:00.000Z", RawValue: 3}},
			nil).Once()
	f.cache.EXPECT().Get("solis_status").Return(&solis.Value{
		Timestamp: now, RawValue: 3, StatusDecoded: "Generating",
	}).Once()

	f.store.EXPECT().GetLastErrorBefore(mock.Anything, "solis_status", start).
		Return(&history.ErrorDataPoint{Timestamp: "2026-10-20T08:00:00.000Z", RawValue: 0}, nil).
		Once()

	res, err := f.svc.Data(ctx, DataQuery{
		Key: "solis_status", Start: "2026-10-25", End: "2026-10-25", Now: now,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Status)
	assert.Len(t, res.Status.History, 1, "the current value is after the range")
	assert.Equal(t, 25*time.Hour, end.Sub(start)+time.Millisecond)
	require.NotNil(t, res.Status.Before, "the state at the range start")
	assert.Equal(t, "2026-10-20T08:00:00.000Z", res.Status.Before.Timestamp)
	assert.NotNil(t, res.Status.Before.StatusDecoded)

	// Only an end: start falls back to the default 30 days, like the other histories. A
	// failed read of the state before is best effort, like the history.
	defaultStart := time.Date(2026, 10, 3, 0, 0, 0, 0, berlin)
	f.store.EXPECT().GetErrorHistory(mock.Anything, "solis_status", defaultStart,
		mock.Anything).Return(nil, nil).Once()
	f.store.EXPECT().GetLastErrorBefore(mock.Anything, "solis_status", defaultStart).
		Return(nil, errors.New("disk")).Once()
	f.cache.EXPECT().Get("solis_status").Return(nil).Once()
	res, err = f.svc.Data(ctx, DataQuery{Key: "solis_status", End: "2026-11-02", Now: now})
	require.NoError(t, err)
	assert.Nil(t, res.Status.Before)

	_, err = f.svc.Data(ctx, DataQuery{
		Key: "solis_status", Start: "2026-11-03", End: "2026-11-01", Now: now,
	})
	assert.ErrorIs(t, err, ErrInvalidRange)
}
