package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// managed is the supervisor's registry entry of one restartable component.
type managed struct {
	name    string
	factory Factory

	// Owned by the supervisor loop (guarded by Supervisor.mu).
	comp      Component
	cancel    context.CancelFunc
	startedAt time.Time
	failures  int
	latched   bool

	// Written by the component's reporter (guarded by mu).
	mu             sync.Mutex
	gen            int
	reportedFailed bool
	reason         string
}

// reset starts a new generation and returns it.
func (m *managed) reset(now time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gen++
	m.startedAt = now
	m.reportedFailed = false
	m.reason = ""
	return m.gen
}

func (m *managed) setReason(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reason = reason
}

// report records a transition of generation gen (stale generations are ignored).
func (m *managed) report(gen int, st State, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.gen {
		return
	}
	if st == Failed {
		m.reportedFailed = true
	}
	m.reason = reason
}

func (m *managed) reported() (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reportedFailed, m.reason
}

// state is the effective state: latched/failed factory/reported failure win over the
// component's own State().
func (m *managed) state() State {
	failed, _ := m.reported()
	if m.latched || m.comp == nil || failed {
		return Failed
	}
	return m.comp.State()
}

// evaluate applies the state model at now; it returns whether a restart is needed and
// why, and resets the failure counter once a restarted component proved healthy.
func (m *managed) evaluate(now time.Time, interval time.Duration) (bool, string) {
	if m.latched {
		return false, ""
	}
	_, reason := m.reported()
	st := m.state()
	if st == Failed {
		return true, reason
	}
	beat := m.comp.LastBeat()
	ref, grace := beat, graceFor(st, interval)
	if !beat.After(m.startedAt) {
		ref, grace = m.startedAt, StartGraceFactor*interval
	}
	if silent := now.Sub(ref); silent > grace {
		return true, fmt.Sprintf("no heartbeat for %s (grace %s)", silent, grace)
	}
	m.resetFailures(st, beat, now, interval)
	return false, ""
}

// resetFailures clears the consecutive-failure counter once a restarted component has
// beaten and stayed healthy for the healthy grace since its start.
func (m *managed) resetFailures(st State, beat, now time.Time, interval time.Duration) {
	if st == Healthy && m.failures > 0 && beat.After(m.startedAt) &&
		now.Sub(m.startedAt) >= HealthyGraceFactor*interval {
		m.failures = 0
	}
}

func graceFor(st State, interval time.Duration) time.Duration {
	if st == Recovering {
		return RecoveringGraceFactor * interval
	}
	return HealthyGraceFactor * interval
}

// status renders the entry for the snapshot.
func (m *managed) status() ComponentStatus {
	st := m.state()
	_, reason := m.reported()
	cs := ComponentStatus{State: st.String(), Restarts: m.failures, Restartable: true}
	if st != Healthy {
		cs.Reason = reason
	}
	if m.comp != nil {
		if b := m.comp.LastBeat(); !b.IsZero() {
			cs.LastBeat = &b
		}
	}
	return cs
}
