package pdf

import (
	"time"

	"github.com/dombyte/solis/internal/report"
)

// Table page geometry: rows per page follow from the space between title and footer.
const (
	tableRowH    = 17.0
	tableTop     = marginTop + titleRuleY + sectionGap
	tableReserve = 3*tableRowH + captionH // header, totals, rules, footnote
)

// rowsPerPage is the number of bucket rows that fit on one table page.
func rowsPerPage() int {
	space := contentBotY - tableTop - tableReserve
	return int(space / tableRowH)
}

// tablePages returns the number of table pages for n buckets (at least one).
func tablePages(n int) int {
	per := rowsPerPage()
	return max(1, (n+per-1)/per)
}

// tableColumns are the column edges (share of the width) and headers.
func tableColumns(g report.Granularity) ([]float64, []string) {
	first := map[report.Granularity]string{
		report.ByDay: "Day", report.ByMonth: "Month", report.ByYear: "Year",
	}[g]
	return []float64{0, 0.25, 0.35, 0.49, 0.59, 0.69, 0.79, 0.9, 1},
		[]string{
			first, "PV", "Consumption", "Import", "Export", "Charge", "Discharge",
			"Self-suff.",
		}
}

// tablePage draws the idx-th page of the bucket table; the last one adds the totals.
func tablePage(d *doc, r report.Report, idx int) {
	title := "Values per " + r.Granularity.String()
	if idx > 0 {
		title += " (continued)"
	}
	y := pageTitle(d, title) + sectionGap/half
	edges, header := tableColumns(r.Granularity)
	table{edges: edges, face: fontSemiBold, size: sizeSmall, rowH: rowH}.row(d, y, header, nil)
	y += rowH
	d.stroke(hex(colorInk), lineRule)
	d.line(marginX, y, marginX+contentW, y)
	body := table{edges: edges, face: fontRegular, size: sizeSmall, rowH: tableRowH}
	from := idx * rowsPerPage()
	to := min(len(r.Buckets), from+rowsPerPage())
	y = bucketRows(d, r, body, y, r.Buckets[from:to])
	if to == len(r.Buckets) {
		d.stroke(hex(colorInk), lineRule)
		d.line(marginX, y, marginX+contentW, y)
		body.face = fontBold
		bucketCells(d, y, body, "Total", r.Summary)
		y += tableRowH
	}
	caption(d, y+gapM, tableFootnote(r))
}

// bucketRows writes one row per bucket from y (weekends shaded, grid-charging days
// marked) and returns the bottom.
func bucketRows(d *doc, r report.Report, t table, y float64, bs []report.Bucket) float64 {
	daily := r.Granularity == report.ByDay
	charged := chargedDays(r.Summary)
	for _, b := range bs {
		if daily && isWeekend(b.From) {
			d.rect(box{marginX, y, contentW, tableRowH}, hex(colorPanel))
		}
		label := bucketRow(b, r.Granularity)
		if daily && charged[b.From] {
			label += " *"
		}
		bucketCells(d, y, t, label, b.Summary)
		y += tableRowH
	}
	return y
}

// bucketCells writes one row of values; "–" without data.
func bucketCells(d *doc, y float64, t table, label string, s report.Summary) {
	cells := []string{label, noValue, noValue, noValue, noValue, noValue, noValue, noValue}
	if s.CompleteDays > 0 {
		e := s.Total
		cells = []string{
			label, number(e.PV), number(e.Consumption), number(e.Import), number(e.Export),
			number(e.Charge), number(e.Discharge), share(s.SelfSufficiency),
		}
	}
	t.row(d, y, cells, nil)
}

// chargedDays indexes the grid-charging days.
func chargedDays(s report.Summary) map[time.Time]bool {
	out := make(map[time.Time]bool, len(s.GridChargingDays))
	for _, d := range s.GridChargingDays {
		out[d] = true
	}
	return out
}

// tableFootnote explains units and marks.
func tableFootnote(r report.Report) string {
	s := "Energy in kWh. "
	if r.Granularity == report.ByDay {
		s += "Weekends shaded. "
		if len(r.Summary.GridChargingDays) > 0 {
			s += "* grid charging detected. "
		}
	} else if len(incompleteBuckets(r)) > 0 {
		s += "n/m days: only n of the m days have data. "
	}
	return s + noValue + " no data. Every series in full detail: CSV export."
}

// isWeekend reports whether day is a Saturday or Sunday.
func isWeekend(day time.Time) bool {
	return day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
}
