package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, from, to string
		days           int
		ok             bool
	}{
		{"month", "2026-10-01", "2026-10-29", 29, true},
		{"single day", "2026-10-29", "2026-10-29", 1, true},
		{"over DST change", "2026-03-28", "2026-03-30", 3, true},
		{"max length", "2016-10-29", "2026-10-29", 3653, true},
		{"from after to", "2026-10-02", "2026-10-01", 0, false},
		{"future", "2026-10-01", "2026-10-30", 0, false},
		{"bad from", "2026-10", "2026-10-29", 0, false},
		{"bad to", "2026-10-01", "29.10.2026", 0, false},
		{"too long", "2016-01-01", "2026-10-29", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, err := ParseRange(tt.from, tt.to, "2026-10-29")
			if !tt.ok {
				require.ErrorIs(t, err, ErrInvalidRange)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.days, r.Days())
			assert.Equal(t, tt.from+".."+tt.to, r.String())
		})
	}
	_, err := ParseRange("2026-10-01", "2026-10-02", "")
	require.ErrorIs(t, err, ErrInvalidRange)
}

func TestRangeFinished(t *testing.T) {
	t.Parallel()
	today := day("2026-10-29")
	r, excluded, err := rng("2026-10-01", "2026-10-28").Finished(today)
	require.NoError(t, err)
	assert.False(t, excluded)
	assert.Equal(t, "2026-10-01..2026-10-28", r.String())

	r, excluded, err = rng("2026-10-01", "2026-10-29").Finished(today)
	require.NoError(t, err)
	assert.True(t, excluded)
	assert.Equal(t, "2026-10-01..2026-10-28", r.String(), "ends yesterday")

	_, _, err = rng("2026-10-29", "2026-10-29").Finished(today)
	require.ErrorIs(t, err, ErrInvalidRange, "today alone has no finished day")
}

func TestGranularity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from, to string
		want     Granularity
	}{
		{"2026-10-01", "2026-12-01", ByDay},   // 62 days
		{"2026-10-01", "2026-12-02", ByMonth}, // 63 days
		{"2024-10-01", "2026-09-30", ByMonth}, // 730 days
		{"2024-01-01", "2025-12-31", ByMonth}, // 731 days (leap year)
		{"2024-01-01", "2026-01-01", ByYear},  // 732 days
		{"2020-01-01", "2026-10-29", ByYear},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, AutoGranularity(rng(tt.from, tt.to)), tt.from+".."+tt.to)
	}
	r := rng("2026-10-01", "2026-10-31")
	for _, s := range []string{"day", "month", "year"} {
		g, err := ParseGranularity(s, r)
		require.NoError(t, err)
		assert.Equal(t, s, g.String())
	}
	g, err := ParseGranularity("", rng("2020-01-01", "2026-10-29"))
	require.NoError(t, err)
	assert.Equal(t, ByYear, g)
	_, err = ParseGranularity("week", r)
	require.ErrorIs(t, err, ErrInvalidRange)
}

func TestParseCompare(t *testing.T) {
	t.Parallel()
	m, err := ParseCompare("")
	require.NoError(t, err)
	assert.Equal(t, CompareLastYear, m)
	for _, s := range []string{"none", "previous", "last_year"} {
		m, err := ParseCompare(s)
		require.NoError(t, err)
		assert.Equal(t, s, m.String())
	}
	_, err = ParseCompare("yesterday")
	require.ErrorIs(t, err, ErrInvalidRange)
}

func TestCompareRanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mode     CompareMode
		from, to string
		want     string
	}{
		{"previous month", ComparePrevious, "2026-10-01", "2026-10-31", "2026-09-01..2026-09-30"},
		{"previous March", ComparePrevious, "2026-03-01", "2026-03-31", "2026-02-01..2026-02-28"},
		{"previous quarter", ComparePrevious, "2026-01-01", "2026-03-31", "2025-10-01..2025-12-31"},
		{"previous year", ComparePrevious, "2025-01-01", "2025-12-31", "2024-01-01..2024-12-31"},
		{"month to date", ComparePrevious, "2026-10-01", "2026-10-29", "2026-09-01..2026-09-29"},
		{
			"month to date clamps", ComparePrevious, "2026-03-01", "2026-03-30",
			"2026-02-01..2026-02-28",
		},
		{"year to date", ComparePrevious, "2026-01-01", "2026-10-29", "2025-01-01..2025-10-29"},
		{"months to date", ComparePrevious, "2026-04-01", "2026-06-15", "2026-01-01..2026-03-15"},
		{
			"to date over new year", ComparePrevious, "2025-11-01", "2026-01-10",
			"2025-08-01..2025-10-10",
		},
		{
			"first of january only", ComparePrevious, "2026-01-01", "2026-01-10",
			"2025-12-01..2025-12-10",
		},
		{"previous odd", ComparePrevious, "2026-03-15", "2026-06-10", "2025-12-17..2026-03-14"},
		{"previous days", ComparePrevious, "2026-10-02", "2026-10-29", "2026-09-04..2026-10-01"},
		{"last year", CompareLastYear, "2026-10-01", "2026-10-31", "2025-10-01..2025-10-31"},
		{"last year leap", CompareLastYear, "2028-02-01", "2028-02-29", "2027-02-01..2027-02-28"},
		{
			"last year from leap", CompareLastYear, "2029-02-28", "2029-03-01",
			"2028-02-28..2028-03-01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tt.mode.Of(rng(tt.from, tt.to))
			require.True(t, ok)
			assert.Equal(t, tt.want, got.String())
		})
	}
	_, ok := CompareNone.Of(rng("2026-10-01", "2026-10-31"))
	assert.False(t, ok)
}

func TestFigureNames(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"energy", "ratio"}, []string{KindEnergy.String(), KindRatio.String()})
	assert.Equal(t, []string{"neutral", "higher", "lower"},
		[]string{Neutral.String(), HigherIsBetter.String(), LowerIsBetter.String()})
}
