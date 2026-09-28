// Package eventbus is the non-blocking in-process event bus that connects the cache,
// poller, aggregator and WebSocket hub without them referencing each other. It is
// created once in the composition root and never recreated.
package eventbus

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned by Subscribe after Close.
var ErrClosed = errors.New("event bus closed")

// Kind is the event type.
type Kind int

const (
	// ValuesUpdated is published by the cache on every write.
	ValuesUpdated Kind = iota
	// PeriodClosed is published by the poller when a day closes.
	PeriodClosed
)

// String returns the kind name for logs.
func (k Kind) String() string {
	switch k {
	case ValuesUpdated:
		return "ValuesUpdated"
	case PeriodClosed:
		return "PeriodClosed"
	}
	return "unknown"
}

// Cache write domains carried in ValuesUpdated events.
const (
	// DomainPoller is the poller's key domain (addressed + derived registers).
	DomainPoller = "poller"
	// DomainAggregator is the aggregator's key domain (computed registers).
	DomainAggregator = "aggregator"
)

// Event is one bus message.
type Event struct {
	// Kind is the event type.
	Kind Kind
	// Domain is the cache write domain (ValuesUpdated).
	Domain string
	// Keys are the written keys (ValuesUpdated).
	Keys []string
	// At is when the event happened.
	At time.Time
	// Day is the closed day key (PeriodClosed).
	Day string
}

// Policy decides what happens when a subscriber's buffer is full.
type Policy int

const (
	// Coalesce drops ValuesUpdated events on overflow: consumers read current state, so
	// an already-queued event covers the dropped one. PeriodClosed is never dropped.
	Coalesce Policy = iota
	// Lossless queues every event in an unbounded overflow list.
	Lossless
)

// Publisher publishes events; Publish never blocks.
type Publisher interface {
	Publish(e Event)
}

// Subscriber registers consumers.
type Subscriber interface {
	// Subscribe returns a channel of events and a cancel function that releases it.
	Subscribe(name string, buf int, p Policy) (<-chan Event, func(), error)
}

// Bus implements Publisher and Subscriber.
type Bus struct {
	mu      sync.RWMutex
	subs    map[*subscription]struct{}
	closed  bool
	dropped atomic.Int64
}

// New creates an empty bus.
func New() *Bus {
	return &Bus{subs: make(map[*subscription]struct{})}
}

type subscription struct {
	name     string
	policy   Policy
	ch       chan Event
	mu       sync.Mutex
	overflow []Event
	kick     chan struct{}
	done     chan struct{}
	once     sync.Once
}

// Subscribe registers a consumer with a buffered channel of size buf (min 1).
func (b *Bus) Subscribe(name string, buf int, p Policy) (<-chan Event, func(), error) {
	if buf < 1 {
		buf = 1
	}
	s := &subscription{
		name: name, policy: p, ch: make(chan Event, buf),
		kick: make(chan struct{}, 1), done: make(chan struct{}),
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, nil, ErrClosed
	}
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	go s.pump()
	return s.ch, func() { b.unsubscribe(s) }, nil
}

func (b *Bus) unsubscribe(s *subscription) {
	b.mu.Lock()
	delete(b.subs, s)
	b.mu.Unlock()
	s.once.Do(func() { close(s.done) })
}

// Publish delivers e to every subscriber without blocking.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for s := range b.subs {
		if !s.deliver(e) {
			b.dropped.Add(1)
		}
	}
}

// deliver tries a direct send, falling back to the overflow list; false = dropped.
func (s *subscription) deliver(e Event) bool {
	s.mu.Lock()
	queued := len(s.overflow) > 0
	s.mu.Unlock()
	if !queued {
		select {
		case s.ch <- e:
			return true
		default:
		}
	}
	if s.policy == Coalesce && e.Kind == ValuesUpdated {
		return false
	}
	s.mu.Lock()
	s.overflow = append(s.overflow, e)
	s.mu.Unlock()
	select {
	case s.kick <- struct{}{}:
	default:
	}
	return true
}

// pump moves overflow events into the channel, blocking only its own goroutine.
func (s *subscription) pump() {
	for {
		select {
		case <-s.done:
			return
		case <-s.kick:
		}
		for {
			s.mu.Lock()
			if len(s.overflow) == 0 {
				s.mu.Unlock()
				break
			}
			e := s.overflow[0]
			s.mu.Unlock()
			select {
			case s.ch <- e:
			case <-s.done:
				return
			}
			s.mu.Lock()
			s.overflow = s.overflow[1:]
			s.mu.Unlock()
		}
	}
}

// Dropped returns the number of coalesced (dropped) deliveries.
func (b *Bus) Dropped() int64 { return b.dropped.Load() }

// Closed reports whether Close was called (health probe).
func (b *Bus) Closed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.closed
}

// Close stops delivery and releases all subscriptions.
func (b *Bus) Close() {
	b.mu.Lock()
	b.closed = true
	subs := b.subs
	b.subs = make(map[*subscription]struct{})
	b.mu.Unlock()
	for s := range subs {
		s.once.Do(func() { close(s.done) })
	}
}
