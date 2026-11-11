package poller

import (
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/period"
)

// ResetThreshold separates a counter reset from a glitch dip: inside the rollover window a
// value below 10 % of the previous one confirms the reset (initial constant).
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

// isReset reports whether cur is a counter reset relative to prev (pure rule).
func isReset(prev, cur float64) bool {
	return cur < ResetThreshold*prev
}

// keyState is the attribution state of one daily key.
type keyState struct {
	openDay string
	base    float64 // last accepted value (comparison base)
	hasBase bool
	forced  bool // the open day was reached by a forced close, not a seen reset
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
	if w, ok := d.roll.WindowAt(now); ok && w.Opening != days[0] {
		days = append(days, w.Opening)
	}
	return days
}

// Seed initialises every key from stored rows (day -> key -> value) and the persisted
// closed-day watermarks, so a restart inside the window compares correctly.
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
	w, in := d.roll.WindowAt(now)
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
		*st = keyState{openDay: expected, forced: true}
	}
	return out, nil
}

// Confirm is the first pass of a poll: it records a reset that any key of values (daily
// key -> decoded value) shows in the current window, before a single value is
// attributed. The inverter resets all counters together, so the keys of one poll must
// agree on the reset whatever order they are attributed in.
func (d *DayAttributor) Confirm(values map[string]float64, now time.Time) {
	w, in := d.roll.WindowAt(now)
	if !in {
		return
	}
	for k, v := range values {
		if st, ok := d.keys[k]; ok && st.showsReset(w.Closing, v) {
			d.confirmed = w.Opening
			return
		}
	}
}

// Attribute decides where value v of key belongs at now. Call ForceClose and Confirm
// first in each poll so the key's open day is never behind the expected day and every
// key sees a reset confirmed in the same poll.
func (d *DayAttributor) Attribute(key string, v float64, now time.Time) Attribution {
	st, ok := d.keys[key]
	if !ok {
		return Attribution{Decision: Discard, Warn: "unknown daily key " + key}
	}
	w, in := d.roll.WindowAt(now)
	if a, followed := d.followReset(st, v, w, in, now); followed {
		return a
	}
	if !st.hasBase || v >= st.base { // value fits: normal max-write
		st.base, st.hasBase = v, true
		return Attribution{Decision: WriteCurrent, Day: st.openDay}
	}
	if in {
		return d.decreaseInWindow(key, st, v, w)
	}
	return st.decreaseOutsideWindow(key, v)
}

// followReset handles a key on the closing day whose own value does not show the reset.
//
// A blind key (no base, or a base of 0, so the counter never decreases) follows a reset
// another key confirmed in this window; without a confirmation its values are held
// (discarded silently) from the opening day's midnight on, so post-reset energy is never
// written to the closing day and counted again on the opening day (review ACQ-H1).
//
// A key with a base that has not decreased is held as well once the reset is confirmed
// and midnight has passed: after an outage across midnight its counter can already be at
// or above yesterday's (small) value, and max-writing that into the closing day would
// count the energy twice. It is not moved, because a value read just before its own
// reset looks the same (the blocks of one poll are read one after another); its decrease
// or the forced close at the window end moves it.
//
// It reports false when the normal rules apply.
func (d *DayAttributor) followReset(st *keyState, v float64, w period.Window, in bool,
	now time.Time,
) (Attribution, bool) {
	if !in || st.openDay != w.Closing {
		return Attribution{}, false
	}
	confirmed := d.confirmed == w.Opening
	if confirmed && st.blind() {
		st.openDay, st.base, st.hasBase = w.Opening, v, true
		return Attribution{Decision: WriteNewDay, Day: w.Opening, Closed: w.Closing}, true
	}
	if !now.Before(w.Midnight) && st.held(v, confirmed) {
		return Attribution{Decision: Discard}, true
	}
	return Attribution{}, false
}

// blind reports whether the key cannot show its own reset: without a base, or with a
// base of 0, the counter never decreases.
func (k *keyState) blind() bool { return !k.hasBase || k.base == 0 }

// held reports whether value v is kept back after midnight: always for a blind key, and
// for any other key once the reset is confirmed and v has not decreased.
func (k *keyState) held(v float64, confirmed bool) bool {
	return k.blind() || (confirmed && v >= k.base)
}

// showsReset reports whether v is a reset of a key still on the closing day.
func (k *keyState) showsReset(closing string, v float64) bool {
	return k.openDay == closing && !k.blind() && isReset(k.base, v)
}

// decreaseOutsideWindow discards a decrease outside the window. A reset-sized drop on a
// day reached only by a forced close means the inverter reset after the window ended
// (review ACQ-M2): the day's row already holds yesterday's counter and cannot shrink, so
// this is reported once with the cause instead of the generic warning.
func (k *keyState) decreaseOutsideWindow(key string, v float64) Attribution {
	if k.forced && isReset(k.base, v) {
		k.forced = false // warn once per key and day
		return Attribution{Decision: Discard, Warn: fmt.Sprintf(
			"%s reset (%.2f -> %.2f) after its day was force-closed at the end of the "+
				"rollover window: the inverter clock or rollover.time is probably off; "+
				"%s's row holds yesterday's energy until the counter exceeds %.2f again "+
				"(fix with `solis backfill` after correcting the time)",
			key, k.base, v, k.openDay, k.base)}
	}
	return Attribution{Decision: Discard, Warn: fmt.Sprintf(
		"%s decreased %.2f -> %.2f outside the rollover window; ignored until it "+
			"exceeds the stored max", key, k.base, v)}
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
