package storage

import (
	"errors"
	"fmt"
)

var (
	// ErrPeriodClosed is returned for writes into an immutable (closed) period.
	ErrPeriodClosed = errors.New("period closed")
	// ErrWriteDomain is returned when a writer targets a key outside its write domain.
	ErrWriteDomain = errors.New("write outside domain")
	// ErrUnknownKey is returned for keys that are not in the register table.
	ErrUnknownKey = errors.New("unknown register key")
)

// PeriodClosedError describes a rejected write into a closed period.
type PeriodClosedError struct {
	// Level is the period level ("daily", "monthly", ...).
	Level string
	// Key is the register key.
	Key string
	// Period is the rejected period key.
	Period string
	// ClosedThrough is the watermark the write violated.
	ClosedThrough string
}

func (e *PeriodClosedError) Error() string {
	return fmt.Sprintf("storage: %s %s %s is closed (closed through %s)",
		e.Level, e.Key, e.Period, e.ClosedThrough)
}

// Is makes errors.Is(err, ErrPeriodClosed) match.
func (e *PeriodClosedError) Is(target error) bool { return target == ErrPeriodClosed }

// WriteDomainError describes a write that violates the per-component write domains.
type WriteDomainError struct {
	// Writer is the rejected writer ("poller", "aggregator").
	Writer string
	// Key is the register key.
	Key string
	// Reason explains the violation.
	Reason string
}

func (e *WriteDomainError) Error() string {
	return fmt.Sprintf("storage: %s may not write %s: %s", e.Writer, e.Key, e.Reason)
}

// Is makes errors.Is(err, ErrWriteDomain) match.
func (e *WriteDomainError) Is(target error) bool { return target == ErrWriteDomain }
