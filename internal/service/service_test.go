package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/service/mocks"
	"github.com/dombyte/solis/internal/solis"
	storagemocks "github.com/dombyte/solis/internal/storage/mocks"
)

var (
	ctx = context.Background()
	t0  = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
)

type fixture struct {
	svc   *ReadService
	store *storagemocks.MockReadStore
	cache *mocks.MockCacheReader
	hlth  *mocks.MockHealthSnapshotter
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	f := fixture{
		store: storagemocks.NewMockReadStore(t),
		cache: mocks.NewMockCacheReader(t),
		hlth:  mocks.NewMockHealthSnapshotter(t),
	}
	f.svc, err = NewReadService(Deps{
		Store: f.store, Cache: f.cache, Health: f.hlth, Registry: reg,
		Decoder: solis.NewDecoder(reg, zerolog.Nop()), Log: zerolog.Nop(),
	})
	require.NoError(t, err)
	return f
}

func TestNewReadService_RequiresDependencies(t *testing.T) {
	_, err := NewReadService(Deps{})
	require.ErrorIs(t, err, ErrMissingDependency)
	assert.ErrorContains(t, err, "Store")
}

func TestKeysSortedAndRegister(t *testing.T) {
	f := newFixture(t)
	keys := f.svc.Keys()
	require.NotEmpty(t, keys)
	for i := 1; i < len(keys); i++ {
		assert.Less(t, keys[i-1].Key, keys[i].Key)
	}
	_, err := f.svc.Register("nope")
	assert.ErrorIs(t, err, ErrUnknownKey)
	var ke *KeyError
	require.ErrorAs(t, err, &ke)
	assert.Equal(t, "unknown register key: nope", ke.Error())
}

func TestCurrent(t *testing.T) {
	f := newFixture(t)
	v := &solis.Value{Key: "grid_power", DecodedValue: 12}
	f.cache.EXPECT().Get("grid_power").Return(v).Once()
	got, err := f.svc.Current("grid_power")
	require.NoError(t, err)
	assert.Same(t, v, got)

	f.cache.EXPECT().Get("battery_soc").Return(nil).Once()
	_, err = f.svc.Current("battery_soc")
	assert.ErrorIs(t, err, ErrNoData)
	assert.Contains(t, err.Error(), "no current value")

	_, err = f.svc.Current("zzz")
	assert.ErrorIs(t, err, ErrUnknownKey)
}

func TestHistoryKindChecks(t *testing.T) {
	f := newFixture(t)
	day := []*history.DailyDataPoint{{Date: "2026-08-05", Value: 1}}
	f.store.EXPECT().GetDailyHistory(ctx, "pv_energy_daily", t0, t0).Return(day, nil).Once()
	got, err := f.svc.DailyHistory(ctx, "pv_energy_daily", t0, t0)
	require.NoError(t, err)
	assert.Equal(t, day, got)

	f.store.EXPECT().GetMonthlyHistory(ctx, "pv_energy_monthly", t0, t0).Return(nil, nil).Once()
	_, err = f.svc.MonthlyHistory(ctx, "pv_energy_monthly", t0, t0)
	require.NoError(t, err)
	f.store.EXPECT().GetYearlyHistory(ctx, "pv_energy_yearly", t0, t0).Return(nil, nil).Once()
	_, err = f.svc.YearlyHistory(ctx, "pv_energy_yearly", t0, t0)
	require.NoError(t, err)

	_, err = f.svc.DailyHistory(ctx, "pv_energy_monthly", t0, t0)
	assert.ErrorIs(t, err, ErrWrongKind)
	_, err = f.svc.MonthlyHistory(ctx, "grid_power", t0, t0)
	assert.ErrorIs(t, err, ErrWrongKind)
	_, err = f.svc.YearlyHistory(ctx, "nope", t0, t0)
	assert.ErrorIs(t, err, ErrUnknownKey)
}

func TestTotal(t *testing.T) {
	f := newFixture(t)
	dp := &history.TotalDataPoint{Value: 5}
	f.store.EXPECT().GetTotalHistory(ctx, "pv_energy_total").Return(dp, nil).Once()
	got, err := f.svc.Total(ctx, "pv_energy_total")
	require.NoError(t, err)
	assert.Same(t, dp, got)

	f.store.EXPECT().GetTotalHistory(ctx, "grid_energy_total").Return(nil, nil).Once()
	_, err = f.svc.Total(ctx, "grid_energy_total")
	assert.ErrorIs(t, err, ErrNoData)

	boom := errors.New("disk")
	f.store.EXPECT().GetTotalHistory(ctx, "backup_energy_total").Return(nil, boom).Once()
	_, err = f.svc.Total(ctx, "backup_energy_total")
	assert.ErrorIs(t, err, boom)

	_, err = f.svc.Total(ctx, "pv_energy_daily")
	assert.ErrorIs(t, err, ErrWrongKind)
}

func TestStatusHistory(t *testing.T) {
	f := newFixture(t)
	f.store.EXPECT().GetErrorHistory(ctx, "grid_fault_1", mock.Anything, mock.Anything).
		Return([]*history.ErrorDataPoint{
			{Timestamp: "2026-08-01T10:00:00Z", RawValue: 1},
			{Timestamp: "2026-08-02T10:00:00Z", RawValue: 0},
		}, nil).Once()
	f.cache.EXPECT().Get("grid_fault_1").Return(&solis.Value{
		Timestamp: t0, RawValue: 2,
		StatusDecoded: []string{"Grid overvoltage"},
	}).Once()

	h, err := f.svc.StatusHistory(ctx, "grid_fault_1")
	require.NoError(t, err)
	assert.Equal(t, "Grid Fault 1 (Bitmask)", h.Name)
	require.Len(t, h.History, 3, "cleared state (raw 0) is kept")
	assert.Equal(t, t0.UTC().Format(period.TimestampLayout), h.History[0].Timestamp,
		"newest first")
	assert.Equal(t, "2026-08-02T10:00:00Z", h.History[1].Timestamp)
	assert.Equal(t, []string{"No grid"}, h.History[2].StatusDecoded)

	// A cached state that equals the newest stored change is not listed twice.
	f.store.EXPECT().GetErrorHistory(ctx, "grid_fault_1", mock.Anything, mock.Anything).
		Return([]*history.ErrorDataPoint{{
			Timestamp: "2026-08-02T10:00:00.000Z",
			RawValue:  1,
		}}, nil).Once()
	f.cache.EXPECT().Get("grid_fault_1").Return(&solis.Value{
		Timestamp: t0, RawValue: 1,
		StatusDecoded: []string{"No grid"},
	}).Once()
	h, err = f.svc.StatusHistory(ctx, "grid_fault_1")
	require.NoError(t, err)
	assert.Len(t, h.History, 1)

	// Store failure is best effort; the current value is still returned.
	f.store.EXPECT().GetErrorHistory(ctx, "solis_status", mock.Anything, mock.Anything).
		Return(nil, errors.New("disk")).Once()
	f.cache.EXPECT().Get("solis_status").Return(nil).Once()
	h, err = f.svc.StatusHistory(ctx, "solis_status")
	require.NoError(t, err)
	assert.Empty(t, h.History)

	_, err = f.svc.StatusHistory(ctx, "pv_energy_daily")
	assert.ErrorIs(t, err, ErrWrongKind)
}

func TestHealth(t *testing.T) {
	f := newFixture(t)
	f.hlth.EXPECT().Snapshot().Return(health.Snapshot{Status: health.StatusDegraded}).Once()
	assert.Equal(t, health.StatusDegraded, f.svc.Health().Status)
}
