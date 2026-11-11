package pdf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dombyte/solis/internal/report"
)

// Patterns page geometry.
const (
	ssChartH  = 120.0
	rowH      = 19.0
	weekBarH  = 10.0
	countBoxH = 40.0
	countCols = 3
	colGap    = 22.0
)

// patternsPage: self-sufficiency per bucket, notable days, weekday/weekend, comparison.
func patternsPage(d *doc, r report.Report, _ Meta) {
	y := pageTitle(d, "Patterns and comparison") + sectionGap
	sectionTitle(d, y, "Self-sufficiency per "+r.Granularity.String())
	area := box{marginX, y + headingH + 8, contentW, ssChartH}
	d.bars(barChart{
		area: area, ticks: chartTicks(r), top: percent, suffix: "%",
		series: []barSeries{{colorSingle, bucketValues(r, func(s report.Summary) *float64 {
			if s.SelfSufficiency == nil {
				return nil
			}
			v := *s.SelfSufficiency * percent
			return &v
		})}},
	})
	y = area.y + area.h + gapS
	caption(d, y, "Whole period: "+share(r.Summary.SelfSufficiency)+
		" (from the summed energy, not the average of the bars).")
	y += captionH + sectionGap
	colW := (contentW - colGap) / half
	notableDays(d, r.Patterns, box{marginX, y, colW, 0})
	y = weekdays(d, r, box{marginX + colW + colGap, y, colW, 0})
	comparisonTable(d, r, y+sectionGap)
}

// notableDays draws the extremes as a small table.
func notableDays(d *doc, p report.Patterns, b box) {
	d.font(fontSemiBold, sizeHeading, hex(colorInk))
	d.text(box{b.x, b.y, b.w, headingH}, "Notable days", alignLeft)
	rows := []struct {
		label string
		v     *report.DayValue
	}{
		{"Best PV day", p.BestPV},
		{"Lowest PV day", p.LowestPV},
		{"Highest consumption", p.MaxConsumption},
		{"Highest grid import", p.MaxImport},
	}
	y := b.y + headingH + gapM
	for _, row := range rows {
		day, val := noValue, noValue
		if row.v != nil {
			day, val = weekdayLabel(row.v.Date), number(row.v.Value)+" kWh"
		}
		cells := box{b.x, y, b.w, rowH}
		d.font(fontRegular, sizeBody, hex(colorMuted))
		d.text(cells, row.label, alignLeft)
		d.font(fontRegular, sizeBody, hex(colorInk))
		d.text(box{b.x + b.w*0.5, y, b.w * 0.3, rowH}, day, alignLeft)
		d.font(fontSemiBold, sizeBody, hex(colorInk))
		d.text(cells, val, alignRight)
		y += rowH
		d.stroke(hex(colorRule), lineThin)
		d.line(b.x, y, b.x+b.w, y)
	}
}

// weekdays draws weekday vs weekend consumption and the day counts; returns the bottom.
func weekdays(d *doc, r report.Report, b box) float64 {
	p := r.Patterns
	d.font(fontSemiBold, sizeHeading, hex(colorInk))
	d.text(box{b.x, b.y, b.w, headingH}, "Consumption per day", alignLeft)
	y := b.y + headingH + gapM
	top := maxPtr(p.WeekdayConsumption, p.WeekendConsumption)
	for _, row := range []struct {
		label string
		days  int
		v     *float64
	}{
		{"Mon–Fri", p.WeekdayDays, p.WeekdayConsumption},
		{"Sat–Sun", p.WeekendDays, p.WeekendConsumption},
	} {
		d.font(fontRegular, sizeBody, hex(colorInk))
		d.text(box{b.x, y, b.w, 13}, fmt.Sprintf("%s (%d days)", row.label, row.days), alignLeft)
		d.font(fontSemiBold, sizeBody, hex(colorInk))
		d.text(box{b.x, y, b.w, 13}, kwh(row.v), alignRight)
		y += 15
		d.rect(box{b.x, y, b.w, weekBarH}, hex(colorPanel))
		if row.v != nil && top > 0 {
			d.rect(box{b.x, y, b.w * *row.v / top, weekBarH}, hex(colorConsumption))
		}
		y += weekBarH + gapL
	}
	counts := [][2]string{
		{strconv.Itoa(p.LowImportDays), "low-import days"},
		{strconv.Itoa(len(r.Summary.GridChargingDays)), "grid-charge days"},
		{fmt.Sprintf("%d/%d", r.Summary.CompleteDays, r.Summary.Days), "days with data"},
	}
	w := (b.w - tileGap*(countCols-1)) / countCols
	for i, c := range counts {
		cb := box{b.x + float64(i)*(w+tileGap), y, w, countBoxH}
		d.rect(cb, hex(colorPanel))
		d.font(fontBold, sizeHeading, hex(colorInk))
		d.text(box{cb.x + 7, cb.y + 5, w, 16}, c[0], alignLeft)
		d.font(fontRegular, sizeSmall, hex(colorMuted))
		d.text(box{cb.x + 7, cb.y + 22, w - 7, 12}, c[1], alignLeft)
	}
	return y + countBoxH
}

// maxPtr returns the larger of two optional values (0 when both are missing).
func maxPtr(a, b *float64) float64 {
	m := 0.0
	for _, v := range []*float64{a, b} {
		if v != nil && *v > m {
			m = *v
		}
	}
	return m
}

// comparisonTable draws the key figures of both periods with their changes.
func comparisonTable(d *doc, r report.Report, y float64) {
	c := r.Comparison
	if c == nil {
		sectionTitle(d, y, "Comparison")
		caption(d, y+headingH+gapS, "No comparison period was selected.")
		return
	}
	sectionTitle(d, y, comparisonTitle(*c))
	y += headingH + gapM
	cols := []float64{0, 0.42, 0.62, 0.82, 1} // column edges as share of the width
	header := comparisonHeader(r)
	tbl := table{edges: cols, face: fontSemiBold, size: sizeBody, rowH: rowH}
	tbl.row(d, y, header, nil)
	tbl.face = fontRegular
	y += rowH
	d.stroke(hex(colorInk), lineRule)
	d.line(marginX, y, marginX+contentW, y)
	for _, f := range r.Figures {
		text, color := change(f, false)
		cells := []string{
			figureLabel(f.ID), withUnit(f.Compared, f.Kind), withUnit(f.Base, f.Kind), text,
		}
		tbl.row(d, y, cells, []uint32{colorInk, colorInk, colorMuted, color})
		y += rowH
		d.stroke(hex(colorRule), lineThin)
		d.line(marginX, y, marginX+contentW, y)
	}
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	d.paragraph(y+gapS, comparisonCaption(r))
}

// comparisonTitle heads the comparison table; a matched comparison names its days.
func comparisonTitle(c report.Comparison) string {
	s := "Compared with " + periodLabel(c.Range)
	if c.Matched {
		s += fmt.Sprintf(" · %d matched %s", c.MatchedDays, plural(c.MatchedDays, "day", "days"))
	}
	return s
}

// comparisonHeader labels the columns: the period names only when the columns hold the
// whole periods (a matched comparison shows a part of each).
func comparisonHeader(r report.Report) []string {
	if r.Comparison.Matched {
		return []string{"Figure", "This period", "Comparison", "Change"}
	}
	return []string{
		"Figure", columnLabel(r.Range, "This period"),
		columnLabel(r.Comparison.Range, "Comparison"), "Change",
	}
}

// table is a row layout: column edges as shares of the content width; the first column
// is left-aligned, the others right-aligned.
type table struct {
	edges      []float64
	face       string
	size, rowH float64
}

// row writes cells at y; colors per cell (nil = ink).
func (t table) row(d *doc, y float64, cells []string, colors []uint32) {
	for i, s := range cells {
		color := uint32(colorInk)
		if colors != nil {
			color = colors[i]
		}
		d.font(t.face, t.size, hex(color))
		x0, x1 := marginX+t.edges[i]*contentW, marginX+t.edges[i+1]*contentW
		align := alignRight
		if i == 0 {
			align = alignLeft
		}
		d.text(box{x0, y, x1 - x0, t.rowH}, s, align)
	}
}

// wrap breaks s at spaces into lines no wider than maxW in the current font.
func (d *doc) wrap(s string, maxW float64) []string {
	var lines []string
	cur := ""
	// Split on plain spaces only: a no-break space ("100 %") keeps its words together.
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' }) {
		next := w
		if cur != "" {
			next = cur + " " + w
		}
		if cur != "" && d.width(next) > maxW {
			lines = append(lines, cur)
			next = w
		}
		cur = next
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// paragraph writes s wrapped to the content width in the current font from y and
// returns the bottom.
func (d *doc) paragraph(y float64, s string) float64 {
	for _, l := range d.wrap(s, contentW) {
		d.text(box{marginX, y, contentW, noteLineH}, l, alignLeft)
		y += noteLineH
	}
	return y
}

// itoa formats n.
func itoa(n int) string { return strconv.Itoa(n) }
