package health

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/utils/clocktest"
)

const interval = 10 * time.Second // sweep 5s, healthy 30s, start 50s, recovering 100s

var t0 = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

type fakeComp struct {
	name     string
	state    atomic.Int32
	beat     atomic.Pointer[time.Time]
	startErr error
	rep      Reporter
	stopped  atomic.Bool
	// startGate, when set, blocks Start until closed; stopEarly records a Stop that ran
	// while Start had not returned yet.
	startGate chan struct{}
	started   atomic.Bool
	stopEarly atomic.Bool
	order     *[]string
	orderMu   *sync.Mutex
}

func (f *fakeComp) Start(context.Context) error {
	if f.startGate != nil {
		<-f.startGate
	}
	f.started.Store(true)
	return f.startErr
}

func (f *fakeComp) Stop() error {
	if !f.started.Load() {
		f.stopEarly.Store(true)
	}
	f.stopped.Store(true)
	if f.order != nil {
		f.orderMu.Lock()
		*f.order = append(*f.order, f.name)
		f.orderMu.Unlock()
	}
	return nil
}
func (f *fakeComp) State() State { return State(f.state.Load()) }
func (f *fakeComp) LastBeat() time.Time {
	if b := f.beat.Load(); b != nil {
		return *b
	}
	return time.Time{}
}
func (f *fakeComp) Beat(t time.Time) { f.beat.Store(&t) }

type harness struct {
	t     *testing.T
	clk   *clocktest.Clock
	sup   *Supervisor
	mu    sync.Mutex
	comps map[string][]*fakeComp
	done  chan struct{}
}

func newHarness(t *testing.T) *harness {
	clk := clocktest.New(t0)
	return &harness{
		t: t, clk: clk, sup: New(context.Background(), interval, clk, zerolog.Nop()),
		comps: make(map[string][]*fakeComp),
	}
}

func (h *harness) factory(name string, mk func() *fakeComp) Factory {
	return func(r Reporter) (Component, error) {
		c := mk()
		c.name, c.rep = name, r
		h.mu.Lock()
		h.comps[name] = append(h.comps[name], c)
		h.mu.Unlock()
		return c, nil
	}
}

func (h *harness) instances(name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.comps[name])
}

func (h *harness) latest(name string) *fakeComp {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.comps[name]
	return c[len(c)-1]
}

func (h *harness) run() {
	h.done = make(chan struct{})
	go func() { h.sup.Run(); close(h.done) }()
	require.True(h.t, h.clk.BlockUntil(1), "ticker registered")
	h.waitSnapshot()
}

// waitSnapshot waits until the loop published a snapshot for the current fake time.
func (h *harness) waitSnapshot() {
	now := h.clk.Now()
	require.Eventually(h.t, func() bool { return !h.sup.Snapshot().At.Before(now) },
		time.Second, time.Millisecond)
}

// sweep advances one sweep interval and waits for it to be processed.
func (h *harness) sweep(n int) {
	for range n {
		if h.sup.Context().Err() != nil {
			return
		}
		h.clk.Advance(h.sup.SweepInterval())
		h.waitSnapshot()
	}
}

func (h *harness) shutdown() {
	h.sup.Shutdown()
	<-h.done
}

func TestHealthyComponentIsLeftAlone(t *testing.T) {
	h := newHarness(t)
	h.sup.Manage("poller", h.factory("poller", func() *fakeComp { return &fakeComp{} }))
	h.run()
	for range 20 {
		h.latest("poller").Beat(h.clk.Now())
		h.sweep(1)
	}
	assert.Equal(t, 1, h.instances("poller"))
	snap := h.sup.Snapshot()
	assert.Equal(t, StatusOK, snap.Status)
	assert.Equal(t, "healthy", snap.Components["poller"].State)
	assert.NotNil(t, snap.Components["poller"].LastBeat)
	h.shutdown()
	assert.ErrorIs(t, context.Cause(h.sup.Context()), ErrShutdown)
	assert.True(t, h.latest("poller").stopped.Load())
}

func TestNeverBeatingComponentRestartsAfterStartGrace(t *testing.T) {
	h := newHarness(t)
	h.sup.Manage("hub", h.factory("hub", func() *fakeComp { return &fakeComp{} }))
	h.run()
	h.sweep(10) // 50s = start grace, not yet exceeded
	assert.Equal(t, 1, h.instances("hub"))
	h.sweep(1)
	assert.Equal(t, 2, h.instances("hub"))
	assert.True(t, h.sup.managed[0].failures == 1)
	h.shutdown()
}

func TestRecoveringGetsLongerGrace(t *testing.T) {
	h := newHarness(t)
	h.sup.Manage("modbus", h.factory("modbus", func() *fakeComp { return &fakeComp{} }))
	h.run()
	c := h.latest("modbus")
	c.state.Store(int32(Recovering))
	c.Beat(h.clk.Now().Add(time.Millisecond))
	h.sweep(1)
	assert.Equal(t, StatusDegraded, h.sup.Snapshot().Status)
	h.sweep(19) // 100s = recovering grace
	assert.Equal(t, 1, h.instances("modbus"))
	h.sweep(2)
	assert.Equal(t, 2, h.instances("modbus"))
	h.shutdown()
}

func TestHealthySilenceRestartsAfterThreeIntervals(t *testing.T) {
	h := newHarness(t)
	h.sup.Manage("agg", h.factory("agg", func() *fakeComp { return &fakeComp{} }))
	h.run()
	h.latest("agg").Beat(h.clk.Now().Add(time.Millisecond))
	h.sweep(6) // 30s
	assert.Equal(t, 1, h.instances("agg"))
	h.sweep(1)
	assert.Equal(t, 2, h.instances("agg"))
	h.shutdown()
}

func TestCounterResetsOnlyAfterGenuineRecovery(t *testing.T) {
	h := newHarness(t)
	h.sup.Manage("p", h.factory("p", func() *fakeComp { return &fakeComp{} }))
	h.run()
	h.sweep(11) // stale -> restart #1
	require.Equal(t, 2, h.instances("p"))
	assert.Equal(t, 1, h.sup.Snapshot().Components["p"].Restarts)

	// Start() succeeded but no beat yet: counter must not reset.
	h.sweep(3)
	assert.Equal(t, 1, h.sup.Snapshot().Components["p"].Restarts)

	// Genuine heartbeat and survival of the healthy grace resets it.
	for range 7 {
		h.latest("p").Beat(h.clk.Now())
		h.sweep(1)
	}
	assert.Equal(t, 0, h.sup.Snapshot().Components["p"].Restarts)
	h.shutdown()
}

func TestBudgetExhaustionEscalatesFatal(t *testing.T) {
	h := newHarness(t)
	h.sup.Manage("poller", h.factory("poller", func() *fakeComp { return &fakeComp{} }))
	h.run()
	for h.sup.Context().Err() == nil {
		h.sweep(1)
	}
	<-h.done
	assert.Equal(t, 1+RestartBudget, h.instances("poller"))
	cause := context.Cause(h.sup.Context())
	require.ErrorIs(t, cause, ErrHealthFatal)
	var ce *ComponentError
	require.True(t, errors.As(cause, &ce))
	assert.Equal(t, "poller", ce.Component)
	assert.Contains(t, ce.Error(), "restart budget exhausted")

	snap := h.sup.Snapshot()
	assert.Equal(t, StatusFailed, snap.Status)
	assert.Equal(t, "poller", snap.Component)
	assert.True(t, h.latest("poller").stopped.Load())
}

func TestBudgetIsIndependentOfInterval(t *testing.T) {
	for _, iv := range []time.Duration{time.Second, 5 * time.Minute} {
		clk := clocktest.New(t0)
		sup := New(context.Background(), iv, clk, zerolog.Nop())
		var n atomic.Int32
		sup.Manage("c", func(Reporter) (Component, error) {
			n.Add(1)
			return &fakeComp{}, nil
		})
		done := make(chan struct{})
		go func() { sup.Run(); close(done) }()
		require.True(t, clk.BlockUntil(1))
		for sup.Context().Err() == nil {
			now := clk.Now()
			clk.Advance(sup.SweepInterval())
			require.Eventually(t, func() bool { return !sup.Snapshot().At.Before(now) },
				time.Second, time.Millisecond)
		}
		<-done
		assert.Equal(t, int32(1+RestartBudget), n.Load(), iv)
	}
}

func TestFactoryPanicAndStartErrorCountAsFailures(t *testing.T) {
	h := newHarness(t)
	var calls atomic.Int32
	h.sup.Manage("x", func(r Reporter) (Component, error) {
		switch calls.Add(1) {
		case 1:
			panic("boom")
		case 2:
			return nil, errors.New("ctor failed")
		case 3:
			return &fakeComp{startErr: errors.New("start failed")}, nil
		default:
			return &fakeComp{}, nil
		}
	})
	h.run()
	assert.Equal(t, "failed", h.sup.Snapshot().Components["x"].State)
	require.Eventually(t, func() bool {
		h.sweep(1)
		return calls.Load() >= 4
	}, time.Second, time.Millisecond)
	assert.Equal(t, 3, h.sup.Snapshot().Components["x"].Restarts)
	assert.NoError(t, context.Cause(h.sup.Context()))
	h.shutdown()
}

func TestReportedFailureRestartsImmediately(t *testing.T) {
	h := newHarness(t)
	h.sup.Manage("hub", h.factory("hub", func() *fakeComp { return &fakeComp{} }))
	h.run()
	old := h.latest("hub")
	old.rep.Report(Failed, "listener died")
	require.Eventually(t, func() bool { return h.instances("hub") == 2 }, time.Second,
		time.Millisecond)
	// Reports from the stopped generation are ignored.
	old.rep.Report(Failed, "late")
	h.latest("hub").Beat(h.clk.Now())
	h.sweep(1)
	assert.Equal(t, 2, h.instances("hub"))
	h.shutdown()
}

func TestWatchedFailureIsFatal(t *testing.T) {
	h := newHarness(t)
	var st atomic.Int32
	h.sup.Watch("storage", func() (State, string) {
		return State(st.Load()), "disk I/O error"
	})
	h.run()
	assert.Equal(t, StatusOK, h.sup.Snapshot().Status)
	st.Store(int32(Failed))
	h.sweep(1)
	<-h.done
	cause := context.Cause(h.sup.Context())
	assert.ErrorIs(t, cause, ErrHealthFatal)
	snap := h.sup.Snapshot()
	assert.Equal(t, StatusFailed, snap.Status)
	assert.Equal(t, "storage", snap.Component)
	assert.Equal(t, "disk I/O error", snap.Reason)
	assert.False(t, snap.Components["storage"].Restartable)
}

func TestShutdownStopsInReverseOrder(t *testing.T) {
	h := newHarness(t)
	var order []string
	var mu sync.Mutex
	for _, n := range []string{"modbus", "poller", "aggregator", "hub"} {
		h.sup.Manage(n, h.factory(n, func() *fakeComp {
			return &fakeComp{order: &order, orderMu: &mu}
		}))
	}
	h.run()
	h.shutdown()
	assert.Equal(t, []string{"hub", "aggregator", "poller", "modbus"}, order)
	assert.ErrorIs(t, context.Cause(h.sup.Context()), ErrShutdown)
}

func TestFreshSupervisorStartsAtZero(t *testing.T) {
	sup := New(context.Background(), interval, clocktest.New(t0), zerolog.Nop())
	snap := sup.Snapshot()
	assert.Equal(t, StatusOK, snap.Status)
	assert.Empty(t, snap.Components)
	assert.Equal(t, MinSweepInterval, sup.SweepInterval())
	sup2 := New(context.Background(), time.Minute, clocktest.New(t0), zerolog.Nop())
	assert.Equal(t, 30*time.Second, sup2.SweepInterval())
}

func TestStateStrings(t *testing.T) {
	assert.Equal(t, "healthy", Healthy.String())
	assert.Equal(t, "recovering", Recovering.String())
	assert.Equal(t, "failed", Failed.String())
}

func TestStopWaitsForStart(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	h.sup.Manage("hub", h.factory("hub", func() *fakeComp { return &fakeComp{startGate: gate} }))
	h.run()
	h.sup.Shutdown()
	time.Sleep(10 * time.Millisecond) // give a premature Stop the chance to run
	assert.False(t, h.latest("hub").stopped.Load(), "Stop must wait for Start")
	close(gate)
	<-h.done
	c := h.latest("hub")
	assert.True(t, c.stopped.Load())
	assert.False(t, c.stopEarly.Load())
}
