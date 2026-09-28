// Package health is the central supervisor: it creates restartable components through
// their factories, watches their self-reported state and heartbeats, restarts them within
// a consecutive-failure budget and escalates by cancelling the root context with
// ErrHealthFatal, which makes the process exit so the container runtime restarts it. It
// also publishes the snapshot served by /health.
package health

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Supervision constants (spec §8; constants, not config).
const (
	// RestartBudget is the number of consecutive restarts before a component latches failed.
	RestartBudget = 3
	// HealthyGraceFactor x poll interval: max silence of a healthy component.
	HealthyGraceFactor = 3
	// StartGraceFactor x poll interval: initial grace after (re)start (3x + 2x start allowance).
	StartGraceFactor = 5
	// RecoveringGraceFactor x poll interval: max silence of a recovering component.
	RecoveringGraceFactor = 10
	// MinSweepInterval is the floor of the staleness sweep (poll interval / 2).
	MinSweepInterval = 5 * time.Second
	// StopTimeout bounds each component Stop so a hung component cannot block shutdown;
	// a restart whose Stop times out escalates to ErrHealthFatal.
	StopTimeout = 5 * time.Second
	// StaleSnapshotFactor x sweep interval: max age of the published snapshot before
	// Snapshot reports the supervisor itself as failed.
	StaleSnapshotFactor = 3
)

// supervisorName is the component named in a snapshot failed for staleness.
const supervisorName = "health"

var (
	// ErrHealthFatal is the root-context cause when component restarts cannot recover the
	// app (budget exhausted, a restart's Stop timed out or a non-restartable part failed);
	// the process then exits non-zero and the container runtime restarts it.
	ErrHealthFatal = errors.New("health fatal")
	// ErrShutdown is the root-context cause of a clean shutdown (signal).
	ErrShutdown = errors.New("shutdown requested")
)

// ComponentError carries the failing component and reason of a fatal escalation.
type ComponentError struct {
	// Component is the component name.
	Component string
	// Reason is the human-readable failure reason.
	Reason string
	// Err is the wrapped sentinel (ErrHealthFatal).
	Err error
}

func (e *ComponentError) Error() string {
	return fmt.Sprintf("%v: %s: %s", e.Err, e.Component, e.Reason)
}

// Unwrap returns the wrapped sentinel.
func (e *ComponentError) Unwrap() error { return e.Err }

// State is a component's self-reported state.
type State int

const (
	// Healthy components beat within their grace.
	Healthy State = iota
	// Recovering components retry internally (e.g. Modbus reconnecting) and keep beating.
	Recovering
	// Failed components need a restart (or, when latched, a whole-app restart).
	Failed
)

// String returns the lower-case state name used in the /health body.
func (s State) String() string {
	switch s {
	case Healthy:
		return "healthy"
	case Recovering:
		return "recovering"
	default:
		return "failed"
	}
}

// Component is the supervision contract of every restartable component.
type Component interface {
	// Start begins the component's goroutines; it returns an error only for
	// unrecoverable conditions.
	Start(ctx context.Context) error
	// Stop is an idempotent graceful shutdown.
	Stop() error
	// State returns the current self-reported state.
	State() State
	// LastBeat returns the last heartbeat, written by the component's own loop.
	LastBeat() time.Time
}

// Reporter pushes rare, discrete state transitions to the supervisor.
type Reporter interface {
	Report(s State, reason string)
}

// Factory constructs a fresh component instance, handing it its Reporter.
type Factory func(Reporter) (Component, error)

// Probe returns the state of a non-restartable part (storage, cache, bus, HTTP server).
type Probe func() (State, string)

// ComponentStatus is one component's entry in the snapshot.
type ComponentStatus struct {
	// State is healthy, recovering or failed.
	State string `json:"state"`
	// Reason explains a non-healthy state.
	Reason string `json:"reason,omitempty"`
	// LastBeat is the last heartbeat (restartable components).
	LastBeat *time.Time `json:"last_beat,omitempty"`
	// Restarts is the consecutive-failure counter.
	Restarts int `json:"restarts"`
	// Restartable distinguishes managed components from watched parts.
	Restartable bool `json:"restartable"`
}

// Overall snapshot status values.
const (
	StatusOK       = "ok"
	StatusDegraded = "degraded"
	StatusFailed   = "failed"
)

// Snapshot is the aggregate health published for /health.
type Snapshot struct {
	// Status is ok, degraded or failed.
	Status string `json:"status"`
	// Component names the failed component (Status failed).
	Component string `json:"component,omitempty"`
	// Reason explains the failure (Status failed).
	Reason string `json:"reason,omitempty"`
	// Components lists every supervised part.
	Components map[string]ComponentStatus `json:"components"`
	// At is when the snapshot was taken.
	At time.Time `json:"timestamp"`
}
