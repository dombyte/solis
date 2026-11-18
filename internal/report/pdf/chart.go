package pdf

import (
	"math"
	"strconv"

	"github.com/dombyte/solis/internal/report"
)

// Chart geometry and scale.
const (
	axisW        = 30.0 // y labels left of the plot
	tickH        = 13.0 // x labels below the plot
	gridLines    = 5
	barGapShare  = 0.25 // part of a bucket slot left empty between groups
	maxTicks     = 12
	tickEvery    = 5 // day-of-month labels: 1, 5, 10, ...
	splitGap     = 1.5
	decade       = 10.0
	legendGap    = 12.0
	legendTextGp = 4.0
)

// niceSteps are the mantissas of round axis maxima.
func niceSteps() []float64 { return []float64{1, 2, 2.5, 5, 10} }

// niceMax rounds v up to 1, 2, 2.5 or 5 times a power of ten (at least 1).
func niceMax(v float64) float64 {
	if v <= 1 {
		return 1
	}
	exp := math.Pow(decade, math.Floor(math.Log10(v)))
	for _, m := range niceSteps() {
		if m*exp >= v {
			return m * exp
		}
	}
	return decade * exp
}

// barSeries is one series of a bar chart; nil values are buckets without data.
type barSeries struct {
	color  uint32
	values []*float64
}

// bucketValues reads one value per bucket; nil for buckets without complete days.
func bucketValues(r report.Report, get func(report.Summary) *float64) []*float64 {
	out := make([]*float64, len(r.Buckets))
	for i, b := range r.Buckets {
		if b.Summary.CompleteDays > 0 {
			out[i] = get(b.Summary)
		}
	}
	return out
}

// total reads one energy series of a summary.
func total(field func(report.Energy) float64) func(report.Summary) *float64 {
	return func(s report.Summary) *float64 {
		v := field(s.Total)
		return &v
	}
}

// maxOf returns the largest value of all series (0 without values).
func maxOf(ss ...barSeries) float64 {
	m := 0.0
	for _, s := range ss {
		for _, v := range s.values {
			if v != nil && *v > m {
				m = *v
			}
		}
	}
	return m
}

// chartTicks labels the buckets on the x axis without crowding it.
func chartTicks(r report.Report) []string {
	n := len(r.Buckets)
	oneMonth := r.Range.From.Format(monthFormat) == r.Range.To.Format(monthFormat)
	dayOfMonth := r.Granularity == report.ByDay && oneMonth && n > maxTicks
	step := (n + maxTicks - 1) / maxTicks
	out := make([]string, n)
	for i, b := range r.Buckets {
		show := i%step == 0
		if dayOfMonth { // 1, 5, 10, ... reads better than every third day
			show = b.From.Day() == 1 || b.From.Day()%tickEvery == 0
		}
		if show {
			out[i] = bucketTick(b, r.Granularity, oneMonth) + axisMark(b, r.Granularity)
		}
	}
	return out
}

// axisMark marks a month or year with days without data (explained by gapNote below the
// chart); day charts show such days as gaps.
func axisMark(b report.Bucket, g report.Granularity) string {
	if g != report.ByDay && incomplete(b) {
		return incompleteMark
	}
	return ""
}

// barChart is a grouped bar chart over the buckets, y from 0 to top.
type barChart struct {
	area   box
	ticks  []string
	series []barSeries
	top    float64 // axis maximum; 0 = niceMax of the data
	suffix string  // appended to the top axis label ("kWh", "%")
}

// bars draws c.
func (d *doc) bars(c barChart) {
	top := c.top
	if top <= 0 {
		top = niceMax(maxOf(c.series...))
	}
	plot := box{c.area.x + axisW, c.area.y, c.area.w - axisW, c.area.h - tickH}
	d.yAxis(plot, top, c.suffix)
	slot := plot.w / float64(len(c.ticks))
	barW := slot * (1 - barGapShare) / float64(len(c.series))
	for i := range c.ticks {
		x := plot.x + float64(i)*slot + slot*barGapShare/half
		for j, s := range c.series {
			if v := s.values[i]; v != nil && *v > 0 {
				h := math.Min(*v, top) / top * plot.h
				d.rect(box{x + float64(j)*barW, plot.y + plot.h - h, barW, h}, hex(s.color))
			}
		}
	}
	d.baseline(plot, plot.y+plot.h)
	d.xTicks(plot, c.ticks, plot.y+plot.h)
}

// yAxis draws the grid lines and their labels.
func (d *doc) yAxis(plot box, top float64, suffix string) {
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	d.stroke(hex(colorGrid), lineThin)
	for i := range gridLines + 1 {
		v := top * float64(i) / gridLines
		y := plot.y + plot.h - plot.h*float64(i)/gridLines
		if i > 0 {
			d.line(plot.x, y, plot.x+plot.w, y)
		}
		label := axisNumber(v)
		if i == gridLines && suffix != "" {
			label += " " + suffix
		}
		d.text(box{plot.x - axisW - tileGap, y - tickH/2, axisW + tileGap - 3, tickH},
			label, alignRight)
	}
}

// axisNumber formats an axis value without needless decimals.
func axisNumber(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// baseline draws the x axis at y.
func (d *doc) baseline(plot box, y float64) {
	d.stroke(hex(colorMuted), lineThin)
	d.line(plot.x, y, plot.x+plot.w, y)
}

// xTicks writes the non-empty tick labels below y, centered on their slots.
func (d *doc) xTicks(plot box, ticks []string, y float64) {
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	slot := plot.w / float64(len(ticks))
	lastRight := math.Inf(-1)
	for i, t := range ticks {
		if t == "" {
			continue
		}
		cx := plot.x + (float64(i)+center)*slot
		w := d.width(t) + legendTextGp
		if cx-w/2 < lastRight { // would overlap the previous label
			continue
		}
		d.text(box{cx - w/2, y + 1, w, tickH}, t, alignCenter)
		lastRight = cx + w/half
	}
}

// divergingChart draws up above and down below a center line on a shared scale.
func (d *doc) divergingChart(area box, ticks []string, up, down barSeries) {
	top := niceMax(maxOf(up, down))
	halfH := (area.h - tickH) / half
	plot := box{area.x + axisW, area.y, area.w - axisW, halfH * half}
	mid := plot.y + halfH
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	for _, l := range []struct {
		y float64
		s string
	}{{plot.y, axisNumber(top)}, {mid, "0"}, {plot.y + plot.h, axisNumber(top)}} {
		d.text(box{area.x, l.y - tickH/2, axisW - 5, tickH}, l.s, alignRight)
	}
	slot := plot.w / float64(len(ticks))
	w := slot * (1 - barGapShare)
	for i := range ticks {
		x := plot.x + float64(i)*slot + slot*barGapShare/half
		if v := up.values[i]; v != nil {
			h := *v / top * halfH
			d.rect(box{x, mid - h, w, h}, hex(up.color))
		}
		if v := down.values[i]; v != nil {
			d.rect(box{x, mid, w, *v / top * halfH}, hex(down.color))
		}
	}
	d.baseline(plot, mid)
	d.xTicks(plot, ticks, plot.y+plot.h)
}

// legendItem is a colored swatch with a label.
type legendItem struct {
	color uint32
	label string
}

// legend draws items right-aligned ending at right, vertically centered on y.
func (d *doc) legend(right, y float64, items []legendItem) {
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	x := right
	for i := len(items) - 1; i >= 0; i-- {
		w := d.width(items[i].label)
		x -= w
		d.text(box{x, y - tickH/2, w + 1, tickH}, items[i].label, alignLeft)
		x -= legendSwatch + legendTextGp
		d.rect(box{x, y - legendSwatch/2, legendSwatch, legendSwatch}, hex(items[i].color))
		x -= legendGap
	}
}

// splitPart is one segment of a 100 % bar.
type splitPart struct {
	color uint32
	share *float64
}

// splitBar draws a 100 % bar of parts; an empty split is a gray bar.
func (d *doc) splitBar(b box, parts []splitPart) {
	if parts[0].share == nil {
		d.rect(b, hex(colorPanel))
		return
	}
	x := b.x
	usable := b.w - splitGap*float64(len(parts)-1)
	for _, p := range parts {
		w := *p.share * usable
		d.rect(box{x, b.y, w, b.h}, hex(p.color))
		x += w + splitGap
	}
}
