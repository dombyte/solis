package pdf

import (
	"fmt"

	"github.com/dombyte/solis/internal/report"
)

// Energy flow page geometry.
const (
	splitBarH    = 19.0
	splitLegendH = 28.0
	gridChartH   = 140.0
	gridSummaryW = 165.0
	noteLineH    = 12.5
)

// flowsPage: PV split, consumption sources, grid exchange, battery and notes.
func flowsPage(d *doc, r report.Report, _ Meta) {
	s := r.Summary
	y := pageTitle(d, "Energy flows") + sectionGap
	y = splitSection(d, y, "Where your solar energy went · "+kwh(energyOf(s, s.Total.PV)), []split{
		{colorPV, "used directly*", s.PVSplit.Solar, s.PVSplit.SolarShare, true},
		{colorBattery, "charged into battery", s.PVSplit.Battery, s.PVSplit.BatteryShare, true},
		{colorExport, "exported to grid", s.PVSplit.Grid, s.PVSplit.GridShare, false},
	})
	y = splitSection(d, y+sectionGap,
		"Where your consumption came from · "+kwh(energyOf(s, s.Total.Consumption)), []split{
			{colorPV, "from solar", s.Sources.Solar, s.Sources.SolarShare, true},
			{
				colorBattery, "from battery", s.Sources.Battery, s.Sources.BatteryShare,
				gridCharged(s),
			},
			{colorImport, "from grid", s.Sources.Grid, s.Sources.GridShare, false},
		})
	caption(d, y+gapXS, loadsCaption(d, s))
	y = gridSection(d, r, y+captionH+sectionGap)
	y = batterySection(d, s, y+sectionGap)
	noteBlock(d, y+sectionGap, notes(r))
}

// loadsCaption is the loads line plus the hint on the ≈ mark; the hint is left out when
// both do not fit on the one caption line (the notes and the last page explain ≈).
func loadsCaption(d *doc, s report.Summary) string {
	line := loadsLine(s) + "."
	withHint := line + " " + estimateMark + " estimated from daily totals."
	d.font(fontRegular, sizeSmall, hex(colorMuted))
	if d.width(withHint) <= contentW {
		return withHint
	}
	return line
}

// estimateMark prefixes values estimated from daily totals (explained on the last page).
const estimateMark = "≈"

// split is one part of a split section; estimate marks a part derived from daily totals.
type split struct {
	color    uint32
	label    string
	energy   float64
	share    *float64
	estimate bool
}

// splitSection draws a heading, a 100 % bar and its legend; returns the bottom.
func splitSection(d *doc, y float64, title string, parts []split) float64 {
	sectionTitle(d, y, title)
	y += headingH + gapM
	bar := make([]splitPart, len(parts))
	shares := make([]float64, len(parts))
	for i, p := range parts {
		bar[i] = splitPart{p.color, p.share}
		if p.share != nil {
			shares[i] = *p.share
		}
	}
	d.splitBar(box{marginX, y, contentW, splitBarH}, bar)
	y += splitBarH + gapM
	pct := roundShares(shares)
	w := contentW / float64(len(parts))
	for i, p := range parts {
		x := marginX + float64(i)*w
		d.rect(box{x, y + 3, legendSwatch, legendSwatch}, hex(p.color))
		label, amount := noValue+" "+p.label, noValue
		if p.share != nil {
			label, amount = fmt.Sprintf("%d %% %s", pct[i], p.label), number(p.energy)+" kWh"
			if p.estimate {
				label, amount = estimateMark+" "+label, estimateMark+" "+amount
			}
		}
		d.font(fontSemiBold, sizeBody, hex(colorInk))
		d.text(box{x + legendSwatch + 5, y, w - legendSwatch - 5, 13}, label, alignLeft)
		d.font(fontRegular, sizeSmall, hex(colorMuted))
		d.text(box{x + legendSwatch + 5, y + 13, w, 12}, amount, alignLeft)
	}
	return y + splitLegendH
}

// loadsLine splits the consumption into household loads, backup output and, when the
// two counters do not add up to the consumption counter, the remainder the inverter
// assigns to neither; the whole percents add up to 100.
func loadsLine(s report.Summary) string {
	t := s.Total
	if t.Consumption <= 0 {
		return "Household loads " + noValue + " · backup output " + noValue
	}
	other := t.OtherLoads()
	if number(other) == number(0) {
		pct := roundShares([]float64{t.Household / t.Consumption, t.Backup / t.Consumption})
		return fmt.Sprintf("Household loads %s kWh (%d %%) · backup output %s kWh (%d %%)",
			number(t.Household), pct[0], number(t.Backup), pct[1])
	}
	line := fmt.Sprintf("Household loads %s kWh · backup output %s kWh · not assigned %s kWh",
		number(t.Household), number(t.Backup), number(other))
	if other < 0 {
		return line // the parts exceed the consumption counter: no meaningful shares
	}
	pct := roundShares([]float64{
		t.Household / t.Consumption, t.Backup / t.Consumption, other / t.Consumption,
	})
	return fmt.Sprintf("Household loads %s kWh (%d %%) · backup output %s kWh (%d %%) · "+
		"not assigned %s kWh (%d %%)", number(t.Household), pct[0], number(t.Backup), pct[1],
		number(other), pct[2])
}

// gridSection draws export (up) and import (down) per bucket plus the totals.
func gridSection(d *doc, r report.Report, y float64) float64 {
	sectionTitle(d, y, "Grid exchange per "+r.Granularity.String())
	d.legend(marginX+contentW, y+headingH/2, []legendItem{
		{colorExport, "Export (up)"}, {colorImport, "Import (down)"},
	})
	area := box{marginX, y + headingH + 8, contentW - gridSummaryW - tileGap*2, gridChartH}
	d.divergingChart(area, chartTicks(r),
		barSeries{colorExport, bucketValues(r, total(func(e report.Energy) float64 {
			return e.Export
		}))},
		barSeries{colorImport, bucketValues(r, total(func(e report.Energy) float64 {
			return e.Import
		}))})
	s := r.Summary
	rows := [][2]string{
		{"Exported", kwh(energyOf(s, s.Total.Export))},
		{"Imported", kwh(energyOf(s, s.Total.Import))},
		{"PV yield exported", share(s.PVSplit.GridShare)},
		{"Consumption from grid", share(s.Sources.GridShare)},
	}
	keyValues(d, box{marginX + contentW - gridSummaryW, area.y + 20, gridSummaryW, 0}, rows)
	return area.y + area.h
}

// keyValues writes label/value rows into b.
func keyValues(d *doc, b box, rows [][2]string) {
	const rowH = 18.0
	for i, row := range rows {
		y := b.y + float64(i)*rowH
		d.font(fontRegular, sizeBody, hex(colorMuted))
		d.text(box{b.x, y, b.w, rowH}, row[0], alignLeft)
		d.font(fontSemiBold, sizeBody, hex(colorInk))
		d.text(box{b.x, y, b.w, rowH}, row[1], alignRight)
	}
}

// batterySection draws the battery and system tiles.
func batterySection(d *doc, s report.Summary, y float64) float64 {
	sectionTitle(d, y, "Battery and system")
	y += headingH + gapM
	diff := kwh(s.BalanceDiff)
	if s.BalanceDiffShare != nil {
		diff += " · " + number(*s.BalanceDiffShare*percent) + " %"
	}
	ratio := share(s.BatteryRatio)
	if s.BatteryRatio != nil {
		ratio = estimateMark + " " + ratio
	}
	ts := []tile{
		{label: "Charged", value: kwh(energyOf(s, s.Total.Charge))},
		{label: "Discharged", value: kwh(energyOf(s, s.Total.Discharge))},
		{label: "Discharged ÷ charged", value: ratio},
		{label: "Balance difference", value: diff},
	}
	w := (contentW - tileGap*(tileCols-1)) / tileCols
	for i, t := range ts {
		b := box{marginX + float64(i)*(w+tileGap), y, w, tileH * 3 / 4}
		d.frame(b, hex(colorRule))
		d.font(fontRegular, sizeSmall, hex(colorMuted))
		d.text(box{b.x + tilePad, b.y + 6, b.w, 12}, t.label, alignLeft)
		d.fit(fontBold, sizeTile, b.w-2*tilePad, t.value, hex(colorInk))
		d.text(box{b.x + tilePad, b.y + 21, b.w - tilePad, 18}, t.value, alignLeft)
	}
	return y + tileH*3/4
}

// noteBlock writes the notes below a rule, wrapped to the content width.
func noteBlock(d *doc, y float64, ns []string) {
	d.stroke(hex(colorRule), lineThin)
	d.line(marginX, y, marginX+contentW, y)
	y += 6
	d.font(fontRegular, sizeSmall, hex(colorBody))
	for _, n := range ns {
		y = d.paragraph(y, n) + gapXS
	}
}
