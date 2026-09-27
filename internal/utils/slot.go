package utils

import "sync/atomic"

// Slot holds the current instance of a value that may be swapped at runtime,
// e.g. the Modbus client replaced by a supervisor restart while the poller keeps
// reading through the slot.
type Slot[T any] struct {
	p atomic.Pointer[T]
}

// Store publishes v as the current value.
func (s *Slot[T]) Store(v T) { s.p.Store(&v) }

// Clear removes the current value.
func (s *Slot[T]) Clear() { s.p.Store(nil) }

// Load returns the current value and whether one is set.
func (s *Slot[T]) Load() (T, bool) {
	p := s.p.Load()
	if p == nil {
		var zero T
		return zero, false
	}
	return *p, true
}
