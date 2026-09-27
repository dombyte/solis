package aggregation

import (
	"fmt"

	"github.com/dombyte/solis/internal/period"
)

// MaxNetDays bounds how many open net days one run recomputes after a long outage.
const MaxNetDays = 62

// Watermarks is the persisted close state a plan is derived from.
type Watermarks struct {
	// FrozenMonth is the last frozen month ("" = none).
	FrozenMonth string
	// FrozenYear is the last frozen year ("" = none).
	FrozenYear string
	// FrozenNetDay is the last frozen net daily day ("" = none).
	FrozenNetDay string
	// ClosedThrough is the last day every daily source key has closed ("" = none).
	ClosedThrough string
}

// Job is one period to recompute.
type Job struct {
	// Level is Daily (net only), Monthly or Yearly.
	Level period.Level
	// Key is the period key (day, month or year).
	Key string
	// From is the first day key summed.
	From string
	// To is the last day key summed (bounded by today for open periods).
	To string
	// Freeze marks the job final: every daily source has closed its last day.
	Freeze bool
}

// Plan lists the periods one run must recompute: every period after the frozen
// watermark up to today's. A period is frozen once all daily keys closed its last day,
// which makes a missed PeriodClosed event only delay (never skip) a freeze.
type Plan struct {
	// Days are net daily jobs.
	Days []Job
	// Months are monthly jobs (oldest first).
	Months []Job
	// Years are yearly jobs (oldest first).
	Years []Job
}

// BuildPlan derives the recompute plan for the run instant p.
func BuildPlan(p period.Period, w Watermarks) (Plan, error) {
	days, err := dayJobs(p, w)
	if err != nil {
		return Plan{}, err
	}
	months, err := periodJobs(p, w, period.Monthly, w.FrozenMonth)
	if err != nil {
		return Plan{}, err
	}
	years, err := periodJobs(p, w, period.Yearly, w.FrozenYear)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Days: days, Months: months, Years: years}, nil
}

func dayJobs(p period.Period, w Watermarks) ([]Job, error) {
	start := w.FrozenNetDay
	floor, err := period.AddDays(p.Day, -MaxNetDays)
	if err != nil {
		return nil, err
	}
	if start == "" || start < floor {
		start = floor
	}
	var jobs []Job
	day, err := period.AddDays(start, 1)
	for err == nil && day <= p.Day {
		jobs = append(jobs, Job{Level: period.Daily, Key: day, From: day, To: day,
			Freeze: w.ClosedThrough != "" && day <= w.ClosedThrough})
		day, err = period.AddDays(day, 1)
	}
	return jobs, err
}

func periodJobs(p period.Period, w Watermarks, l period.Level, frozen string) ([]Job, error) {
	var keys []string
	var err error
	switch l {
	case period.Monthly:
		keys, err = period.MonthsBetween(frozen, p.Month)
	case period.Yearly:
		keys, err = period.YearsBetween(frozen, p.Year)
	default:
		return nil, fmt.Errorf("aggregation: no period jobs for level %s", l)
	}
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(keys))
	for _, k := range keys {
		j, err := boundedJob(p, w, l, k)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func boundedJob(p period.Period, w Watermarks, l period.Level, key string) (Job, error) {
	var first, last string
	var err error
	if l == period.Monthly {
		first, last, err = period.MonthBounds(key)
	} else {
		first, last, err = period.YearBounds(key)
	}
	if err != nil {
		return Job{}, err
	}
	to := last
	if p.Day < to {
		to = p.Day
	}
	return Job{Level: l, Key: key, From: first, To: to,
		Freeze: w.ClosedThrough != "" && last <= w.ClosedThrough}, nil
}
