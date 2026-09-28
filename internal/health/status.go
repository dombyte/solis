package health

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/dombyte/solis/internal/util"
)

// Status is the reusable half of the Component contract: an atomic heartbeat and a
// state that is pushed to the Reporter only when it changes.
type Status struct {
	rep   Reporter
	clock util.Clock
	beat  atomic.Int64 // unix nanos, 0 = never
	mu    sync.Mutex
	state State
}

// NewStatus returns a healthy status reporting to rep.
func NewStatus(rep Reporter, clock util.Clock) *Status {
	return &Status{rep: rep, clock: clock}
}

// Beat records a heartbeat now (cheap atomic write).
func (s *Status) Beat() {
	s.beat.Store(s.clock.Now().UnixNano())
}

// Set changes the state and reports the transition (no report if unchanged).
func (s *Status) Set(st State, reason string) {
	s.mu.Lock()
	changed := s.state != st
	s.state = st
	s.mu.Unlock()
	if changed {
		s.rep.Report(st, reason)
	}
}

// State returns the current state.
func (s *Status) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// LastBeat returns the last heartbeat (zero if none).
func (s *Status) LastBeat() time.Time {
	n := s.beat.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}
