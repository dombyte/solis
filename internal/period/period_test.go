package period

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	return loc
}

func TestOf_KeyFormats(t *testing.T) {
	loc := berlin(t)
	tests := []struct {
		name             string
		at               time.Time
		day, month, year string
	}{
		{"midday", time.Date(2026, 8, 5, 12, 0, 0, 0, loc), "2026-08-05", "2026-08", "2026"},
		{
			"just before midnight", time.Date(2026, 12, 31, 23, 59, 59, 0, loc),
			"2026-12-31", "2026-12", "2026",
		},
		{"midnight", time.Date(2027, 1, 1, 0, 0, 0, 0, loc), "2027-01-01", "2027-01", "2027"},
		{
			"spring forward gap normalises", time.Date(2026, 3, 29, 2, 30, 0, 0, loc),
			"2026-03-29", "2026-03", "2026",
		},
		{
			"fall back repeated hour", time.Date(2026, 10, 25, 2, 30, 0, 0, loc),
			"2026-10-25", "2026-10", "2026",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Of(tt.at)
			assert.Equal(t, tt.day, p.Day)
			assert.Equal(t, tt.month, p.Month)
			assert.Equal(t, tt.year, p.Year)
			assert.Equal(t, tt.day, p.Key(Daily))
			assert.Equal(t, tt.month, p.Key(Monthly))
			assert.Equal(t, tt.year, p.Key(Yearly))
			assert.Empty(t, p.Key(Total))
		})
	}
}

func TestLevel_String(t *testing.T) {
	assert.Equal(t, "daily", Daily.String())
	assert.Equal(t, "monthly", Monthly.String())
	assert.Equal(t, "yearly", Yearly.String())
	assert.Equal(t, "total", Total.String())
	assert.Equal(t, "unknown", Level(42).String())
}

func TestKeyArithmetic(t *testing.T) {
	d, err := AddDays("2026-02-28", 1)
	require.NoError(t, err)
	assert.Equal(t, "2026-03-01", d)

	first, last, err := MonthBounds("2028-02")
	require.NoError(t, err)
	assert.Equal(t, "2028-02-01", first)
	assert.Equal(t, "2028-02-29", last)

	first, last, err = YearBounds("2026")
	require.NoError(t, err)
	assert.Equal(t, "2026-01-01", first)
	assert.Equal(t, "2026-12-31", last)

	m, err := AddMonths("2026-12", 1)
	require.NoError(t, err)
	assert.Equal(t, "2027-01", m)

	y, err := AddYears("2026", -1)
	require.NoError(t, err)
	assert.Equal(t, "2025", y)
}

func TestKeyArithmetic_InvalidKeys(t *testing.T) {
	_, err := AddDays("2026-13-01", 1)
	assert.ErrorIs(t, err, ErrInvalidKey)
	_, _, err = MonthBounds("2026")
	assert.ErrorIs(t, err, ErrInvalidKey)
	_, _, err = YearBounds("26")
	assert.ErrorIs(t, err, ErrInvalidKey)
	_, err = AddMonths("x", 1)
	assert.ErrorIs(t, err, ErrInvalidKey)
	_, err = AddYears("x", 1)
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestMonthsAndYearsBetween(t *testing.T) {
	months, err := MonthsBetween("2026-10", "2027-02")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-11", "2026-12", "2027-01", "2027-02"}, months)

	months, err = MonthsBetween("", "2026-05")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-05"}, months)

	months, err = MonthsBetween("2026-05", "2026-05")
	require.NoError(t, err)
	assert.Empty(t, months)

	years, err := YearsBetween("2024", "2026")
	require.NoError(t, err)
	assert.Equal(t, []string{"2025", "2026"}, years)

	_, err = MonthsBetween("bad", "2026-05")
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestParseRollover(t *testing.T) {
	valid := map[string]string{
		"23:59": "23:59", "00:00": "00:00", "09:05": "09:05",
		"20:30": "20:30",
	}
	for in, want := range valid {
		r, err := ParseRollover(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, r.String())
	}
	invalid := []string{
		"11:59 PM", "24:00", "9:05", "23:60", "2359", "", "ab:cd", "23-59",
		" 23:59", "23:5x",
	}
	for _, in := range invalid {
		_, err := ParseRollover(in)
		assert.ErrorIs(t, err, ErrInvalidRollover, in)
	}
}

func TestWindowAt_LateRollover(t *testing.T) {
	loc := berlin(t)
	r, err := ParseRollover("23:59")
	require.NoError(t, err)

	tests := []struct {
		name     string
		at       time.Time
		inWindow bool
		closing  string
		opening  string
	}{
		{"before window", time.Date(2026, 8, 5, 22, 58, 0, 0, loc), false, "", ""},
		{
			"window start", time.Date(2026, 8, 5, 22, 59, 0, 0, loc), true,
			"2026-08-05", "2026-08-06",
		},
		{
			"just after midnight", time.Date(2026, 8, 6, 0, 5, 0, 0, loc), true,
			"2026-08-05", "2026-08-06",
		},
		{"window end exclusive", time.Date(2026, 8, 6, 0, 59, 0, 0, loc), false, "", ""},
		{
			"year boundary", time.Date(2027, 1, 1, 0, 30, 0, 0, loc), true,
			"2026-12-31", "2027-01-01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, ok := r.WindowAt(tt.at)
			require.Equal(t, tt.inWindow, ok)
			if ok {
				assert.Equal(t, tt.closing, w.Closing)
				assert.Equal(t, tt.opening, w.Opening)
			}
		})
	}
}

func TestWindowAt_EarlyRollover(t *testing.T) {
	loc := berlin(t)
	r, err := ParseRollover("00:30")
	require.NoError(t, err)

	w, ok := r.WindowAt(time.Date(2026, 8, 5, 23, 45, 0, 0, loc))
	require.True(t, ok)
	assert.Equal(t, "2026-08-05", w.Closing)
	assert.Equal(t, "2026-08-06", w.Opening)
	assert.Equal(t, time.Date(2026, 8, 6, 1, 30, 0, 0, loc), w.End)
}

func TestWindow_DSTNights(t *testing.T) {
	loc := berlin(t)
	r, err := ParseRollover("23:59")
	require.NoError(t, err)

	// Spring forward (2026-03-29 02:00 -> 03:00) happens after this window ends.
	w, ok := r.WindowAt(time.Date(2026, 3, 29, 0, 30, 0, 0, loc))
	require.True(t, ok)
	assert.Equal(t, "2026-03-28", w.Closing)
	assert.Equal(t, "2026-03-29", w.Opening)
	assert.Equal(t, 2*WindowHalfWidth, w.End.Sub(w.Start))

	// A rollover inside the skipped hour normalises forward instead of failing.
	r2, err := ParseRollover("02:30")
	require.NoError(t, err)
	w2, ok := r2.WindowAt(time.Date(2026, 3, 29, 3, 10, 0, 0, loc))
	require.True(t, ok)
	assert.Equal(t, "2026-03-29", w2.Opening)

	// Fall back (2026-10-25 03:00 -> 02:00): window is still two real hours wide.
	w3, ok := r2.WindowAt(time.Date(2026, 10, 25, 2, 45, 0, 0, loc))
	require.True(t, ok)
	assert.Equal(t, "2026-10-24", w3.Closing)
	assert.Equal(t, 2*WindowHalfWidth, w3.End.Sub(w3.Start))
}

func TestLastEnded(t *testing.T) {
	loc := berlin(t)
	r, err := ParseRollover("23:59")
	require.NoError(t, err)

	// Mid-day: last window ended this morning at 00:59 and opened today.
	w := r.LastEnded(time.Date(2026, 8, 6, 12, 0, 0, 0, loc))
	assert.Equal(t, "2026-08-06", w.Opening)

	// Inside tonight's window: the last ended window is last night's.
	w = r.LastEnded(time.Date(2026, 8, 7, 0, 10, 0, 0, loc))
	assert.Equal(t, "2026-08-06", w.Opening)

	// Exactly at the end the window counts as ended.
	w = r.LastEnded(time.Date(2026, 8, 7, 0, 59, 0, 0, loc))
	assert.Equal(t, "2026-08-07", w.Opening)

	r2, err := ParseRollover("02:00")
	require.NoError(t, err)
	// 00:30 with a 02:00 rollover: today has not opened yet.
	w = r2.LastEnded(time.Date(2026, 8, 7, 0, 30, 0, 0, loc))
	assert.Equal(t, "2026-08-06", w.Opening)
}
