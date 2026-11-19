// Package aggregation holds the pure period math shared by the live aggregator and the
// CLI backfill: daily sums and the frozen baseline go in, monthly/yearly/total/net
// values come out. No I/O, no clock, no logging.
package aggregation

import (
	"maps"
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

// LevelValues computes one period of a level from its daily source sums: the edge
// targets (plus offsets[target], e.g. the cutover offsets; nil = none) and the net values
// from them. The live aggregator and the backfill share it, so both write the same rows.
func LevelValues(edges []solis.Edge, pairs []solis.NetPair, sums, offsets Values) Values {
	vals := AddOffsets(ApplyEdges(edges, sums), offsets)
	return Merge(vals, ApplyNet(pairs, vals))
}

// Fold adds a closed year's per-total-key sums to the baseline and returns the new one.
func Fold(baseline, add Values) Values {
	out := make(Values, len(baseline)+len(add))
	maps.Copy(out, baseline)
	for k, v := range add {
		out[k] += v
	}
	return out
}

// AddOffsets returns values with offsets[k] added to every key present in values (keys
// without a value are not created). Used for the cutover month/year, which continue
// from the inverter-reported value.
func AddOffsets(values, offsets Values) Values {
	out := make(Values, len(values))
	for k, v := range values {
		out[k] = v + offsets[k]
	}
	return out
}

// Merge combines value maps; later maps win on duplicate keys.
func Merge(vs ...Values) Values {
	out := make(Values)
	for _, m := range vs {
		maps.Copy(out, m)
	}
	return out
}
