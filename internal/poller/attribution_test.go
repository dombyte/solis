package poller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	return loc
}

func rollover(t *testing.T, s string) period.Rollover {
	t.Helper()
	r, err := period.ParseRollover(s)
	require.NoError(t, err)
	return r
}

type step struct {
	at       string // "2006-01-02 15:04" in Europe/Berlin
	key      string
	value    float64
	decision Decision
	day      string
	closed   string
	warn     bool
}

func runSteps(t *testing.T, d *DayAttributor, loc *time.Location, steps []step) {
	t.Helper()
	for i, s := range steps {
		now, err := time.ParseInLocation("2006-01-02 15:04", s.at, loc)
		require.NoError(t, err)
		_, err = d.ForceClose(now)
		require.NoError(t, err)
		got := d.Attribute(s.key, s.value, now)
		assert.Equal(t, s.decision, got.Decision, "step %d (%s %s=%v)", i, s.at, s.key, s.value)
		if s.decision != Discard {
			assert.Equal(t, s.day, got.Day, "step %d day", i)
		}
		assert.Equal(t, s.closed, got.Closed, "step %d closed", i)
		assert.Equal(t, s.warn, got.Warn != "", "step %d warn: %q", i, got.Warn)
	}
}

func seeded(t *testing.T, roll string, keys []string, now time.Time,
	rows map[string]map[string]float64,
) *DayAttributor {
	t.Helper()
	d := NewDayAttributor(rollover(t, roll), keys)
	d.Seed(now, rows, nil)
	return d
}

func TestAttribute_MidnightResetPerKey(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv", "grid"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 20:00", "pv", 30, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 20:00", "grid", 5, WriteCurrent, "2026-08-05", "", false},
		// Value fits: max-write continues after the window opens and after midnight.
		{"2026-08-05 23:30", "pv", 30.5, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-06 00:01", "pv", 30.6, WriteCurrent, "2026-08-05", "", false},
		// pv resets: only pv's day closes.
		{"2026-08-06 00:05", "pv", 0.1, WriteNewDay, "2026-08-06", "2026-08-05", false},
		{"2026-08-06 00:05", "grid", 5.2, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-06 00:10", "pv", 0.2, WriteCurrent, "2026-08-06", "", false},
		// grid resets later inside the window.
		{"2026-08-06 00:20", "grid", 0, WriteNewDay, "2026-08-06", "2026-08-05", false},
	})
	assert.Equal(t, "2026-08-06", d.OpenDay("pv"))
}

func TestAttribute_SmallDipInsideWindowIsGlitch(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 23:40", "pv", 4123.8, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 23:45", "pv", 4119.2, Discard, "", "", false},
		{"2026-08-05 23:50", "pv", 4124.0, WriteCurrent, "2026-08-05", "", false},
	})
}

func TestAttribute_ForcedCloseAtWindowEnd(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"backup", "pv"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 22:00", "backup", 0, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 22:00", "pv", 12, WriteCurrent, "2026-08-05", "", false},
		// A zero counter cannot show its reset; without a confirmation it is held.
		{"2026-08-06 00:30", "backup", 0, Discard, "", "", false},
	})
	// Window ends at 00:59: every key still on the old day is force-closed.
	closes, err := d.ForceClose(time.Date(2026, 8, 6, 0, 59, 0, 0, loc))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"backup": "2026-08-05", "pv": "2026-08-05"}, closes)
	runSteps(t, d, loc, []step{
		{"2026-08-06 01:00", "backup", 0, WriteCurrent, "2026-08-06", "", false},
		{"2026-08-06 01:00", "pv", 12, WriteCurrent, "2026-08-06", "", false},
	})
	closes, _ = d.ForceClose(time.Date(2026, 8, 6, 1, 5, 0, 0, loc))
	assert.Empty(t, closes)
}

func TestAttribute_DecreaseOutsideWindowIgnoredUntilRecovered(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv"}, time.Date(2026, 8, 5, 10, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 12:00", "pv", 10, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 12:05", "pv", 0.2, Discard, "", "", true}, // mid-day reboot
		{"2026-08-05 12:10", "pv", 5, Discard, "", "", true},
		{"2026-08-05 13:00", "pv", 10.1, WriteCurrent, "2026-08-05", "", false},
	})
}

// The reproduction from the review (ACQ-H1): grid import is 0 all day, pv confirms the
// midnight reset, and grid's first post-reset energy must land on the new day only.
func TestAttribute_ZeroBaseKeyFollowsConfirmedReset(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv", "grid"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 22:00", "pv", 30, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 22:00", "grid", 0, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-06 00:01", "pv", 0, WriteNewDay, "2026-08-06", "2026-08-05", false},
		{"2026-08-06 00:50", "grid", 0.4, WriteNewDay, "2026-08-06", "2026-08-05", false},
		{"2026-08-06 01:00", "grid", 0.5, WriteCurrent, "2026-08-06", "", false},
	})
}

// Within one poll the keys are attributed in map order: a zero-base key seen before the
// confirming key is held for one poll instead of being written to the closing day.
func TestAttribute_ZeroBaseKeyBeforeConfirmationInSamePollIsHeld(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv", "grid"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 22:00", "pv", 30, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 22:00", "grid", 0, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 23:50", "grid", 0, WriteCurrent, "2026-08-05", "", false}, // before midnight
		{"2026-08-06 00:05", "grid", 0.1, Discard, "", "", false},
		{"2026-08-06 00:05", "pv", 0.1, WriteNewDay, "2026-08-06", "2026-08-05", false},
		{"2026-08-06 00:10", "grid", 0.2, WriteNewDay, "2026-08-06", "2026-08-05", false},
	})
}

// Without any confirmation the zero-base key is held until the forced close and then
// starts the new day with its full post-reset value: counted once, on the new day.
func TestAttribute_ZeroBaseKeyWithoutConfirmationWaitsForForcedClose(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"grid"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 22:00", "grid", 0, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-06 00:30", "grid", 0.3, Discard, "", "", false},
		{"2026-08-06 01:00", "grid", 0.5, WriteCurrent, "2026-08-06", "", false},
	})
}

// A small base cannot show a 90 % drop; once another key confirmed the reset, any
// decrease on the closing day is the reset.
func TestAttribute_SmallBaseFollowsConfirmedReset(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv", "grid"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 22:00", "pv", 30, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 22:00", "grid", 0.5, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 23:45", "grid", 0.4, Discard, "", "", false}, // unconfirmed: glitch
		{"2026-08-06 00:01", "pv", 0, WriteNewDay, "2026-08-06", "2026-08-05", false},
		{"2026-08-06 00:05", "grid", 0.1, WriteNewDay, "2026-08-06", "2026-08-05", false},
	})
}

func TestAttribute_DoubleResetAdvancesOnce(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 23:00", "pv", 20, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-06 00:02", "pv", 0.1, WriteNewDay, "2026-08-06", "2026-08-05", false},
		{"2026-08-06 00:04", "pv", 0.5, WriteCurrent, "2026-08-06", "", false},
		{"2026-08-06 00:06", "pv", 0, Discard, "", "", true}, // second reset: no new day
		{"2026-08-06 00:30", "pv", 0.6, WriteCurrent, "2026-08-06", "", false},
	})
}

func TestAttribute_PreMidnightResetUsesWindowNewDay(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 23:00", "pv", 20, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-05 23:30", "pv", 0.1, WriteNewDay, "2026-08-06", "2026-08-05", false},
	})
}

func TestAttribute_DSTNights(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	tests := []struct {
		name  string
		roll  string
		seed  time.Time
		steps []step
	}{
		{
			"spring forward, rollover 02:30 in the skipped hour", "02:30",
			time.Date(2026, 3, 28, 20, 0, 0, 0, loc),
			[]step{
				{"2026-03-29 00:30", "pv", 9, WriteCurrent, "2026-03-28", "", false},
				{"2026-03-29 03:15", "pv", 0.1, WriteNewDay, "2026-03-29", "2026-03-28", false},
			},
		},
		{
			"fall back night, rollover 23:59", "23:59",
			time.Date(2026, 10, 24, 20, 0, 0, 0, loc),
			[]step{
				{"2026-10-24 23:00", "pv", 9, WriteCurrent, "2026-10-24", "", false},
				{"2026-10-25 00:10", "pv", 0, WriteNewDay, "2026-10-25", "2026-10-24", false},
				{"2026-10-25 02:30", "pv", 0.5, WriteCurrent, "2026-10-25", "", false},
			},
		},
		{
			"fall back, rollover 02:30 in the repeated hour", "02:30",
			time.Date(2026, 10, 24, 20, 0, 0, 0, loc),
			[]step{
				{"2026-10-25 01:00", "pv", 9, WriteCurrent, "2026-10-24", "", false},
				{"2026-10-25 02:40", "pv", 0.1, WriteNewDay, "2026-10-25", "2026-10-24", false},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := seeded(t, tt.roll, []string{"pv"}, tt.seed, nil)
			runSteps(t, d, loc, tt.steps)
		})
	}
}

func TestSeed_ColdStartInsideWindow(t *testing.T) {
	loc := berlin(t)
	now := time.Date(2026, 8, 6, 0, 10, 0, 0, loc)
	d := NewDayAttributor(rollover(t, "23:59"), []string{"pv", "grid"})
	assert.Equal(t, []string{"2026-08-05", "2026-08-06"}, d.SeedDays(now))

	// pv already advanced before the restart, grid did not.
	d.Seed(now, map[string]map[string]float64{
		"2026-08-05": {"pv": 30, "grid": 5},
		"2026-08-06": {"pv": 0.3},
	}, map[string]string{"pv": "2026-08-05"})
	assert.Equal(t, "2026-08-06", d.OpenDay("pv"))
	assert.Equal(t, "2026-08-05", d.OpenDay("grid"))

	// First small grid value compares as a reset against yesterday's stored max.
	got := d.Attribute("grid", 0.1, now)
	assert.Equal(t, WriteNewDay, got.Decision)
	assert.Equal(t, "2026-08-05", got.Closed)
	// pv continues on the new day.
	assert.Equal(t, WriteCurrent, d.Attribute("pv", 0.4, now).Decision)
}

// Cold start inside the window with no stored row for the closing day (first install, or
// down all day): the first post-midnight value is held unless a stored new-day row
// already confirms the reset.
func TestSeed_ColdStartWithoutBase(t *testing.T) {
	loc := berlin(t)
	now := time.Date(2026, 8, 6, 0, 10, 0, 0, loc)
	d := NewDayAttributor(rollover(t, "23:59"), []string{"pv", "grid"})
	_, err := d.Seed(now, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, Discard, d.Attribute("grid", 0.1, now).Decision)

	confirmed := NewDayAttributor(rollover(t, "23:59"), []string{"pv", "grid"})
	_, err = confirmed.Seed(now, map[string]map[string]float64{"2026-08-06": {"pv": 0.2}}, nil)
	require.NoError(t, err)
	got := confirmed.Attribute("grid", 0.1, now)
	assert.Equal(t, WriteNewDay, got.Decision)
	assert.Equal(t, "2026-08-06", got.Day)
}

// A restart after an outage reports the days closed while the app was down (ACQ-M1).
func TestSeed_ReportsClosesMissedDuringOutage(t *testing.T) {
	loc := berlin(t)
	d := NewDayAttributor(rollover(t, "23:59"), []string{"pv", "grid", "fresh"})
	missed, err := d.Seed(time.Date(2026, 8, 6, 12, 0, 0, 0, loc), nil,
		map[string]string{"pv": "2026-08-02", "grid": "2026-08-05"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"pv": "2026-08-05"}, missed)

	// Inside the window the closing day is still open: only older days are missed.
	missed, err = d.Seed(time.Date(2026, 8, 6, 0, 10, 0, 0, loc), nil,
		map[string]string{"pv": "2026-08-04", "grid": "2026-08-03"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"grid": "2026-08-04"}, missed)
}

func TestSeed_OutsideWindowAndEmpty(t *testing.T) {
	loc := berlin(t)
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, loc)
	d := NewDayAttributor(rollover(t, "23:59"), []string{"pv"})
	assert.Equal(t, []string{"2026-08-06"}, d.SeedDays(now))
	d.Seed(now, map[string]map[string]float64{"2026-08-06": {"pv": 7}}, nil)
	assert.Equal(t, "2026-08-06", d.OpenDay("pv"))
	got := d.Attribute("pv", 6, now)
	assert.Equal(t, Discard, got.Decision)
	assert.NotEmpty(t, got.Warn)

	empty := NewDayAttributor(rollover(t, "23:59"), []string{"pv"})
	empty.Seed(now, nil, nil)
	assert.Equal(t, WriteCurrent, empty.Attribute("pv", 1, now).Decision)
	assert.Equal(t, Discard, empty.Attribute("zz", 1, now).Decision)
	assert.Empty(t, empty.OpenDay("zz"))
}

func TestForceClose_AfterOutage(t *testing.T) {
	loc := berlin(t)
	d := NewDayAttributor(rollover(t, "23:59"), []string{"pv"})
	d.Seed(time.Date(2026, 8, 1, 12, 0, 0, 0, loc), nil, nil)
	closes, err := d.ForceClose(time.Date(2026, 8, 6, 12, 0, 0, 0, loc))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"pv": "2026-08-05"}, closes)
	assert.Equal(t, "2026-08-06", d.OpenDay("pv"))
}

func TestPureRules(t *testing.T) {
	assert.True(t, isReset(100, 9.9))
	assert.False(t, isReset(100, 10))
	assert.False(t, isReset(0, 0))
	loc := berlin(t)
	_, ok := rollover(t, "23:59").WindowAt(time.Date(2026, 8, 5, 23, 0, 0, 0, loc))
	assert.True(t, ok)
	_, ok = rollover(t, "23:59").WindowAt(time.Date(2026, 8, 5, 12, 0, 0, 0, loc))
	assert.False(t, ok)
}

// An inverter whose clock is off resets after the window's forced close: the new day's
// row already holds yesterday's counter. This cannot be undone, but it is reported once
// with its cause instead of a generic "decreased" warning (review ACQ-M2).
func TestAttribute_LateResetAfterForcedCloseIsExplained(t *testing.T) {
	loc := berlin(t)
	d := seeded(t, "23:59", []string{"pv"}, time.Date(2026, 8, 5, 20, 0, 0, 0, loc), nil)
	runSteps(t, d, loc, []step{
		{"2026-08-05 22:00", "pv", 30, WriteCurrent, "2026-08-05", "", false},
		{"2026-08-06 01:00", "pv", 30, WriteCurrent, "2026-08-06", "", false}, // forced
	})
	now := time.Date(2026, 8, 6, 1, 10, 0, 0, loc)
	got := d.Attribute("pv", 0.1, now)
	assert.Equal(t, Discard, got.Decision)
	assert.Contains(t, got.Warn, "after its day was force-closed")
	got = d.Attribute("pv", 0.2, now.Add(time.Minute))
	assert.Contains(t, got.Warn, "outside the rollover window", "explained only once")
}
