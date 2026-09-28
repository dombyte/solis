package poller

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/health/mocks"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/utils"
	"github.com/dombyte/solis/internal/utils/clocktest"
)

const pollEvery = 5 * time.Second

var bg = context.Background()

// device is a fake inverter: register address -> value.
type device struct {
	mu         sync.Mutex
	regs       map[uint16]uint16
	connected  atomic.Bool
	failNext   atomic.Int32
	dropOnFail atomic.Bool // failures are transport errors that drop the connection
	reads      atomic.Int32
}

func newDevice() *device {
	d := &device{regs: make(map[uint16]uint16)}
	d.connected.Store(true)
	return d
}

func (d *device) set(addr uint16, v uint16) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.regs[addr] = v
}

func (d *device) ReadRegisters(_ context.Context, addr, count uint16) ([]uint16, error) {
	d.reads.Add(1)
	if d.failNext.Load() > 0 {
		d.failNext.Add(-1)
		if d.dropOnFail.Load() { // like modbus.Client.markLost on a transport error
			d.connected.Store(false)
		}
		return nil, errors.New("timeout")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]uint16, count)
	for i := range out {
		out[i] = d.regs[addr+uint16(i)]
	}
	return out, nil
}

func (d *device) IsConnected() bool { return d.connected.Load() }

type env struct {
	t      *testing.T
	clk    *clocktest.Clock
	dev    *device
	st     *storage.Storage
	cache  *cache.Cache
	bus    *eventbus.Bus
	events <-chan eventbus.Event
	p      *Poller
	polls  atomic.Int32
}

// countingCache counts completed poll cycles (cache replacement is the last step).
type countingCache struct {
	*cache.Cache
	n *atomic.Int32
}

func (c countingCache) ReplaceDomain(d string, v map[string]*solis.Value, at time.Time) {
	defer c.n.Add(1)
	c.Cache.ReplaceDomain(d, v, at)
}

func newEnv(t *testing.T, start time.Time) *env {
	t.Helper()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	clk := clocktest.New(start)
	cfg := storage.Settings{
		Path:        filepath.Join(t.TempDir(), "s.db"),
		Synchronous: "NORMAL", TempStore: "MEMORY",
	}
	st, err := storage.New(cfg, reg, clk, zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	bus := eventbus.New()
	events, unsub, err := bus.Subscribe("test", 64, eventbus.Lossless)
	require.NoError(t, err)
	t.Cleanup(unsub)

	e := &env{
		t: t, clk: clk, dev: newDevice(), st: st, cache: cache.New(bus, zerolog.Nop()), bus: bus,
		events: events,
	}
	var src utils.Slot[Reader]
	src.Store(e.dev)
	rep := mocks.NewMockReporter(t)
	rep.EXPECT().Report(health.Recovering, mock.Anything).Maybe()
	rep.EXPECT().Report(health.Healthy, "").Maybe()
	roll := rollover(t, "23:59")
	e.p, err = New(Deps{
		Settings: Settings{
			Interval: pollEvery, BlockAttempts: 1,
			BlockRetryDelay: time.Second, PollTimeout: 5 * time.Second,
		},
		Rollover: roll, Source: &src, Store: st, Cache: countingCache{e.cache, &e.polls},
		Bus: bus, Decoder: solis.NewDecoder(reg, zerolog.Nop()), Registry: reg, Clock: clk,
		Timeout: time.Second, Reporter: rep, Log: zerolog.Nop(),
	})
	require.NoError(t, err)
	return e
}

func (e *env) start() {
	require.NoError(e.t, e.p.Start(bg))
	e.t.Cleanup(func() { _ = e.p.Stop() })
}

func (e *env) waitPolls(n int32) {
	require.Eventually(e.t, func() bool { return e.polls.Load() >= n }, 2*time.Second,
		time.Millisecond, "want %d polls, have %d", n, e.polls.Load())
}

// next advances to the next scheduled poll and waits for it.
func (e *env) next() {
	n := e.polls.Load()
	require.True(e.t, e.clk.BlockUntil(1))
	e.clk.Advance(pollEvery)
	e.waitPolls(n + 1)
}

func (e *env) daily(key, day string) float64 {
	d, _ := time.ParseInLocation("2006-01-02", day, time.Local)
	pts, err := e.st.GetDailyHistory(bg, key, d, d)
	require.NoError(e.t, err)
	require.Len(e.t, pts, 1, "%s %s", key, day)
	return pts[0].Value
}

func TestPoll_WritesDailyStatusAndCache(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	e.dev.set(33035, 250)    // pv_energy_daily 25.0
	e.dev.set(33058, 5230)   // pv_total_power
	e.dev.set(33095, 0x0003) // solis_status Generating
	e.dev.set(33130, 0xFFFF) // grid_power = -2 W (Int32)
	e.dev.set(33131, 0xFFFE)
	e.dev.set(33135, 1)   // discharging
	e.dev.set(33150, 800) // battery_power
	e.start()
	e.waitPolls(1)

	assert.InDelta(t, 25.0, e.daily("pv_energy_daily", "2026-08-05"), 1e-9)
	assert.InDelta(t, 5230.0, e.cache.Get("pv_total_power").DecodedValue, 1e-9)
	assert.InDelta(t, -2.0, e.cache.Get("grid_power").DecodedValue, 1e-9)
	assert.InDelta(t, -800.0, e.cache.Get(solis.KeyBatteryPowerSigned).DecodedValue, 1e-9)
	assert.Equal(t, "Generating",
		e.cache.Get("solis_status").StatusDecoded.(map[string]string)["name"])
	assert.Nil(t, e.cache.Get("pv_energy_monthly"), "computed keys are not the poller's")
	assert.Equal(t, health.Healthy, e.p.State())

	// Status is written on change only.
	e.next()
	e.dev.set(33095, 0x000F)
	e.next()
	hist, err := e.st.GetErrorHistory(bg, "solis_status", time.Time{}, e.clk.Now())
	require.NoError(t, err)
	assert.Len(t, hist, 2)
}

func TestPoll_NonOverlappingSchedule(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	e.start()
	e.waitPolls(1) // immediately
	require.True(t, e.clk.BlockUntil(1))
	e.clk.Advance(pollEvery - time.Second)
	assert.Never(t, func() bool { return e.polls.Load() > 1 }, 30*time.Millisecond,
		time.Millisecond)
	e.clk.Advance(time.Second)
	e.waitPolls(2)
}

func TestPoll_DisconnectedIsRecovering(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	e.dev.connected.Store(false)
	e.start()
	require.Eventually(t, func() bool { return e.p.State() == health.Recovering }, time.Second,
		time.Millisecond)
	assert.Zero(t, e.dev.reads.Load())
	assert.False(t, e.p.LastBeat().IsZero())

	e.dev.connected.Store(true)
	e.next()
	assert.Equal(t, health.Healthy, e.p.State())
}

func TestPoll_BlockRetryThenSuccess(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	e.dev.set(33035, 10)
	e.dev.failNext.Store(1)
	e.start()
	require.True(t, e.clk.BlockUntil(1)) // retry delay timer
	e.clk.Advance(time.Second)
	e.waitPolls(1)
	assert.InDelta(t, 1.0, e.daily("pv_energy_daily", "2026-08-05"), 1e-9)
}

func TestPoll_TransportFailureSkipsRetry(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	e.dev.dropOnFail.Store(true)
	e.dev.failNext.Store(1)
	e.start()
	require.Eventually(t, func() bool { return e.p.State() == health.Recovering }, time.Second,
		time.Millisecond)
	assert.Equal(t, int32(1), e.dev.reads.Load(), "a lost connection is not retried")
	assert.Zero(t, e.polls.Load())
}

func TestPoll_ReadFailureDiscardsPartialPoll(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	e.dev.failNext.Store(2) // both attempts of block 1 fail
	e.start()
	require.True(t, e.clk.BlockUntil(1))
	e.clk.Advance(time.Second)
	require.Eventually(t, func() bool { return e.p.State() == health.Recovering }, time.Second,
		time.Millisecond)
	assert.Zero(t, e.polls.Load())
}

func TestPoll_MidnightRolloverEmitsPeriodClosed(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 23, 58, 0, 0, time.Local))
	e.dev.set(33035, 300) // pv 30.0
	e.dev.set(33175, 50)  // grid_export 5.0
	e.start()
	e.waitPolls(1)

	e.clk.Set(time.Date(2026, 8, 6, 0, 5, 0, 0, time.Local))
	e.dev.set(33035, 1) // pv reset -> 0.1
	e.next()
	assert.InDelta(t, 30.0, e.daily("pv_energy_daily", "2026-08-05"), 1e-9)
	assert.InDelta(t, 0.1, e.daily("pv_energy_daily", "2026-08-06"), 1e-9)
	assert.InDelta(t, 5.0, e.daily("grid_export_daily", "2026-08-05"), 1e-9)

	var closed eventbus.Event
	require.Eventually(t, func() bool {
		select {
		case ev := <-e.events:
			if ev.Kind == eventbus.PeriodClosed {
				closed = ev
				return true
			}
		default:
		}
		return false
	}, time.Second, time.Millisecond)
	assert.Equal(t, "2026-08-05", closed.Day)

	// A late write for the closed day is rejected by storage (not by the poller).
	err := e.st.WritePoll(bg, storage.PollWrite{Daily: []storage.DailyRow{
		{Key: "pv_energy_daily", Day: "2026-08-05", Value: 99},
	}})
	assert.ErrorIs(t, err, storage.ErrPeriodClosed)

	// At window end the remaining keys are force-closed.
	e.clk.Set(time.Date(2026, 8, 6, 0, 59, 0, 0, time.Local))
	e.next()
	stt, err := e.st.CloseState(bg)
	require.NoError(t, err)
	assert.Equal(t, "2026-08-05", stt.ClosedThrough)
}

func TestPoll_ColdStartSeedsFromStorage(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 6, 0, 10, 0, 0, time.Local))
	require.NoError(t, e.st.WritePoll(bg, storage.PollWrite{Daily: []storage.DailyRow{
		{Key: "pv_energy_daily", Day: "2026-08-05", Value: 30, Raw: 300},
	}}))
	e.dev.set(33035, 2) // 0.2 after the inverter reset
	e.start()
	e.waitPolls(1)
	assert.InDelta(t, 30.0, e.daily("pv_energy_daily", "2026-08-05"), 1e-9)
	assert.InDelta(t, 0.2, e.daily("pv_energy_daily", "2026-08-06"), 1e-9)
}

func TestPoll_MidDayDipKeepsCachedMax(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	e.dev.set(33035, 100)
	e.start()
	e.waitPolls(1)
	e.dev.set(33035, 3) // reboot
	e.next()
	assert.InDelta(t, 10.0, e.cache.Get("pv_energy_daily").DecodedValue, 1e-9)
	assert.InDelta(t, 10.0, e.daily("pv_energy_daily", "2026-08-05"), 1e-9)
}

func TestPoll_SeedFailureIsRecovering(t *testing.T) {
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	require.NoError(t, e.st.Close())
	e.start()
	require.Eventually(t, func() bool { return e.p.State() == health.Recovering }, time.Second,
		time.Millisecond)
}

func TestNewAndStop(t *testing.T) {
	_, err := New(Deps{})
	assert.ErrorIs(t, err, ErrMissingDependency)
	e := newEnv(t, time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	require.NoError(t, e.p.Stop()) // never started
	require.NoError(t, e.p.Stop())
}

func TestMaxDay(t *testing.T) {
	assert.Equal(t, "2026-08-05", maxDay("", "2026-08-05"))
	assert.Equal(t, "2026-08-06", maxDay("2026-08-06", "2026-08-05"))
}
