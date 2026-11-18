package report

import (
	"time"
)

// Bucket is one group of days (a day, month or year) of the range.
type Bucket struct {
	// Key is YYYY-MM-DD, YYYY-MM or YYYY.
	Key string
	// From and To are the bucket's days inside the range.
	From, To time.Time
	// Partial is true when the range cuts the bucket (a month or year not fully inside).
	Partial bool
	Summary Summary
}

// buckets groups the range by g; buckets without data are included (CompleteDays 0).
func buckets(r Range, g Granularity, days []Day) []Bucket {
	var out []Bucket
	i := 0
	for day := r.From; !day.After(r.To); {
		first, last := g.bounds(day)
		from, to := latest(first, r.From), earliest(last, r.To)
		start := i
		for i < len(days) && !days[i].Date.After(to) {
			i++
		}
		b := Range{From: from, To: to}
		out = append(out, Bucket{
			Key: g.key(day), From: from, To: to,
			Partial: !from.Equal(first) || !to.Equal(last),
			Summary: summarize(days[start:i], b.Days()),
		})
		day = to.AddDate(0, 0, 1)
	}
	return out
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
