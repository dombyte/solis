package period

import (
	"errors"
	"fmt"
	"time"
)

// WindowHalfWidth is the tolerated drift between the configured rollover time and the
// inverter's actual counter reset (spec §7.1: rollover time ± 1 h).
const WindowHalfWidth = time.Hour

// noonHour splits rollover times into "closes today" (>= 12:00, e.g. 23:59) and
// "closes yesterday" (< 12:00, e.g. 00:30): the new day is the one whose midnight is
// nearest to the rollover instant.
const noonHour = 12

// ErrInvalidRollover is returned for anything that is not strict 24-hour HH:MM.
var ErrInvalidRollover = errors.New("invalid rollover time")

// Rollover is the configured daily rollover time of day.
type Rollover struct {
	hour   int
	minute int
}

// ParseRollover parses strict 24-hour "HH:MM" (e.g. "23:59"). "9:05", "24:00" and
// "11:59 PM" are rejected.
func ParseRollover(s string) (Rollover, error) {
	const (
		layoutLen = 5
		colonPos  = 2
		maxHour   = 23
		maxMinute = 59
	)
	if len(s) != layoutLen || s[colonPos] != ':' {
		return Rollover{}, invalidRollover(s)
	}
	h, okH := twoDigits(s[:colonPos])
	m, okM := twoDigits(s[colonPos+1:])
	if !okH || !okM || h > maxHour || m > maxMinute {
		return Rollover{}, invalidRollover(s)
	}
	return Rollover{hour: h, minute: m}, nil
}

func invalidRollover(s string) error {
	return fmt.Errorf("%w: %q (want strict 24-hour HH:MM)", ErrInvalidRollover, s)
}

// twoDigits converts exactly two ASCII digits to an int.
func twoDigits(s string) (int, bool) {
	const base = 10
	if s[0] < '0' || s[0] > '9' || s[1] < '0' || s[1] > '9' {
		return 0, false
	}
	return int(s[0]-'0')*base + int(s[1]-'0'), true
}

// String returns the HH:MM form.
func (r Rollover) String() string {
	return fmt.Sprintf("%02d:%02d", r.hour, r.minute)
}

// Window is one rollover window around a rollover instant.
type Window struct {
	// Anchor is the rollover instant.
	Anchor time.Time
	// Start is Anchor - WindowHalfWidth.
	Start time.Time
	// End is Anchor + WindowHalfWidth; at End the day is force-closed.
	End time.Time
	// Closing is the day key that closes in this window.
	Closing string
	// Opening is the day key that starts in this window.
	Opening string
}

// Contains reports whether t lies inside the window (inclusive start, exclusive end).
func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

// windowOn builds the window whose anchor falls on the calendar date of day in loc.
func (r Rollover) windowOn(day time.Time) Window {
	y, m, d := day.Date()
	anchor := time.Date(y, m, d, r.hour, r.minute, 0, 0, day.Location())
	opening := time.Date(y, m, d, 0, 0, 0, 0, day.Location())
	if r.hour >= noonHour {
		opening = opening.AddDate(0, 0, 1)
	}
	return Window{
		Anchor:  anchor,
		Start:   anchor.Add(-WindowHalfWidth),
		End:     anchor.Add(WindowHalfWidth),
		Closing: opening.AddDate(0, 0, -1).Format(DayLayout),
		Opening: opening.Format(DayLayout),
	}
}

// WindowAt returns the rollover window containing now, if any.
func (r Rollover) WindowAt(now time.Time) (Window, bool) {
	for _, off := range []int{-1, 0, 1} {
		w := r.windowOn(now.AddDate(0, 0, off))
		if w.Contains(now) {
			return w, true
		}
	}
	return Window{}, false
}

// LastEnded returns the most recent window whose End is at or before now. Its Opening
// is the day every daily key must have reached by now (force-close target).
func (r Rollover) LastEnded(now time.Time) Window {
	for off := 1; off >= -2; off-- {
		w := r.windowOn(now.AddDate(0, 0, off))
		if !w.End.After(now) {
			return w
		}
	}
	// Unreachable: a window ends at most ~49 h before any instant two days later.
	return r.windowOn(now.AddDate(0, 0, -2))
}
