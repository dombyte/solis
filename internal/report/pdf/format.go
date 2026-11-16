package pdf

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dombyte/solis/internal/report"
	"github.com/dombyte/solis/internal/util"
)

const (
	percent     = 100
	noValue     = "–"
	minus       = "−"
	thousands   = 3
	oneDecimal  = 1
	dayFormat   = "2 Jan 2006"
	shortDay    = "2 Jan"
	monthFormat = "Jan 2006"
	// maxNamedBuckets caps the incomplete months or years a chart caption spells out.
	maxNamedBuckets = 3
)

// number formats v with one decimal and thousands separators ("4,512.3", "−3.0"). It
// rounds to the API's two decimals first, exactly like the Reports page formats the JSON
// values, so the PDF and the page never differ in the last digit.
func number(v float64) string {
	s := strconv.FormatFloat(math.Abs(util.RoundTo2DecimalPlaces(v)), 'f', oneDecimal, 64)
	if s == "0.0" {
		return s
	}
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%thousands == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String() + "." + frac
	if v < 0 {
		return minus + out
	}
	return out
}

// kwh formats an optional energy as "403.7 kWh"; "–" when missing.
func kwh(v *float64) string {
	if v == nil {
		return noValue
	}
	return number(*v) + " kWh"
}

// energyOf returns &v, or nil when s has no complete days (no data is not 0 kWh).
func energyOf(s report.Summary, v float64) *float64 {
	if s.CompleteDays == 0 {
		return nil
	}
	return &v
}

// share formats an optional 0..1 share as "76.2 %".
func share(v *float64) string {
	if v == nil {
		return noValue
	}
	return number(*v*percent) + " %"
}

// dayLabel is "1 Oct 2026".
func dayLabel(t time.Time) string { return t.Format(dayFormat) }

// weekdayLabel is "Sat 10 Oct".
func weekdayLabel(t time.Time) string { return t.Format("Mon " + shortDay) }

// rangeLabel is "1 Oct 2026 – 31 Oct 2026".
func rangeLabel(r report.Range) string {
	return dayLabel(r.From) + " – " + dayLabel(r.To)
}

// periodLabel names r briefly: "Oct 2026" for a whole month, "2026" for a whole year,
// otherwise the day range.
func periodLabel(r report.Range) string {
	whole := r.From.Day() == 1 && r.To.AddDate(0, 0, 1).Day() == 1
	switch {
	case !whole || r.From.Year() != r.To.Year():
		return rangeLabel(r)
	case r.From.Month() == time.January && r.To.Month() == time.December:
		return strconv.Itoa(r.From.Year())
	case r.From.Month() == r.To.Month():
		return r.From.Format(monthFormat)
	default:
		return rangeLabel(r)
	}
}

// columnLabel is the short period label for a table column ("Oct 2026", "2026"), or
// fallback when the period has no short name.
func columnLabel(r report.Range, fallback string) string {
	if l := periodLabel(r); l != rangeLabel(r) {
		return l
	}
	return fallback
}

// bucketTick labels a bucket on a chart axis.
func bucketTick(b report.Bucket, g report.Granularity, oneMonth bool) string {
	switch {
	case g == report.ByYear:
		return b.Key[:4]
	case g == report.ByMonth:
		return b.From.Format("Jan 06")
	case oneMonth:
		return strconv.Itoa(b.From.Day())
	default:
		return b.From.Format(shortDay)
	}
}

// bucketName names a month or year bucket ("Oct 2026", "2026").
func bucketName(b report.Bucket, g report.Granularity) string {
	if g == report.ByYear {
		return b.Key
	}
	return b.From.Format(monthFormat)
}

// bucketRow labels a bucket in the table; months and years that the range cuts
// (partial) or that have days without data (n/m days) are marked.
func bucketRow(b report.Bucket, g report.Granularity) string {
	if g == report.ByDay {
		return weekdayLabel(b.From)
	}
	var marks []string
	if b.Partial {
		marks = append(marks, "partial")
	}
	if incomplete(b) {
		marks = append(marks, dayCount(b.Summary))
	}
	if len(marks) == 0 {
		return bucketName(b, g)
	}
	return bucketName(b, g) + " (" + strings.Join(marks, ", ") + ")"
}

// incomplete reports whether b has data for some but not all of its days.
func incomplete(b report.Bucket) bool {
	return b.Summary.CompleteDays > 0 && !b.Summary.Complete()
}

// dayCount is "22/30 days".
func dayCount(s report.Summary) string {
	return strconv.Itoa(s.CompleteDays) + "/" + strconv.Itoa(s.Days) + " days"
}

// incompleteBuckets returns the buckets of r with data for only some of their days.
func incompleteBuckets(r report.Report) []report.Bucket {
	var out []report.Bucket
	for _, b := range r.Buckets {
		if incomplete(b) {
			out = append(out, b)
		}
	}
	return out
}

// incompleteMark follows the axis label of a month or year with days without data.
const incompleteMark = "*"

// gapNote is the chart caption on missing data: day charts show gaps, month and year
// charts name the buckets whose bars sum fewer days (marked on the axis).
func gapNote(r report.Report) string {
	if r.Granularity == report.ByDay {
		return "Gaps are days without data."
	}
	bs := incompleteBuckets(r)
	if len(bs) == 0 {
		return ""
	}
	names := make([]string, 0, maxNamedBuckets)
	for _, b := range bs[:min(len(bs), maxNamedBuckets)] {
		names = append(names, bucketName(b, r.Granularity)+" "+dayCount(b.Summary))
	}
	s := incompleteMark + " Not all days have data: " + strings.Join(names, ", ")
	if rest := len(bs) - len(names); rest > 0 {
		s += " and " + strconv.Itoa(rest) + " more"
	}
	return s + "."
}

// roundShares rounds shares (summing to ~1) to whole percents that sum to 100
// (largest remainder), so a split never reads 47 % + 20 % + 32 %.
func roundShares(shares []float64) []int {
	out := make([]int, len(shares))
	type rem struct {
		i int
		r float64
	}
	rems := make([]rem, len(shares))
	sum, total := 0, 0.0
	for _, s := range shares {
		total += s
	}
	if total <= 0 {
		return out
	}
	for i, s := range shares {
		v := s * percent
		out[i] = int(math.Floor(v))
		rems[i] = rem{i, v - math.Floor(v)}
		sum += out[i]
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r > rems[b].r })
	for k := 0; sum < percent && k < len(rems); k++ {
		out[rems[k].i]++
		sum++
	}
	return out
}
