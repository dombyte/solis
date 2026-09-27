// Package aggregation holds the pure period math shared by the live aggregator and the
// CLI backfill: daily sums and the frozen baseline go in, monthly/yearly/total/net
// values come out. No I/O, no clock, no logging.
package aggregation

import (
	"sort"

	"github.com/dombyte/solis/internal/solis"
)

// Values maps register keys to values.
type Values map[string]float64

// SourceKeys returns the distinct daily source keys of edges, sorted.
func SourceKeys(edges []solis.Edge) []string {
	seen := make(map[string]struct{}, len(edges))
	var out []string
	for _, e := range edges {
		if _, ok := seen[e.Source]; !ok {
			seen[e.Source] = struct{}{}
			out = append(out, e.Source)
		}
	}
	sort.Strings(out)
	return out
}

// ApplyEdges maps daily source sums onto edge targets. A missing source sum counts as 0
// (no daily rows in the period).
func ApplyEdges(edges []solis.Edge, sums Values) Values {
	out := make(Values, len(edges))
	for _, e := range edges {
		out[e.Target] = sums[e.Source]
	}
	return out
}

// ApplyNet computes Target = Export - Import for every pair whose export and import are
// both present in values.
func ApplyNet(pairs []solis.NetPair, values Values) Values {
	out := make(Values, len(pairs))
	for _, p := range pairs {
		exp, okE := values[p.Export]
		imp, okI := values[p.Import]
		if okE && okI {
			out[p.Target] = exp - imp
		}
	}
	return out
}

// Totals composes total = baseline(target) + sum of daily rows since the last closed
// year (since is keyed by daily source).
func Totals(edges []solis.Edge, baseline, since Values) Values {
	out := make(Values, len(edges))
	for _, e := range edges {
		out[e.Target] = baseline[e.Target] + since[e.Source]
	}
	return out
}

// Fold adds a closed year's per-total-key sums to the baseline and returns the new one.
func Fold(baseline, add Values) Values {
	out := make(Values, len(baseline)+len(add))
	for k, v := range baseline {
		out[k] = v
	}
	for k, v := range add {
		out[k] += v
	}
	return out
}

// Merge combines value maps; later maps win on duplicate keys.
func Merge(maps ...Values) Values {
	out := make(Values)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
