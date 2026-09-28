package maintenance

import (
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/dombyte/solis/internal/aggregation"
)

// unchangedEpsilon treats values equal at display precision (2 decimals) as unchanged.
const unchangedEpsilon = 0.005

// Line is one recomputed (key, period).
type Line struct {
	Level  string
	Key    string
	Period string
	Old    float64
	HadOld bool
	New    float64
	Unit   string
}

// String renders one report line, e.g.
// "monthly  pv_energy_monthly        2026-08   412.30 kWh -> 409.87 kWh".
func (l Line) String() string {
	old := "     n/a"
	if l.HadOld {
		old = fmt.Sprintf("%8.2f", l.Old)
	}
	return fmt.Sprintf("%-8s %-24s %-7s %s %s -> %6.2f %s", l.Level, l.Key, l.Period, old,
		l.Unit, l.New, l.Unit)
}

// Skip is one period left untouched because the daily history does not reach its start.
type Skip struct {
	Level      string
	Period     string
	FirstDaily string // oldest daily row ("" = none)
}

// String renders e.g. "skipped  yearly   2025     daily history starts 2025-06-01; kept".
func (s Skip) String() string {
	why := "no daily history"
	if s.FirstDaily != "" {
		why = "daily history starts " + s.FirstDaily
	}
	return fmt.Sprintf("skipped  %-8s %-7s  %s; stored value kept", s.Level, s.Period, why)
}

// Report collects the lines of one run.
type Report struct {
	Lines   []Line
	Skipped []Skip
}

func (r *Report) add(l Line) { r.Lines = append(r.Lines, l) }

func (r *Report) skip(s Skip) { r.Skipped = append(r.Skipped, s) }

// Counts returns recomputed, unchanged and lower (daily-data gap) row counts.
func (r Report) Counts() (recomputed, unchanged, lower int) {
	for _, l := range r.Lines {
		recomputed++
		switch {
		case l.HadOld && math.Abs(l.New-l.Old) < unchangedEpsilon:
			unchanged++
		case l.HadOld && l.New < l.Old:
			lower++
		}
	}
	return recomputed, unchanged, lower
}

// Write prints the gap warning, every line and the summary.
func (r Report) Write(w io.Writer) error {
	lines := []string{"note: periods with daily-data gaps come out lower than the " +
		"previously polled values"}
	for _, l := range r.Lines {
		lines = append(lines, l.String())
	}
	for _, s := range r.Skipped {
		lines = append(lines, s.String())
	}
	rec, same, lower := r.Counts()
	lines = append(lines, fmt.Sprintf("summary: %d rows recomputed, %d unchanged, %d lower "+
		"(daily-data gaps), %d periods skipped (incomplete daily history)", rec, same, lower,
		len(r.Skipped)))
	for _, s := range lines {
		if _, err := fmt.Fprintln(w, s); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(v aggregation.Values) []string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
