package report

// FigureKind says how a key figure is measured and compared.
type FigureKind int

const (
	// KindEnergy is kWh; changes in kWh and percent.
	KindEnergy FigureKind = iota
	// KindRatio is a share 0..1; changes in percentage points (as a fraction).
	KindRatio
)

// Direction says which change of a figure is an improvement.
type Direction int

const (
	// Neutral: neither direction is better (consumption, export, battery).
	Neutral Direction = iota
	// HigherIsBetter: an increase is an improvement.
	HigherIsBetter
	// LowerIsBetter: a decrease is an improvement.
	LowerIsBetter
)

// Figure is one key figure of the report.
type Figure struct {
	// ID names the figure (renderers map it to a label).
	ID     string
	Kind   FigureKind
	Better Direction
	// Value is the range's value over all its complete days; nil = no data.
	Value *float64
	// Compared and Base are the two values the change is computed from: the range's and
	// the comparison period's. With Comparison.Matched both cover only the matched days
	// (so Compared can differ from Value); otherwise Compared equals Value and Base is
	// the whole comparison period. Nil without a comparison or without data.
	Compared, Base *float64
	// Change = Compared - Base: kWh, or a fraction for ratios (x100 = percentage points).
	Change *float64
	// ChangePct is Change / base for energy figures (a fraction); nil when base is 0.
	ChangePct *float64
}

// figureDef defines one key figure.
type figureDef struct {
	id     string
	kind   FigureKind
	better Direction
	value  func(Summary) *float64
}

// figureDefs lists the key figures in display order.
func figureDefs() []figureDef {
	return []figureDef{
		{"pv", KindEnergy, HigherIsBetter, energy(func(e Energy) float64 { return e.PV })},
		{
			"consumption", KindEnergy, Neutral,
			energy(func(e Energy) float64 { return e.Consumption }),
		},
		{
			"self_sufficiency", KindRatio, HigherIsBetter,
			func(s Summary) *float64 { return s.SelfSufficiency },
		},
		{
			"self_consumption", KindRatio, HigherIsBetter,
			func(s Summary) *float64 { return s.SelfConsumption },
		},
		{
			"grid_import", KindEnergy, LowerIsBetter,
			energy(func(e Energy) float64 { return e.Import }),
		},
		{"grid_export", KindEnergy, Neutral, energy(func(e Energy) float64 { return e.Export })},
		{
			"battery_charge", KindEnergy, Neutral,
			energy(func(e Energy) float64 { return e.Charge }),
		},
		{
			"battery_discharge", KindEnergy, Neutral,
			energy(func(e Energy) float64 { return e.Discharge }),
		},
	}
}

// energy reads a total; nil without complete days (no data is not 0 kWh).
func energy(field func(Energy) float64) func(Summary) *float64 {
	return func(s Summary) *float64 { return withData(s, field(s.Total)) }
}

// withData returns &v, or nil when s has no complete days.
func withData(s Summary, v float64) *float64 {
	if s.CompleteDays == 0 {
		return nil
	}
	return &v
}

// figures builds the key figures of cur with the changes between the sides of cmp when
// it is not nil.
func figures(cur Summary, cmp *sides) []Figure {
	defs := figureDefs()
	out := make([]Figure, len(defs))
	for i, d := range defs {
		out[i] = d.build(cur, cmp)
	}
	return out
}

// build evaluates d for cur and, with a comparison, the compared values and their change.
func (d figureDef) build(cur Summary, cmp *sides) Figure {
	f := Figure{ID: d.id, Kind: d.kind, Better: d.better, Value: d.value(cur)}
	if cmp == nil {
		return f
	}
	f.Compared, f.Base = d.value(cmp.cur), d.value(cmp.base)
	if f.Compared == nil || f.Base == nil {
		return f
	}
	change := *f.Compared - *f.Base
	f.Change = &change
	if d.kind == KindEnergy {
		f.ChangePct = quotient(change, *f.Base)
	}
	return f
}

// String returns the API name of k: energy or ratio.
func (k FigureKind) String() string {
	if k == KindRatio {
		return "ratio"
	}
	return "energy"
}

// String returns the API name of d: higher, lower or neutral.
func (d Direction) String() string {
	switch d {
	case HigherIsBetter:
		return "higher"
	case LowerIsBetter:
		return "lower"
	default:
		return "neutral"
	}
}
