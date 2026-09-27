package aggregator

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/health/mocks"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/utils/clocktest"
)

const iv = 10 * time.Second // debounce 40s, heartbeat 50s

var bg = context.Background()

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// countingStore observes runs and can inject failures.
type countingStore struct {
	Store
	runs atomic.Int32
	fail atomic.Bool
}

// WriteComputed counts a run once its write has completed.
func (c *countingStore) WriteComputed(ctx context.Context, w storage.ComputedWrite) error {
	defer c.runs.Add(1)
	return c.Store.WriteComputed(ctx, w)
}

func (c *countingStore) CloseState(ctx context.Context) (storage.CloseState, error) {
	if c.fail.Load() {
		return storage.CloseState{}, errors.New("disk I/O error")
	}
	return c.Store.CloseState(ctx)
}

// countingCache counts completed merges (the last step of a run).
type countingCache struct {
	*cache.Cache
	merges atomic.Int32
}

func (c *countingCache) Merge(domain string, v map[string]*solis.Value, at time.Time) {
	defer c.merges.Add(1)
	c.Cache.Merge(domain, v, at)
}

type env struct {
	t     *testing.T
	clk   *clocktest.Clock
	st    *storage.Storage
	store *countingStore
	bus   *eventbus.Bus
	cache *countingCache
	reg   *solis.Registry
	agg   *Aggregator
}

func newEnv(t *testing.T, start, cutover time.Time) *env {
	t.Helper()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	clk := clocktest.New(start)
	cfg := &config.StorageSettings{Path: filepath.Join(t.TempDir(), "s.db"),
		Synchronous: "NORMAL", TempStore: "MEMORY"}
	st, err := storage.New(cfg, reg, clk, zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	_, _, err = st.EnsureCutover(bg, period.Of(cutover))
	require.NoError(t, err)
	bus := eventbus.New()
	e := &env{t: t, clk: clk, st: st, store: &countingStore{Store: st}, bus: bus,
		cache: &countingCache{Cache: cache.New(bus)}, reg: reg}
	rep := mocks.NewMockReporter(t)
	rep.EXPECT().Report(health.Recovering, "disk I/O error").Maybe()
	rep.EXPECT().Report(health.Healthy, "").Maybe()
	e.agg, err = New(Deps{Store: e.store, Cache: e.cache, Bus: bus, Registry: reg, Clock: clk,
		PollInterval: iv, Timeout: time.Second, Reporter: rep, Log: zerolog.Nop()})
	require.NoError(t, err)
	return e
}

func (e *env) start() {
	require.NoError(e.t, e.agg.Start(bg))
	e.t.Cleanup(func() { _ = e.agg.Stop() })
	require.True(e.t, e.clk.BlockUntil(2), "beat ticker + heartbeat registered")
}

func (e *env) daily(key, day string, v float64) {
	require.NoError(e.t, e.st.WritePoll(bg, storage.PollWrite{
		Daily: []storage.DailyRow{{Key: key, Day: day, Value: v, Raw: v * 10}}}))
}

func (e *env) closeAll(day string) {
	var closes []storage.DayClose
	for _, k := range e.reg.DailyKeys() {
		closes = append(closes, storage.DayClose{Key: k, Day: day})
	}
	require.NoError(e.t, e.st.WritePoll(bg, storage.PollWrite{Close: closes}))
}

func (e *env) pollEvent() {
	e.bus.Publish(eventbus.Event{Kind: eventbus.ValuesUpdated, Domain: eventbus.DomainPoller,
		At: e.clk.Now()})
}

func (e *env) waitRuns(n int32) {
	require.Eventually(e.t, func() bool { return e.cache.merges.Load() >= n }, 2*time.Second,
		time.Millisecond, "want %d runs, have %d", n, e.cache.merges.Load())
}

func (e *env) cached(key string) float64 {
	v := e.cache.Get(key)
	require.NotNil(e.t, v, key)
	return v.DecodedValue
}

func TestNew_MissingDependencies(t *testing.T) {
	_, err := New(Deps{})
	assert.ErrorIs(t, err, ErrMissingDependency)
}

func TestColdStartStaysIdle(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-08-01 00:00"))
	e.start()
	e.pollEvent()
	e.clk.Advance(HeartbeatFactor * iv)
	e.clk.Advance(HeartbeatFactor * iv)
	assert.Never(t, func() bool { return e.store.runs.Load() > 0 }, 50*time.Millisecond,
		time.Millisecond)
	assert.Nil(t, e.cache.Get("pv_energy_monthly"))
}

func TestComputesAllLevelsFromDailyRows(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-07-15 00:00"))
	e.daily("pv_energy_daily", "2026-07-31", 7)
	e.daily("pv_energy_daily", "2026-08-01", 10)
	e.daily("pv_energy_daily", "2026-08-05", 5.25)
	e.daily("pv_energy_daily", "2026-08-09", 99) // future-dated: excluded by the bound
	e.daily("grid_export_daily", "2026-08-05", 3)
	e.daily("grid_import_daily", "2026-08-05", 4.5)
	e.start()

	e.pollEvent()
	e.waitRuns(1)
	assert.InDelta(t, 15.25, e.cached("pv_energy_monthly"), 1e-9)
	assert.InDelta(t, 22.25, e.cached("pv_energy_yearly"), 1e-9)
	assert.InDelta(t, 22.25, e.cached("pv_energy_total"), 1e-9)
	assert.InDelta(t, -1.5, e.cached("grid_energy_daily"), 1e-9)
	assert.InDelta(t, -1.5, e.cached("grid_energy_monthly"), 1e-9)
	assert.InDelta(t, -1.5, e.cached("grid_energy_yearly"), 1e-9)
	assert.InDelta(t, -1.5, e.cached("grid_energy_total"), 1e-9)
	assert.InDelta(t, 0.0, e.cached("backup_energy_monthly"), 1e-9)
	assert.Equal(t, "kWh", e.cache.Get("pv_energy_monthly").Unit)

	// Open previous month is persisted but not cached as "current".
	jul, err := e.st.GetMonthlyHistory(bg, "pv_energy_monthly", at("2026-07-01 00:00"),
		at("2026-07-01 00:00"))
	require.NoError(t, err)
	require.Len(t, jul, 1)
	assert.InDelta(t, 7.0, jul[0].Value, 1e-9)
	net, err := e.st.GetDailyHistory(bg, "grid_energy_daily", at("2026-08-05 00:00"),
		at("2026-08-05 00:00"))
	require.NoError(t, err)
	require.Len(t, net, 1)
	assert.InDelta(t, -1.5, net[0].Value, 1e-9)
}

func TestDebounceCoalescesPollEvents(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-08-01 00:00"))
	e.daily("pv_energy_daily", "2026-08-05", 1)
	e.start()

	e.pollEvent() // leading edge: immediate run
	e.waitRuns(1)
	e.clk.Advance(iv)
	e.pollEvent() // inside the window: arms one trailing run
	require.True(t, e.clk.BlockUntil(3), "debounce armed")
	for range 5 {
		e.pollEvent()
	}
	e.clk.Advance(2 * iv)
	assert.Never(t, func() bool { return e.store.runs.Load() > 1 }, 30*time.Millisecond,
		time.Millisecond)
	e.clk.Advance(iv) // 40s after the first run
	e.waitRuns(2)
	assert.Never(t, func() bool { return e.store.runs.Load() > 2 }, 30*time.Millisecond,
		time.Millisecond)
}

func TestHeartbeatFiresWithoutEvents(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-08-01 00:00"))
	e.daily("pv_energy_daily", "2026-08-05", 1)
	e.start()
	e.clk.Advance(HeartbeatFactor*iv - time.Second)
	assert.Never(t, func() bool { return e.store.runs.Load() > 0 }, 30*time.Millisecond,
		time.Millisecond)
	e.clk.Advance(time.Second)
	e.waitRuns(1)
	assert.InDelta(t, 1.0, e.cached("pv_energy_monthly"), 1e-9)
}

func TestOwnEventsAreIgnored(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-08-01 00:00"))
	e.daily("pv_energy_daily", "2026-08-05", 1)
	e.start()
	e.pollEvent()
	e.waitRuns(1)
	e.clk.Advance(DebounceFactor * iv)
	// The run's own cache merge published an aggregator-domain event: no feedback loop.
	assert.Never(t, func() bool { return e.store.runs.Load() > 1 }, 50*time.Millisecond,
		time.Millisecond)
}

func TestPeriodClosedFreezesMonthAndFoldsYear(t *testing.T) {
	e := newEnv(t, at("2027-01-01 00:30"), at("2026-11-20 00:00"))
	e.daily("pv_energy_daily", "2026-12-31", 20)
	e.daily("pv_energy_daily", "2026-11-25", 5)
	e.daily("pv_energy_daily", "2027-01-01", 0.5)
	e.closeAll("2026-12-31")
	e.start()

	e.bus.Publish(eventbus.Event{Kind: eventbus.PeriodClosed, Day: "2026-12-31"})
	e.waitRuns(1)

	st, err := e.st.CloseState(bg)
	require.NoError(t, err)
	assert.Equal(t, "2026-12", st.FrozenMonth)
	assert.Equal(t, "2026", st.FrozenYear)
	assert.Equal(t, "2026", st.BaselineYear)
	assert.Equal(t, "2026-12-31", st.FrozenNetDay)
	assert.InDelta(t, 25.0, st.Baseline["pv_energy_total"], 1e-9)

	// Total continuity across the fold: baseline 25 + 0.5 of the new year.
	assert.InDelta(t, 25.5, e.cached("pv_energy_total"), 1e-9)
	assert.InDelta(t, 0.5, e.cached("pv_energy_monthly"), 1e-9)
	assert.InDelta(t, 0.5, e.cached("pv_energy_yearly"), 1e-9)

	// A late (duplicate) close event is idempotent.
	e.bus.Publish(eventbus.Event{Kind: eventbus.PeriodClosed, Day: "2026-12-31"})
	e.waitRuns(2)
	st2, _ := e.st.CloseState(bg)
	assert.InDelta(t, 25.0, st2.Baseline["pv_energy_total"], 1e-9)
	assert.InDelta(t, 25.5, e.cached("pv_energy_total"), 1e-9)
}

func TestMissedCloseEventIsCaughtUpByHeartbeat(t *testing.T) {
	e := newEnv(t, at("2026-09-01 02:00"), at("2026-08-10 00:00"))
	e.daily("pv_energy_daily", "2026-08-31", 3)
	e.closeAll("2026-08-31") // the PeriodClosed event was lost
	e.start()
	e.clk.Advance(HeartbeatFactor * iv)
	e.waitRuns(1)
	st, err := e.st.CloseState(bg)
	require.NoError(t, err)
	assert.Equal(t, "2026-08", st.FrozenMonth)
}

func TestStoreErrorReportsRecoveringThenHealthy(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-08-01 00:00"))
	e.daily("pv_energy_daily", "2026-08-05", 1)
	e.store.fail.Store(true)
	e.start()
	e.pollEvent()
	require.Eventually(t, func() bool { return e.agg.State() == health.Recovering },
		time.Second, time.Millisecond)
	e.store.fail.Store(false)
	e.clk.Advance(DebounceFactor * iv)
	e.pollEvent()
	e.waitRuns(1)
	require.Eventually(t, func() bool { return e.agg.State() == health.Healthy },
		time.Second, time.Millisecond)
}

func TestBeatsWhileIdleAndStopIsIdempotent(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-08-01 00:00"))
	e.start()
	first := e.agg.LastBeat()
	e.clk.Advance(iv)
	require.Eventually(t, func() bool { return e.agg.LastBeat().After(first) }, time.Second,
		time.Millisecond)
	require.NoError(t, e.agg.Stop())
	require.NoError(t, e.agg.Stop())

	unstarted, err := New(e.agg.d)
	require.NoError(t, err)
	require.NoError(t, unstarted.Stop())
}

func TestStartFailsOnClosedBus(t *testing.T) {
	e := newEnv(t, at("2026-08-05 12:00"), at("2026-08-01 00:00"))
	e.bus.Close()
	assert.ErrorIs(t, e.agg.Start(bg), eventbus.ErrClosed)
}
