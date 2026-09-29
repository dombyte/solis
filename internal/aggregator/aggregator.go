// Package aggregator is the event-driven runner that recomputes monthly, yearly, total and
// net values from daily rows. It reacts to the poller's cache events (debounced 4x poll
// interval), to PeriodClosed events and to a heartbeat (5x) and has no reference to the
// poller. The math lives in the pure aggregation package.
package aggregator

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/util"
)

// Cadence factors of the poll interval (constants, not config).
const (
	DebounceFactor  = 4
	HeartbeatFactor = 5
	eventBuffer     = 16
	subscriberName  = "aggregator"
)

// ErrMissingDependency is returned by New when a required dependency is nil.
var ErrMissingDependency = errors.New("aggregator: missing dependency")

// Store is the aggregator's storage write domain.
type Store = storage.AggregatorStore

// Cache receives the computed values (aggregator domain).
type Cache interface {
	Merge(domain string, values map[string]*solis.Value, at time.Time)
}

// Registry provides the computation definitions.
type Registry interface {
	ByKey(key string) (solis.Register, bool)
	Edges(l period.Level) []solis.Edge
	NetPairs(l period.Level) []solis.NetPair
}

// Deps are the aggregator's dependencies.
type Deps struct {
	Store        Store
	Cache        Cache
	Bus          eventbus.Subscriber
	Registry     Registry
	Clock        util.Clock
	PollInterval time.Duration
	// Timeout bounds the storage calls of one run.
	Timeout  time.Duration
	Reporter health.Reporter
	Log      zerolog.Logger
}

// Aggregator implements health.Component.
type Aggregator struct {
	*health.Status
	d Deps

	mu      sync.Mutex
	cancel  context.CancelFunc
	unsub   func()
	done    chan struct{}
	stopped bool

	lastRun time.Time
}

// New validates dependencies and returns a stopped aggregator.
func New(d Deps) (*Aggregator, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	return &Aggregator{Status: health.NewStatus(d.Reporter, d.Clock), d: d}, nil
}

func (d Deps) validate() error {
	return util.RequireAll(ErrMissingDependency,
		util.Requirement{Name: "Store", OK: d.Store != nil},
		util.Requirement{Name: "Cache", OK: d.Cache != nil},
		util.Requirement{Name: "Bus", OK: d.Bus != nil},
		util.Requirement{Name: "Registry", OK: d.Registry != nil},
		util.Requirement{Name: "Clock", OK: d.Clock != nil},
		util.Requirement{Name: "Reporter", OK: d.Reporter != nil},
		util.Requirement{Name: "PollInterval", OK: d.PollInterval > 0},
	)
}

// Start subscribes to the bus and starts the loop. It is a no-op once started or
// stopped, so a late Start never leaks a subscription.
func (a *Aggregator) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped || a.cancel != nil {
		return nil
	}
	events, unsub, err := a.d.Bus.Subscribe(subscriberName, eventBuffer, eventbus.Coalesce)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	a.cancel, a.unsub, a.done = cancel, unsub, make(chan struct{})
	a.Beat()
	go a.loop(ctx, events)
	return nil
}

// Stop ends the loop and releases the subscription (idempotent).
func (a *Aggregator) Stop() error {
	a.mu.Lock()
	if a.stopped || a.cancel == nil {
		a.stopped = true
		a.mu.Unlock()
		return nil
	}
	a.stopped = true
	cancel, unsub, done := a.cancel, a.unsub, a.done
	a.mu.Unlock()
	cancel()
	<-done
	unsub()
	return nil
}

// loop multiplexes events, the debounce timer, the heartbeat and the liveness beat.
func (a *Aggregator) loop(ctx context.Context, events <-chan eventbus.Event) {
	a.d.Log.Debug().Dur("interval", a.d.PollInterval).Msg("aggregator loop started")
	defer close(a.done)
	iv := a.d.PollInterval
	beat := a.d.Clock.NewTicker(iv)
	defer beat.Stop()
	heartbeat := a.d.Clock.NewTimer(HeartbeatFactor * iv)
	defer heartbeat.Stop()
	debounce := a.d.Clock.NewTimer(time.Hour)
	debounce.Stop()
	armed := false

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			a.d.Log.Debug().Stringer("kind", e.Kind).Str("domain", e.Domain).
				Int("keys", len(e.Keys)).Msg("event received")
			armed = a.onEvent(ctx, e, debounce, armed)
		case <-debounce.C():
			armed = false
			a.run(ctx)
		case <-heartbeat.C():
			a.d.Log.Debug().Msg("heartbeat triggered")
			a.run(ctx)
		case <-beat.C():
			a.Beat()
			continue
		}
		heartbeat.Reset(a.untilHeartbeat())
	}
}

// onEvent runs immediately for PeriodClosed and for the first poller event of a debounce
// window; later events in the window arm one trailing run. Own (aggregator-domain)
// events are ignored.
func (a *Aggregator) onEvent(ctx context.Context, e eventbus.Event, debounce util.Timer,
	armed bool,
) bool {
	switch {
	case e.Kind == eventbus.PeriodClosed:
		a.run(ctx)
		return armed
	case e.Domain != eventbus.DomainPoller:
		return armed
	}
	window := DebounceFactor * a.d.PollInterval
	since := a.d.Clock.Now().Sub(a.lastRun)
	if a.lastRun.IsZero() || since >= window {
		a.run(ctx)
		return armed
	}
	if !armed {
		debounce.Reset(window - since)
	}
	return true
}

// untilHeartbeat is the time left until the heartbeat fallback fires.
func (a *Aggregator) untilHeartbeat() time.Duration {
	hb := HeartbeatFactor * a.d.PollInterval
	if a.lastRun.IsZero() {
		return hb
	}
	if left := hb - a.d.Clock.Now().Sub(a.lastRun); left > 0 {
		return left
	}
	return hb
}

// run performs one aggregation run; errors are reported as Recovering, never fatal.
func (a *Aggregator) run(ctx context.Context) {
	now := a.d.Clock.Now() // captured once per run
	a.Beat()
	rctx, cancel := a.runContext(ctx)
	defer cancel()
	ok, err := a.d.Store.HasDailyData(rctx)
	if err == nil && !ok {
		a.d.Log.Debug().Msg("aggregation skipped: no daily data yet")
		return // cold start: stay idle until daily data exists
	}
	a.lastRun = now
	a.d.Log.Debug().Msg("aggregation run starting")
	if err == nil {
		err = a.execute(rctx, now)
	}
	if err != nil && ctx.Err() != nil {
		return // stopped mid-run: not a failure (review AGG-L6)
	}
	if err != nil {
		a.d.Log.Error().Err(err).Msg("aggregation run failed")
		a.Set(health.Recovering, err.Error())
		return
	}
	a.d.Log.Debug().Msg("aggregation run completed")
	a.Set(health.Healthy, "")
}

func (a *Aggregator) runContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.d.Timeout > 0 {
		return context.WithTimeout(ctx, a.d.Timeout)
	}
	return context.WithCancel(ctx)
}
