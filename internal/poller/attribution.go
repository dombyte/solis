package poller

import (
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// ResetThreshold separates a counter reset from a glitch dip: inside the rollover window a
// value below 10 % of the previous one confirms the reset (spec §7.1, initial constant).
const ResetThreshold = 0.10

// Decision is the outcome of attributing one daily value.
type Decision int

const (
	// WriteCurrent is a normal max-write into the key's open day.
	WriteCurrent Decision = iota
	// WriteNewDay confirms a reset: the key advanced to the window's new day.
	WriteNewDay
	// Discard ignores the value (glitch dip, mid-day decrease, second reset).
	Discard
)

// Attribution is the result for one key and value.
type Attribution struct {
	// Decision is what to do with the value.
	Decision Decision
	// Day is the day key to write (Write* decisions).
	Day string
	// Closed is the day this decision closed ("" if none).
	Closed string
	// Warn explains a discarded decrease worth a warning ("" for silent dips).
	Warn string
}

// inWindow reports whether now lies inside a rollover window (pure rule).
func inWindow(r period.Rollover, now time.Time) (period.Window, bool) {
	return r.WindowAt(now)
}

// isReset reports whether cur is a counter reset relative to prev (pure rule).
func isReset(prev, cur float64) bool {
	return cur < ResetThreshold*prev
}

// keyState is the attribution state of one daily key.
type keyState struct {
	openDay string
	base    float64 // last accepted value (comparison base)
	hasBase bool
}

// DayAttributor owns day attribution and rollover detection for all daily keys. Each key
// is tracked independently, except that the inverter resets all counters together: once
// one key confirmed the reset in a window, keys that cannot see it themselves (no base,
// or a base of 0, so no decrease) follow it. The forced close at window expiry applies
// to all keys. It is not safe for concurrent use (the poll loop is its only user).
type DayAttributor struct {
	roll      period.Rollover
	keys      map[string]*keyState
	confirmed string // Opening day of the latest window with a confirmed reset
}

// NewDayAttributor creates an attributor for the given daily keys.
func NewDayAttributor(roll period.Rollover, keys []string) *DayAttributor {
	d := &DayAttributor{roll: roll, keys: make(map[string]*keyState, len(keys))}
	for _, k := range keys {
		d.keys[k] = &keyState{}
	}
	return d
}

// SeedDays lists the day rows needed to seed at now: the expected open day and, inside
// a window, the window's new day.
func (d *DayAttributor) SeedDays(now time.Time) []string {
	days := []string{d.roll.LastEnded(now).Opening}
	if w, ok := inWindow(d.roll, now); ok && w.Opening != days[0] {
		days = append(days, w.Opening)
	}
	return days
}

// Seed initialises every key from stored rows (day -> key -> value) and the persisted
// closed-day watermarks, so a restart inside the window compares correctly (spec §7.2).
// It returns the closes missed while the app was down: per key, the day before the
// expected open day when the key's watermark is older.
func (d *DayAttributor) Seed(now time.Time, rows map[string]map[string]float64,
	closed map[string]string,
) (map[string]string, error) {
	expected := d.roll.LastEnded(now).Opening
	closedThrough, err := period.AddDays(expected, -1)
	if err != nil {
		return nil, err
	}
	missed := make(map[string]string)
	for k, st := range d.keys {
		d.seedKey(k, st, now, rows, closed[k])
		if c := closed[k]; c != "" && c < closedThrough {
			missed[k] = closedThrough
		}
	}
	return missed, nil
}

// seedKey restores one key: its base on the expected day, or, inside a window, the new
// day when a stored row or its watermark shows it already advanced (which also means
// the reset was confirmed in this window).
func (d *DayAttributor) seedKey(k string, st *keyState, now time.Time,
	rows map[string]map[string]float64, closed string,
) {
	expected := d.roll.LastEnded(now).Opening
	*st = keyState{openDay: expected}
	if v, ok := rows[expected][k]; ok {
		st.base, st.hasBase = v, true
	}
	w, in := inWindow(d.roll, now)
	_, advanced := rows[w.Opening][k]
	if in && (advanced || closed >= expected) {
		st.openDay = w.Opening
		st.base, st.hasBase = rows[w.Opening][k], advanced
		d.confirmed = w.Opening
	}
}

// ForceClose closes, for every key still behind the day it must have reached by now
// (window expired without a confirmed reset, or the app was down), all days up to the
// expected one. It returns the per-key closed-through days.
func (d *DayAttributor) ForceClose(now time.Time) (map[string]string, error) {
	expected := d.roll.LastEnded(now).Opening
	closedThrough, err := period.AddDays(expected, -1)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for k, st := range d.keys {
		if st.openDay == "" || st.openDay >= expected {
			continue
		}
		out[k] = closedThrough
		*st = keyState{openDay: expected}
	}
	return out, nil
}

// Attribute decides where value v of key belongs at now. Call ForceClose first in each
// poll so the key's open day is never behind the expected day.
func (d *DayAttributor) Attribute(key string, v float64, now time.Time) Attribution {
	st, ok := d.keys[key]
	if !ok {
		return Attribution{Decision: Discard, Warn: "unknown daily key " + key}
	}
	w, in := inWindow(d.roll, now)
	if a, blind := d.blindReset(st, v, w, in, now); blind {
		return a
	}
	if !st.hasBase || v >= st.base { // value fits: normal max-write
		st.base, st.hasBase = v, true
		return Attribution{Decision: WriteCurrent, Day: st.openDay}
	}
	if in {
		return d.decreaseInWindow(key, st, v, w)
	}
	return Attribution{Decision: Discard, Warn: fmt.Sprintf(
		"%s decreased %.2f -> %.2f outside the rollover window; ignored until it "+
			"exceeds the stored max", key, st.base, v)}
}

// blindReset handles a key on the closing day that cannot see the reset itself: with no
// base or a base of 0 the counter never decreases. Such a key follows a reset another
// key confirmed in this window; without a confirmation its values are held (discarded
// silently) from the opening day's midnight on, so post-reset energy is never written
// to the closing day and counted again on the opening day (review ACQ-H1). It reports
// false when the normal rules apply (the key can detect its reset, or it is still
// before midnight without a confirmation).
func (d *DayAttributor) blindReset(st *keyState, v float64, w period.Window, in bool,
	now time.Time,
) (Attribution, bool) {
	switch {
	case !in || st.openDay != w.Closing || (st.hasBase && st.base > 0):
		return Attribution{}, false
	case d.confirmed == w.Opening:
		st.openDay, st.base, st.hasBase = w.Opening, v, true
		return Attribution{Decision: WriteNewDay, Day: w.Opening, Closed: w.Closing}, true
	case now.Before(w.Midnight):
		return Attribution{}, false
	default:
		return Attribution{Decision: Discard}, true
	}
}

// decreaseInWindow handles a decrease inside window w: a substantial decrease on the
// closing day, or any decrease once another key confirmed the reset in this window,
// confirms the reset (one advance per window); anything else is discarded.
func (d *DayAttributor) decreaseInWindow(key string, k *keyState, v float64,
	w period.Window,
) Attribution {
	if k.openDay != w.Closing {
		return Attribution{Decision: Discard, Warn: fmt.Sprintf(
			"%s decreased %.2f -> %.2f after its reset in this window; ignored", key, k.base, v)}
	}
	if d.confirmed != w.Opening && !isReset(k.base, v) {
		return Attribution{Decision: Discard} // glitch dip inside the window
	}
	d.confirmed = w.Opening
	k.openDay, k.base = w.Opening, v
	return Attribution{Decision: WriteNewDay, Day: w.Opening, Closed: w.Closing}
}

// OpenDay returns the key's open day (tests, diagnostics).
func (d *DayAttributor) OpenDay(key string) string {
	if st, ok := d.keys[key]; ok {
		return st.openDay
	}
	return ""
}
