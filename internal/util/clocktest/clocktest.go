// Package clocktest provides a manually advanced util.Clock for tests of loops that
// wait on timers (no wall-clock sleeps in tests).
package clocktest

import (
	"sort"
	"sync"
	"time"

	"github.com/dombyte/solis/internal/util"
)

// Clock is a fake util.Clock. Time only moves when Advance or Set is called; timers
// and tickers whose deadline is reached fire during that call.
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []*waiter
	added   chan struct{}
}

type waiter struct {
	at     time.Time
	period time.Duration // 0 = timer
	ch     chan time.Time
	active bool
}

// New returns a fake clock starting at start.
func New(start time.Time) *Clock {
	return &Clock{now: start, added: make(chan struct{}, 1)}
}

// Now returns the fake current time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer creates a fake timer.
func (c *Clock) NewTimer(d time.Duration) util.Timer {
	return &fakeTimer{c: c, w: c.add(d, 0)}
}

// NewTicker creates a fake ticker.
func (c *Clock) NewTicker(d time.Duration) util.Ticker {
	return &fakeTicker{c: c, w: c.add(d, d)}
}

func (c *Clock) add(d, period time.Duration) *waiter {
	c.mu.Lock()
	w := &waiter{at: c.now.Add(d), period: period, ch: make(chan time.Time, 1), active: true}
	c.waiters = append(c.waiters, w)
	if period == 0 && d <= 0 {
		c.fire(w) // like time.NewTimer(0): fires right away
	}
	c.mu.Unlock()
	select {
	case c.added <- struct{}{}:
	default:
	}
	return w
}

// Advance moves the clock forward by d, firing due timers in deadline order.
func (c *Clock) Advance(d time.Duration) {
	c.Set(c.Now().Add(d))
}

// Set moves the clock to t (never backwards), firing due timers in deadline order.
func (c *Clock) Set(t time.Time) {
	for {
		c.mu.Lock()
		w := c.nextDue(t)
		if w == nil {
			if t.After(c.now) {
				c.now = t
			}
			c.mu.Unlock()
			return
		}
		if w.at.After(c.now) {
			c.now = w.at
		}
		c.fire(w)
		c.mu.Unlock()
	}
}

// nextDue returns the active waiter with the earliest deadline <= t. Caller holds mu.
func (c *Clock) nextDue(t time.Time) *waiter {
	due := make([]*waiter, 0, len(c.waiters))
	for _, w := range c.waiters {
		if w.active && !w.at.After(t) {
			due = append(due, w)
		}
	}
	if len(due) == 0 {
		return nil
	}
	sort.SliceStable(due, func(i, j int) bool { return due[i].at.Before(due[j].at) })
	return due[0]
}

// fire delivers a tick without blocking. Caller holds mu.
func (c *Clock) fire(w *waiter) {
	select {
	case w.ch <- c.now:
	default:
	}
	if w.period > 0 {
		w.at = w.at.Add(w.period)
		return
	}
	w.active = false
}

// blockUntilTimeout bounds BlockUntil so a broken test fails instead of hanging.
const blockUntilTimeout = 2 * time.Second

// BlockUntil waits (real time, bounded) until at least n timers/tickers are active.
// It lets a test wait for a goroutine to reach its select before advancing the clock.
func (c *Clock) BlockUntil(n int) bool {
	deadline := time.After(blockUntilTimeout)
	for {
		if c.activeCount() >= n {
			return true
		}
		select {
		case <-c.added:
		case <-time.After(time.Millisecond):
		case <-deadline:
			return false
		}
	}
}

func (c *Clock) activeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, w := range c.waiters {
		if w.active {
			n++
		}
	}
	return n
}

type fakeTimer struct {
	c *Clock
	w *waiter
}

func (t *fakeTimer) C() <-chan time.Time { return t.w.ch }

// Stop and Reset discard a pending tick, matching time.Timer since Go 1.23.
func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.w.active
	t.w.active = false
	drain(t.w.ch)
	return was
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.c.mu.Lock()
	was := t.w.active
	t.w.at = t.c.now.Add(d)
	t.w.active = true
	drain(t.w.ch)
	if d <= 0 {
		t.c.fire(t.w)
	}
	t.c.mu.Unlock()
	select {
	case t.c.added <- struct{}{}:
	default:
	}
	return was
}

func drain(ch chan time.Time) {
	select {
	case <-ch:
	default:
	}
}

type fakeTicker struct {
	c *Clock
	w *waiter
}

func (t *fakeTicker) C() <-chan time.Time { return t.w.ch }

func (t *fakeTicker) Stop() {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	t.w.active = false
}

func (t *fakeTicker) Reset(d time.Duration) {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	t.w.period = d
	t.w.at = t.c.now.Add(d)
	t.w.active = true
}
