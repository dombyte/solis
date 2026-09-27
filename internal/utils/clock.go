package utils

import "time"

// Clock abstracts time so loops that wait (poller, aggregator, supervisor, hub)
// can be driven by a fake clock in tests.
type Clock interface {
	// Now returns the current local time.
	Now() time.Time
	// NewTimer creates a one-shot timer firing after d.
	NewTimer(d time.Duration) Timer
	// NewTicker creates a ticker firing every d.
	NewTicker(d time.Duration) Ticker
}

// Timer is the subset of *time.Timer used by the application.
type Timer interface {
	// C returns the channel the timer fires on.
	C() <-chan time.Time
	// Stop prevents the timer from firing; see time.Timer.Stop.
	Stop() bool
	// Reset changes the timer to fire after d; see time.Timer.Reset.
	Reset(d time.Duration) bool
}

// Ticker is the subset of *time.Ticker used by the application.
type Ticker interface {
	// C returns the channel the ticker delivers ticks on.
	C() <-chan time.Time
	// Stop turns the ticker off.
	Stop()
	// Reset changes the ticker period to d.
	Reset(d time.Duration)
}

// RealClock is the production Clock backed by the time package.
type RealClock struct{}

// NewRealClock returns the wall clock.
func NewRealClock() RealClock { return RealClock{} }

// Now returns time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer wraps time.NewTimer.
func (RealClock) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

// NewTicker wraps time.NewTicker.
func (RealClock) NewTicker(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time        { return r.t.C }
func (r realTimer) Stop() bool                 { return r.t.Stop() }
func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }

type realTicker struct{ t *time.Ticker }

func (r realTicker) C() <-chan time.Time   { return r.t.C }
func (r realTicker) Stop()                 { r.t.Stop() }
func (r realTicker) Reset(d time.Duration) { r.t.Reset(d) }
