package health

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/utils"
)

// reportBuffer sizes the push channel; reports are wake-ups, state lives in the entry.
const reportBuffer = 32

// Supervisor owns the root context and every restartable component.
type Supervisor struct {
	interval time.Duration
	clock    utils.Clock
	log      zerolog.Logger

	ctx    context.Context
	cancel context.CancelCauseFunc
	wake   chan struct{}

	mu      sync.Mutex // guards the registries; the Run loop is the only mutator after Run
	managed []*managed
	watched []*watched

	snap atomic.Pointer[Snapshot]
}

type watched struct {
	name   string
	probe  Probe
	state  State
	reason string
}

// New creates a supervisor whose root context derives from parent. pollInterval scales
// every grace period.
func New(parent context.Context, pollInterval time.Duration, clock utils.Clock,
	log zerolog.Logger) *Supervisor {
	ctx, cancel := context.WithCancelCause(parent)
	s := &Supervisor{
		interval: pollInterval, clock: clock, log: log,
		ctx: ctx, cancel: cancel, wake: make(chan struct{}, reportBuffer),
	}
	s.snap.Store(&Snapshot{Status: StatusOK, Components: map[string]ComponentStatus{},
		At: clock.Now()})
	return s
}

// Context is the root context; every component Start gets a child of it.
func (s *Supervisor) Context() context.Context { return s.ctx }

// Shutdown cancels the root context with the clean-shutdown cause.
func (s *Supervisor) Shutdown() { s.cancel(ErrShutdown) }

// Manage registers a restartable component; Run creates it through the factory.
func (s *Supervisor) Manage(name string, f Factory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managed = append(s.managed, &managed{name: name, factory: f})
}

// Watch registers a non-restartable part whose failure escalates immediately.
func (s *Supervisor) Watch(name string, p Probe) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watched = append(s.watched, &watched{name: name, probe: p})
}

// Snapshot returns the last published health snapshot; it never blocks on components.
func (s *Supervisor) Snapshot() Snapshot {
	return *s.snap.Load()
}

// SweepInterval is poll interval / 2 with a floor of MinSweepInterval.
func (s *Supervisor) SweepInterval() time.Duration {
	return max(s.interval/2, MinSweepInterval)
}

// Run starts all components in registration order, supervises them until the root
// context is cancelled, then stops them in reverse order. The caller reads the cause
// with context.Cause(s.Context()).
func (s *Supervisor) Run() {
	s.mu.Lock()
	for _, m := range s.managed {
		s.start(m)
	}
	s.mu.Unlock()
	s.sweep()

	ticker := s.clock.NewTicker(s.SweepInterval())
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			s.stopAll()
			return
		case <-s.wake:
			s.sweep()
		case <-ticker.C():
			s.sweep()
		}
	}
}

// sweep applies the state model to every component and publishes a snapshot.
func (s *Supervisor) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	for _, m := range s.managed {
		if s.ctx.Err() != nil {
			break
		}
		if restart, reason := m.evaluate(now, s.interval); restart {
			s.restart(m, reason)
		}
	}
	for _, w := range s.watched {
		w.state, w.reason = w.probe()
		if w.state == Failed && s.ctx.Err() == nil {
			s.fatal(w.name, "non-restartable component failed: "+w.reason)
		}
	}
	s.publish(now)
}

// restart recreates m within the budget or latches it and escalates. Caller holds mu.
func (s *Supervisor) restart(m *managed, reason string) {
	if m.failures >= RestartBudget {
		m.latched = true
		m.setReason("restart budget exhausted: " + reason)
		s.fatal(m.name, "restart budget exhausted: "+reason)
		return
	}
	m.failures++
	s.log.Warn().Str("target", m.name).Int("attempt", m.failures).Str("reason", reason).
		Msg("restarting component")
	s.stop(m)
	s.start(m)
}

// fatal cancels the root context with a wrapped ErrHealthFatal (no-op if cancelled).
func (s *Supervisor) fatal(name, reason string) {
	s.log.Error().Str("target", name).Str("reason", reason).Msg("health fatal escalation")
	s.cancel(&ComponentError{Component: name, Reason: reason, Err: ErrHealthFatal})
}

// start creates a fresh instance and starts it without waiting for Start. Caller holds mu.
func (s *Supervisor) start(m *managed) {
	gen := m.reset(s.clock.Now())
	rep := &reporter{s: s, m: m, gen: gen}
	comp, err := safeFactory(m.factory, rep)
	if err != nil {
		m.comp = nil
		m.setReason(fmt.Sprintf("factory failed: %v", err))
		s.log.Error().Str("target", m.name).Err(err).Msg("component factory failed")
		return
	}
	m.comp = comp
	ctx, cancel := context.WithCancel(s.ctx)
	m.cancel = cancel
	go func() {
		if err := comp.Start(ctx); err != nil {
			rep.Report(Failed, "start failed: "+err.Error())
		}
	}()
}

// safeFactory converts a factory panic into an error (counts as a failure).
func safeFactory(f Factory, r Reporter) (c Component, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("factory panic: %v", p)
		}
	}()
	return f(r)
}

// stop stops m's current instance, bounded by StopTimeout. Caller holds mu.
func (s *Supervisor) stop(m *managed) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.comp == nil {
		return
	}
	done := make(chan error, 1)
	comp := m.comp
	go func() { done <- comp.Stop() }()
	timer := s.clock.NewTimer(StopTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			s.log.Warn().Str("target", m.name).Err(err).Msg("component stop failed")
		}
	case <-timer.C():
		s.log.Warn().Str("target", m.name).Msg("component stop timed out")
	}
}

// stopAll stops every component in reverse start order.
func (s *Supervisor) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.managed) - 1; i >= 0; i-- {
		s.stop(s.managed[i])
	}
	s.publish(s.clock.Now())
}

// reporter is the push channel of one component instance; reports from an old
// generation (a restarted instance) are ignored.
type reporter struct {
	s   *Supervisor
	m   *managed
	gen int
}

// Report records a state transition and wakes the supervisor loop. It never takes the
// supervisor lock, so a component may report while it is being stopped.
func (r *reporter) Report(st State, reason string) {
	r.m.report(r.gen, st, reason)
	select {
	case r.s.wake <- struct{}{}:
	default:
	}
}

// publish stores a fresh snapshot. Caller holds mu.
func (s *Supervisor) publish(now time.Time) {
	snap := Snapshot{Status: StatusOK, Components: make(map[string]ComponentStatus), At: now}
	for _, m := range s.managed {
		cs := m.status()
		snap.Components[m.name] = cs
		s.mergeStatus(&snap, m.name, cs, m.latched)
	}
	for _, w := range s.watched {
		cs := ComponentStatus{State: w.state.String(), Reason: w.reason}
		snap.Components[w.name] = cs
		s.mergeStatus(&snap, w.name, cs, w.state == Failed)
	}
	s.snap.Store(&snap)
}

// mergeStatus folds one component into the overall status: a latched restartable or a
// failed non-restartable part fails the whole app; anything not healthy degrades it.
func (s *Supervisor) mergeStatus(snap *Snapshot, name string, cs ComponentStatus, fatal bool) {
	switch {
	case fatal:
		if snap.Status != StatusFailed {
			snap.Status, snap.Component, snap.Reason = StatusFailed, name, cs.Reason
		}
	case cs.State != Healthy.String() && snap.Status == StatusOK:
		snap.Status = StatusDegraded
	}
}
