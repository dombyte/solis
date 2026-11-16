package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/report"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// expectDays answers every series of [from, to] with one row per listed day (value 1), in
// one read.
func expectDays(f fixture, from, to string, days ...string) {
	values := make(map[string]map[string]float64)
	for _, k := range report.DailyKeys() {
		values[k] = make(map[string]float64, len(days))
		for _, d := range days {
			values[k][d] = 1
		}
	}
	f.store.EXPECT().GetDailyValues(mock.Anything, report.DailyKeys(), date(from), date(to)).
		Return(values, nil).Once()
}

func TestReport_WithComparison(t *testing.T) {
	f := newFixture(t)
	expectDays(f, "2026-10-01", "2026-10-30", "2026-10-01", "2026-10-02") // without today
	expectDays(f, "2026-09-01", "2026-09-30", "2026-09-01")
	r, err := f.svc.Report(ctx, ReportQuery{
		From: "2026-10-01", To: "2026-10-31", Compare: "previous", Today: "2026-10-31",
	})
	require.NoError(t, err)
	assert.Equal(t, report.ByDay, r.Granularity)
	assert.Equal(t, 2, r.Summary.CompleteDays)
	assert.InDelta(t, 2.0, r.Summary.Total.PV, 1e-9)
	require.NotNil(t, r.Comparison)
	assert.Equal(t, report.ComparePrevious, r.Comparison.Mode)
	assert.Equal(t, 1, r.Comparison.Summary.CompleteDays)
	assert.True(t, r.Comparison.Matched)
	assert.Equal(t, 1, r.Comparison.MatchedDays, "1 Oct against 1 Sep")
	assert.True(t, r.TodayExcluded)
	assert.Equal(t, "2026-10-01..2026-10-30", r.Range.String())
	assert.Equal(t, "2026-09-01..2026-09-30", r.Comparison.Range.String(),
		"the comparison follows the finished range")
}

func TestReport_NoComparisonAndGranularity(t *testing.T) {
	f := newFixture(t)
	expectDays(f, "2026-01-01", "2026-10-29")
	r, err := f.svc.Report(ctx, ReportQuery{
		From: "2026-01-01", To: "2026-10-29", Compare: "none", Granularity: "day",
		Today: "2026-10-30",
	})
	require.NoError(t, err)
	assert.Nil(t, r.Comparison)
	assert.False(t, r.TodayExcluded, "the range ended yesterday")
	assert.Len(t, r.Buckets, 302)
}

func TestReport_InvalidParameters(t *testing.T) {
	f := newFixture(t) // no store calls expected
	for _, q := range []ReportQuery{
		{From: "2026-10-01", To: "2026-10-30", Today: "2026-10-29"},
		{From: "2026-10-01", To: "2026-10-29", Today: "2026-10-29", Granularity: "week"},
		{From: "2026-10-01", To: "2026-10-29", Today: "2026-10-29", Compare: "x"},
		{From: "2026-10-29", To: "2026-10-29", Today: "2026-10-29"}, // only today
		{From: "2026-10-01", To: "2026-10-29", Today: "29.10.2026"},
	} {
		_, err := f.svc.Report(ctx, q)
		require.ErrorIs(t, err, report.ErrInvalidRange, q)
	}
}

func TestReport_StoreErrors(t *testing.T) {
	boom := errors.New("boom")
	f := newFixture(t)
	f.store.EXPECT().GetDailyValues(mock.Anything, mock.Anything, mock.Anything,
		mock.Anything).Return(nil, boom).Once()
	_, err := f.svc.Report(ctx, ReportQuery{
		From: "2026-10-01", To: "2026-10-29", Compare: "none", Today: "2026-10-30",
	})
	require.ErrorIs(t, err, boom)

	f = newFixture(t)
	expectDays(f, "2026-10-01", "2026-10-29")
	f.store.EXPECT().GetDailyValues(mock.Anything, mock.Anything, mock.Anything,
		mock.Anything).Return(nil, boom).Once()
	_, err = f.svc.Report(ctx, ReportQuery{
		From: "2026-10-01", To: "2026-10-29", Today: "2026-10-30",
	})
	require.ErrorIs(t, err, boom, "comparison read fails")
}
