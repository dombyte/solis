package pdf

import (
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/report"
)

// Methodology page geometry: formula table columns as shares of the content width.
const (
	formulaNameW = 0.27
	formulaExprW = 0.40
	formulaGap   = 8.0
	bulletIndent = 10.0
	paraGap      = 4.0
	methodGap    = 13.0 // between sections: this page is dense
)

// formula is one row of the formula table.
type formula struct {
	name, expr, note string
}

// standardFormulas are the established definitions (self-sufficiency and self-consumption
// rate as commonly used for PV systems, relative change and percentage points).
func standardFormulas() []formula {
	return []formula{
		{
			"Self-sufficiency", "(consumption − grid import) ÷ consumption",
			"Share of the consumption not bought from the grid (autarky).",
		},
		{
			"Self-consumption rate", "(PV − grid export) ÷ PV",
			"Share of the PV yield used on site, directly or through the battery.",
		},
		{"Change in %", "(value − comparison) ÷ comparison", "Energy figures."},
		{"Change in points", "value − comparison", "Shares: percentage points."},
	}
}

// derivedFormulas are this report's estimates from daily totals and plain balances; the
// estimates are marked ≈ in the report.
func derivedFormulas() []formula {
	return []formula{
		{
			"≈ Battery charge from PV", "min(battery charge, PV − grid export)",
			"Per day. The rest of the charge must have come from the grid.",
		},
		{
			"≈ PV used directly", "PV − grid export − battery charge from PV",
			"Per day. Also contains the PV-side balance difference.",
		},
		{
			"≈ Consumption from solar", "consumption − import − (discharge − charge from grid)",
			"Not covered by grid or battery; grid-charged energy counts as grid.",
		},
		{
			"≈ Discharged ÷ charged", "battery discharge ÷ battery charge",
			"The quotient of two uncalibrated counters, not an efficiency. " +
				"From 28 days on.",
		},
		{
			"Balance difference", "(PV + import + discharge) − (consumption + export + charge)",
			"Energy the counters do not account for: conversion losses, standby, " +
				"counter inaccuracy. From 28 days on.",
		},
	}
}

// methodPage explains the data used, the formulas and the rules of the report and returns
// the bottom of its content (tests keep it above the footer).
func methodPage(d *doc, r report.Report, m Meta) float64 {
	y := pageTitle(d, "How this report is calculated") + methodGap
	sectionTitle(d, y, "Data used")
	y = bullets(d, y+headingH+gapM, dataNotes(r, m))
	y += methodGap
	sectionTitle(d, y, "Standard figures")
	y = formulaTable(d, y+headingH+gapM, standardFormulas())
	y += methodGap
	sectionTitle(d, y, "Estimates and derived figures")
	y = bullets(d, y+headingH+gapM, []string{"The inverter provides daily totals only, " +
		"so it cannot tell which energy flowed where within a day. The values marked ≈ " +
		"are estimated with these rules; they are not standardised figures."})
	y = formulaTable(d, y, derivedFormulas())
	y += methodGap
	sectionTitle(d, y, "Rules")
	return bullets(d, y+headingH+gapM, ruleNotes(r))
}

// dataNotes describes the source data of r.
func dataNotes(r report.Report, m Meta) []string {
	s := r.Summary
	used := fmt.Sprintf("A day is used only when all eight counters were stored for it. "+
		"This report uses %d of %d days", s.CompleteDays, s.Days)
	if c := r.Comparison; c != nil {
		used += fmt.Sprintf(", the comparison period %d of %d days", c.Summary.CompleteDays,
			c.Summary.Days)
	}
	return []string{
		"The inverter's daily energy counters, stored by Solis Monitor for every calendar " +
			"day (" + zoneName(m.Generated) + "): PV yield, consumption of all loads, " +
			"household loads, backup output (the inverter assigns a small rest of the " +
			"consumption to neither), grid import, grid " +
			"export, battery charge and battery discharge. The counters count in steps of " +
			"0.1 kWh.",
		used + ".",
		"Not used: live power values, the inverter's own monthly, yearly and lifetime " +
			"counters and the net grid value.",
	}
}

// ruleNotes lists how values are aggregated, compared and rounded.
func ruleNotes(r report.Report) []string {
	out := []string{
		"Every share is calculated from the summed energy of the whole period, never as an " +
			"average of daily shares, so each day counts with its energy.",
		"Charts and the table group the days by " + r.Granularity.String() + "; a month " +
			"or year cut by the period is marked as partial.",
		comparisonRule(r.Comparison),
		"Values are shown with one decimal. The splits are rounded to whole percent so " +
			"their parts add up to 100\u00a0%.", // no break inside "100 %"
	}
	if r.TodayExcluded {
		out = append(out, "The requested period ended today. Today's counters are still "+
			"running, so the report ends the day before.")
	}
	return out
}

// comparisonRule explains how the comparison period was chosen and compared.
func comparisonRule(c *report.Comparison) string {
	if c == nil {
		return "No comparison period was selected."
	}
	how := "the same dates one year earlier (29 February becomes 28 February)"
	if c.Mode == report.ComparePrevious {
		how = "the period directly before: whole months are compared with the same number " +
			"of months before, a period starting on the 1st with the same days of the " +
			"period before, any other period with the same number of days before"
	}
	s := "Comparison: " + how + "."
	if c.Matched {
		s += " The periods are not equally long or days are missing, so the changes " +
			"compare only the days both periods have data for."
	}
	return s
}

// zoneName names the server's time zone ("Europe/Berlin", else its abbreviation).
func zoneName(t time.Time) string {
	if name := t.Location().String(); name != "Local" && name != "" {
		return name
	}
	return "time zone " + t.Format("MST")
}

// bullets writes each item as a wrapped bullet point from y and returns the bottom.
func bullets(d *doc, y float64, items []string) float64 {
	d.font(fontRegular, sizeBody, hex(colorBody))
	w := contentW - bulletIndent
	for _, item := range items {
		d.text(box{marginX, y, bulletIndent, noteLineH}, "•", alignLeft)
		for _, l := range d.wrap(item, w) {
			d.text(box{marginX + bulletIndent, y, w, noteLineH}, l, alignLeft)
			y += noteLineH
		}
		y += paraGap
	}
	return y
}

// formulaTable writes the formulas in three wrapped columns and returns the bottom.
func formulaTable(d *doc, y float64, fs []formula) float64 {
	nameW := contentW * formulaNameW
	exprW := contentW * formulaExprW
	noteW := contentW - nameW - exprW - formulaGap - formulaGap
	for _, f := range fs {
		top := y
		d.font(fontSemiBold, sizeSmall, hex(colorInk))
		yName := column(d, box{marginX, top, nameW, 0}, f.name)
		d.font(fontRegular, sizeSmall, hex(colorInk))
		yExpr := column(d, box{marginX + nameW + formulaGap, top, exprW, 0}, f.expr)
		d.font(fontRegular, sizeSmall, hex(colorMuted))
		yNote := column(d, box{marginX + contentW - noteW, top, noteW, 0}, f.note)
		y = max(yName, yExpr, yNote) + gapXS
		d.stroke(hex(colorRule), lineThin)
		d.line(marginX, y, marginX+contentW, y)
		y += gapS
	}
	return y
}

// column writes s wrapped into the width of b from b.y and returns the bottom.
func column(d *doc, b box, s string) float64 {
	y := b.y
	for _, l := range d.wrap(s, b.w) {
		d.text(box{b.x, y, b.w, noteLineH}, l, alignLeft)
		y += noteLineH
	}
	return y
}
