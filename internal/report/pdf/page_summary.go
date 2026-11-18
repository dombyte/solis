package pdf

import (
	"strings"

	"github.com/dombyte/solis/internal/report"
)

// Summary page geometry.
const (
	headerH      = 60.0
	tileCols     = 4
	mainChartH   = 260.0
	highlightH   = 44.0
	highlightCnt = 3
	captionH     = 12.0
	generatedFmt = "2 Jan 2006, 15:04"
)

// summaryPage: header, key figure tiles, PV vs consumption chart, highlights.
func summaryPage(d *doc, r report.Report, m Meta) {
	y := header(d, r, m)
	y = keyFigures(d, r, y+sectionGap)
	y = mainChart(d, r, y+sectionGap)
	highlightRow(d, r, y+sectionGap)
}

// header draws the title block and returns its bottom.
func header(d *doc, r report.Report, m Meta) float64 {
	y := marginTop
	d.font(fontBold, sizeTitle, hex(colorInk))
	d.text(box{marginX, y, contentW / 2, 26}, "Energy Report", alignLeft)
	d.font(fontSemiBold, sizeHeading, hex(colorInk))
	d.text(box{marginX, y + 28, contentW * 2 / 3, 15}, rangeLabel(r.Range), alignLeft)
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	d.text(box{marginX, y + 44, contentW * 2 / 3, 12}, compareLine(r), alignLeft)

	right := box{marginX + contentW/2, y, contentW / 2, 14}
	d.font(fontBold, sizeBody, hex(colorBrand))
	d.text(right, "Solis Monitor", alignRight)
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	right.y += 15
	d.text(right, "Generated "+m.Generated.Format(generatedFmt), alignRight)
	right.y += 13
	d.text(right, "Version "+m.Version, alignRight)

	bottom := y + headerH
	d.stroke(hex(colorInk), lineRule)
	d.line(marginX, bottom, marginX+contentW, bottom)
	return bottom
}

// compareLine describes the comparison period.
func compareLine(r report.Report) string {
	c := r.Comparison
	if c == nil {
		return "No comparison period"
	}
	what := map[report.CompareMode]string{
		report.CompareLastYear: "same period last year", report.ComparePrevious: "previous period",
	}[c.Mode]
	return "Compared with " + rangeLabel(c.Range) + " (" + what + ")" + matchedLabel(*c)
}

// keyFigures draws the tiles in rows of four and returns their bottom.
func keyFigures(d *doc, r report.Report, y float64) float64 {
	ts := tiles(r)
	w := (contentW - tileGap*(tileCols-1)) / tileCols
	for i, t := range ts {
		col, row := i%tileCols, i/tileCols
		drawTile(d, box{
			marginX + float64(col)*(w+tileGap), y + float64(row)*(tileH+tileGap),
			w, tileH,
		}, t)
	}
	rows := (len(ts) + tileCols - 1) / tileCols
	return y + float64(rows)*(tileH+tileGap) - tileGap
}

// drawTile draws one key figure tile.
func drawTile(d *doc, b box, t tile) {
	d.frame(b, hex(colorRule))
	in := box{b.x + tilePad, b.y + tilePad - 2, b.w - 2*tilePad, 12}
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	d.text(in, t.label, alignLeft)
	in.y += 14
	in.h = 20
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	uw := d.width(t.unit) + gapS
	d.fit(fontBold, sizeValue, in.w-uw, t.value, hex(colorInk))
	d.text(in, t.value, alignLeft)
	vw := d.width(t.value)
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	d.text(box{in.x + vw + 3, in.y + gapXS, in.w - vw, in.h}, t.unit, alignLeft)
	if t.delta == "" {
		return
	}
	in.y += 21
	in.h = 12
	d.font(fontSemiBold, sizeSmall, hex(t.deltaColor))
	d.text(in, t.delta, alignLeft)
	dw := d.width(t.delta) + gapS
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	if base := fitBase(d, t.base, in.w-dw); base != "" {
		d.text(box{in.x + dw, in.y, in.w - dw, in.h}, base, alignLeft)
	}
}

// fitBase shortens "vs 3,694.7 kWh" to "vs 3,694.7" and then to nothing until it fits.
func fitBase(d *doc, base string, maxW float64) string {
	if d.width(base) <= maxW {
		return base
	}
	short := strings.TrimSuffix(strings.TrimSuffix(base, " kWh"), " %")
	if d.width(short) <= maxW {
		return short
	}
	return ""
}

// mainChart draws PV vs consumption per bucket and returns its bottom.
func mainChart(d *doc, r report.Report, y float64) float64 {
	sectionTitle(d, y, "PV yield vs consumption per "+r.Granularity.String())
	d.legend(marginX+contentW, y+headingH/2, []legendItem{
		{colorPV, "PV yield"}, {colorConsumption, "Consumption"},
	})
	area := box{marginX, y + headingH + 8, contentW, mainChartH}
	d.bars(barChart{
		area: area, ticks: chartTicks(r), suffix: "kWh",
		series: []barSeries{
			{colorPV, bucketValues(r, total(func(e report.Energy) float64 { return e.PV }))},
			{colorConsumption, bucketValues(r, total(func(e report.Energy) float64 {
				return e.Consumption
			}))},
		},
	})
	bottom := area.y + area.h + gapS
	caption(d, bottom, strings.TrimSpace("kWh per "+r.Granularity.String()+". "+gapNote(r)))
	return bottom + captionH
}

// highlightRow draws the three highlight boxes.
func highlightRow(d *doc, r report.Report, y float64) {
	hs := highlights(r)
	w := (contentW - tileGap*(highlightCnt-1)) / highlightCnt
	for i, h := range hs {
		b := box{marginX + float64(i)*(w+tileGap), y, w, highlightH}
		d.rect(b, hex(colorPanel))
		d.font(fontRegular, sizeSmall, hex(colorMuted))
		d.text(box{b.x + tilePad, b.y + 7, b.w - 2*tilePad, 12}, h.label, alignLeft)
		d.font(fontSemiBold, sizeBody, hex(colorInk))
		d.text(box{b.x + tilePad, b.y + 22, b.w - 2*tilePad, 14}, h.value, alignLeft)
	}
}

// sectionTitle writes a section heading at y.
func sectionTitle(d *doc, y float64, s string) {
	d.font(fontSemiBold, sizeHeading, hex(colorInk))
	d.text(box{marginX, y, contentW, headingH}, s, alignLeft)
}

// caption writes a small muted line at y.
func caption(d *doc, y float64, s string) {
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	d.text(box{marginX, y, contentW, captionH}, s, alignLeft)
}

// pageTitle writes the title of a following page and returns the content top.
func pageTitle(d *doc, s string) float64 {
	d.font(fontBold, sizePage, hex(colorInk))
	d.text(box{marginX, marginTop, contentW, 22}, s, alignLeft)
	y := marginTop + titleRuleY
	d.stroke(hex(colorInk), lineRule)
	d.line(marginX, y, marginX+contentW, y)
	return y
}

// footer writes the running footer with the page number.
func footer(d *doc, r report.Report, n, total int) {
	d.stroke(hex(colorRule), lineThin)
	d.line(marginX, footerY, marginX+contentW, footerY)
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	b := box{marginX, footerY + 4, contentW, 12}
	d.text(b, "Solis energy report · "+rangeLabel(r.Range), alignLeft)
	d.text(b, "Page "+itoa(n)+" of "+itoa(total), alignRight)
}
