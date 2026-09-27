// Package poller runs the non-overlapping poll loop: it reads the planned register blocks
// through the current Modbus reader, decodes them, attributes daily values to days
// (rollover), writes daily and status rows (its exclusive write domain), replaces its key
// domain in the cache and emits PeriodClosed. It never references the aggregator.
package poller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/utils"
)

// ErrMissingDependency is returned by New when a required dependency is nil.
var ErrMissingDependency = errors.New("poller: missing dependency")

// Reader is the Modbus access the poller needs.
type Reader interface {
	ReadRegisters(ctx context.Context, addr, count uint16) ([]uint16, error)
	IsConnected() bool
}

// Source yields the current Reader (swapped when the Modbus component restarts).
type Source interface {
	Load() (Reader, bool)
}

// Store is the poller's storage write domain.
type Store = storage.PollerStore

// Cache receives the poller's key domain.
type Cache interface {
	ReplaceDomain(domain string, values map[string]*solis.Value, at time.Time)
}

// Decoder decodes read blocks and derives live values.
type Decoder interface {
	DecodeBlock(b solis.Block, raw []uint16, at time.Time) map[string]*solis.Value
	Derive(values map[string]*solis.Value, at time.Time)
}

// Registry provides the read plan and register classes.
type Registry interface {
	Blocks() []solis.Block
	DailyKeys() []string
	ByKey(key string) (solis.Register, bool)
}

// Deps are the poller's dependencies.
type Deps struct {
	Settings config.PollerSettings
	Rollover period.Rollover
	Source   Source
	Store    Store
	Cache    Cache
	Bus      eventbus.Publisher
	Decoder  Decoder
	Registry Registry
	Clock    utils.Clock
	// Timeout bounds storage calls.
	Timeout  time.Duration
	Reporter health.Reporter
	Log      zerolog.Logger
}

// Poller implements health.Component.
type Poller struct {
	*health.Status
	d    Deps
	attr *DayAttributor

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool

	// Loop-owned state.
	seeded        bool
	lastStatus    map[string]float64
	lastDaily     map[string]*solis.Value
	pendingCloses map[string]string
	emitted       map[string]bool
}

// New validates dependencies and returns a stopped poller.
func New(d Deps) (*Poller, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	return &Poller{
		Status:        health.NewStatus(d.Reporter, d.Clock),
		d:             d,
		attr:          NewDayAttributor(d.Rollover, d.Registry.DailyKeys()),
		lastStatus:    make(map[string]float64),
		lastDaily:     make(map[string]*solis.Value),
		pendingCloses: make(map[string]string),
		emitted:       make(map[string]bool),
	}, nil
}

func (d Deps) validate() error {
	required := []bool{d.Source != nil, d.Store != nil, d.Cache != nil, d.Bus != nil,
		d.Decoder != nil, d.Registry != nil, d.Clock != nil, d.Reporter != nil,
		d.Settings.Interval > 0}
	for _, ok := range required {
		if !ok {
			return ErrMissingDependency
		}
	}
	return nil
}

// Start launches the poll loop; the first poll runs immediately.
func (p *Poller) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cancel, p.done = cancel, make(chan struct{})
	p.mu.Unlock()
	p.Beat()
	go p.loop(ctx)
	return nil
}

// Stop ends the loop (idempotent).
func (p *Poller) Stop() error {
	p.mu.Lock()
	if p.stopped || p.cancel == nil {
		p.stopped = true
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	cancel, done := p.cancel, p.done
	p.mu.Unlock()
	cancel()
	<-done
	return nil
}

// loop schedules polls without overlap: the next poll starts one interval after the
// previous start (or immediately when a poll took longer).
func (p *Poller) loop(ctx context.Context) {
	defer close(p.done)
	timer := p.d.Clock.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C():
			start := p.d.Clock.Now()
			p.pollCycle(ctx, start)
			timer.Reset(max(0, p.d.Settings.Interval-p.d.Clock.Now().Sub(start)))
		}
	}
}

// pollCycle is one poll: seed if needed, read, attribute, persist, cache, emit.
func (p *Poller) pollCycle(ctx context.Context, start time.Time) {
	p.Beat()
	defer p.Beat()
	if !p.seeded {
		if err := p.seed(ctx, start); err != nil {
			p.d.Log.Error().Err(err).Str("tag", "storage_failure").Msg("seeding failed")
			p.Set(health.Recovering, "seeding failed: "+err.Error())
			return
		}
		p.d.Log.Debug().Msg("seeding complete")
	}
	reader, ok := p.d.Source.Load()
	if !ok || !reader.IsConnected() {
		p.Set(health.Recovering, "modbus not connected")
		return
	}
	values, err := p.readAll(ctx, reader, start)
	if err != nil {
		p.d.Log.Warn().Err(err).Msg("poll failed")
		p.Set(health.Recovering, "poll failed: "+err.Error())
		return
	}
	p.d.Log.Debug().Int("values", len(values)).Msg("poll cycle read complete")
	p.d.Decoder.Derive(values, start)
	if err := p.persist(ctx, values, start); err != nil {
		p.d.Log.Error().Err(err).Str("tag", "storage_failure").Msg("storing poll failed")
		p.Set(health.Recovering, "storage: "+err.Error())
		return
	}
	p.d.Log.Debug().Dur("duration", p.d.Clock.Now().Sub(start)).Msg("poll cycle complete")
	p.Set(health.Healthy, "")
}

// storageContext bounds a storage call.
func (p *Poller) storageContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if p.d.Timeout > 0 {
		return context.WithTimeout(ctx, p.d.Timeout)
	}
	return context.WithCancel(ctx)
}

// seed loads the open-day rows (one query), last status values and watermarks.
func (p *Poller) seed(ctx context.Context, now time.Time) error {
	sctx, cancel := p.storageContext(ctx)
	defer cancel()
	s, err := p.d.Store.Seed(sctx, p.attr.SeedDays(now))
	if err != nil {
		return err
	}
	p.attr.Seed(now, s.Daily, s.Closed)
	for k, v := range s.Status {
		p.lastStatus[k] = v
	}
	p.seeded = true
	return nil
}
