package pdf

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/report"
)

func ptr(v float64) *float64 { return &v }

func utc(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestNumber(t *testing.T) {
	t.Parallel()
	for in, want := range map[float64]string{
		0: "0.0", 403.66: "403.7", 4512.34: "4,512.3", 1234567.8: "1,234,567.8",
		-3: "−3.0", -0.04: "0.0", 999.96: "1,000.0",
	} {
		assert.Equal(t, want, number(in), in)
	}
	assert.Equal(t, "88.5", number(88.446), "two decimals first (88.45), like the page")
	assert.InDelta(t, 88.5, round1(88.446), 1e-9)
	assert.Equal(t, "–", kwh(nil))
	assert.Equal(t, "76.2 %", share(ptr(0.7619)))
	assert.Equal(t, "–", share(nil))
}

func TestRoundShares(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []int{47, 20, 33}, roundShares([]float64{0.4702, 0.2024, 0.3274}))
	assert.Equal(t, []int{34, 33, 33}, roundShares([]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}))
	assert.Equal(t, []int{0, 0, 0}, roundShares([]float64{0, 0, 0}))
}

func TestLabels(t *testing.T) {
	t.Parallel()
	oct := report.Range{From: utc(2026, 10, 1), To: utc(2026, 10, 31)}
	year := report.Range{From: utc(2025, 1, 1), To: utc(2025, 12, 31)}
	odd := report.Range{From: utc(2026, 10, 1), To: utc(2026, 10, 29)}
	quarter := report.Range{From: utc(2026, 1, 1), To: utc(2026, 3, 31)}
	assert.Equal(t, "Oct 2026", periodLabel(oct))
	assert.Equal(t, "2025", periodLabel(year))
	assert.Equal(t, "1 Oct 2026 – 29 Oct 2026", periodLabel(odd))
	assert.Equal(t, "1 Jan 2026 – 31 Mar 2026", periodLabel(quarter))
	assert.Equal(t, "Oct 2026", columnLabel(oct, "This period"))
	assert.Equal(t, "This period", columnLabel(odd, "This period"))
	assert.Equal(t, "Sat 10 Oct", weekdayLabel(utc(2026, 10, 10)))

	b := report.Bucket{Key: "2026-03", From: utc(2026, 3, 15), Partial: true}
	assert.Equal(t, "Mar 2026 (partial)", bucketRow(b, report.ByMonth))
	assert.Equal(t, "2026", bucketRow(report.Bucket{Key: "2026"}, report.ByYear))
	assert.Equal(t, "Mar 26", bucketTick(b, report.ByMonth, false))
	assert.Equal(t, "15 Mar", bucketTick(b, report.ByDay, false))
	assert.Equal(t, "15", bucketTick(b, report.ByDay, true))
}

func TestChange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		f     report.Figure
		want  string
		color uint32
	}{
		{"energy up, better", report.Figure{
			Kind: report.KindEnergy, Better: report.HigherIsBetter,
			Change: ptr(32.7), ChangePct: ptr(0.0881),
		}, "↑ 8.8 %", colorBetter},
		{"import down, better", report.Figure{
			Kind: report.KindEnergy, Better: report.LowerIsBetter,
			Change: ptr(-11.8), ChangePct: ptr(-0.13),
		}, "↓ 13.0 %", colorBetter},
		{"ratio from the compared values, not the headline", report.Figure{
			Kind: report.KindRatio, Better: report.HigherIsBetter, Change: ptr(-0.13649),
			Value: ptr(0.5), Compared: ptr(0.84254), Base: ptr(0.97960),
		}, "↓ 13.7 points", colorWorse},
		{"neutral", report.Figure{
			Kind: report.KindEnergy, Change: ptr(1), ChangePct: ptr(0.05),
		}, "↑ 5.0 %", colorBody},
		{"zero base: kWh", report.Figure{
			Kind: report.KindEnergy, Better: report.HigherIsBetter, Change: ptr(13.5),
			Value: ptr(40), Compared: ptr(13.5), Base: ptr(0),
		}, "↑ 13.5 kWh", colorBetter},
		{"no change", report.Figure{
			Kind: report.KindEnergy, Better: report.HigherIsBetter, Change: ptr(0.01),
			Compared: ptr(5.02), Base: ptr(5.01), ChangePct: ptr(0.0002),
		}, "±0.0 %", colorMuted},
		{"no data", report.Figure{Kind: report.KindEnergy}, "no data", colorMuted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, color := change(tt.f, true)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.color, color)
		})
	}
	plain, _ := change(report.Figure{
		Kind: report.KindEnergy, Change: ptr(-1), ChangePct: ptr(-0.037),
	}, false)
	assert.Equal(t, "−3.7 %", plain)
}

// matchedReport is October with 1-10 Oct only, compared with a complete October 2025.
func matchedReport() report.Report {
	r := octoberReport(true)
	in := report.Input{
		Range: r.Range, Granularity: report.ByDay, Days: october()[:10],
		Compare: &report.CompareInput{Mode: report.CompareLastYear, Range: r.Comparison.Range},
	}
	for _, d := range october() {
		d.Date = d.Date.AddDate(-1, 0, 0)
		d.PV *= 0.5
		in.Compare.Days = append(in.Compare.Days, d)
	}
	return report.Build(in)
}

func TestMatchedComparison(t *testing.T) {
	t.Parallel()
	r := matchedReport()
	require.True(t, r.Comparison.Matched)
	pv := tiles(r)[0]
	assert.Equal(t, "↑ 100.0 %", pv.delta)
	assert.Equal(t, "on 10 days", pv.base)
	assert.Equal(t, "Compared with 1 Oct 2025 – 31 Oct 2025 (same period last year) · "+
		"10 matched days", compareLine(r))
	assert.Equal(t, "Compared with Oct 2025 · 10 matched days", comparisonTitle(*r.Comparison))
	assert.Equal(t, []string{"Figure", "This period", "Comparison", "Change"}, comparisonHeader(r))
	assert.Contains(t, comparisonCaption(r), "Days are missing (10 of 31 days and 31 of 31 days)")
	uneven := octoberReport(true)
	uneven.Comparison.Matched, uneven.Comparison.MatchedDays = true, 30
	assert.Contains(t, comparisonCaption(uneven), "The periods differ in length (31 of 31")
	assert.Contains(t, comparisonCaption(r), "only the 10 days both periods have data for")
	assert.Contains(t, comparisonRule(r.Comparison), "days both periods have data for")

	none := report.Comparison{Matched: true}
	assert.Empty(t, tileBase(report.Figure{}, none), "no common days: only \"no data\"")
	assert.Equal(t, "Compared with – · 0 matched days",
		strings.Replace(comparisonTitle(none), periodLabel(none.Range), "–", 1))
	assert.Equal(t, []string{"Figure", "Oct 2026", "Oct 2025", "Change"},
		comparisonHeader(octoberReport(true)))
}

func TestLoadsLine(t *testing.T) {
	t.Parallel()
	line := func(c, h, b float64) string {
		return loadsLine(report.Summary{Total: report.Energy{Consumption: c, Household: h, Backup: b}})
	}
	assert.Equal(t, "Household loads – · backup output –", line(0, 0, 0))
	assert.Equal(t, "Household loads 9.0 kWh (90 %) · backup output 1.0 kWh (10 %)", line(10, 9, 1))
	assert.Equal(t, "Household loads 348.0 kWh (36 %) · backup output 602.4 kWh (61 %) · "+
		"not assigned 30.8 kWh (3 %)", line(981.2, 348.0, 602.4))
	assert.Equal(t, "Household loads 6.0 kWh · backup output 5.0 kWh · not assigned −1.0 kWh",
		line(10, 6, 5))
}

func TestLoadsCaption(t *testing.T) {
	t.Parallel()
	d, err := newDoc(meta())
	require.NoError(t, err)
	d.newPage()
	short := report.Summary{Total: report.Energy{Consumption: 10, Household: 9, Backup: 1}}
	assert.True(t, strings.HasSuffix(loadsCaption(d, short), "≈ estimated from daily totals."))
	long := report.Summary{Total: report.Energy{
		Consumption: 123456.7, Household: 61234.5, Backup: 51234.5,
	}}
	got := loadsCaption(d, long)
	assert.True(t, strings.HasSuffix(got, "not assigned 10,987.7 kWh (9 %)."), got)
	assert.LessOrEqual(t, d.width(got), contentW)
	require.NoError(t, d.err)
}

func TestBatteryNote(t *testing.T) {
	t.Parallel()
	assert.Contains(t, batteryNote(report.Summary{}), "at least 28 complete days")
	assert.Contains(t, batteryNote(report.Summary{BatteryRatio: ptr(1.152)}), "not an efficiency")
	assert.Empty(t, batteryNote(report.Summary{BatteryRatio: ptr(0.89)}))
}

func TestIncompleteBuckets(t *testing.T) {
	t.Parallel()
	bucket := func(month time.Month, days, complete int) report.Bucket {
		return report.Bucket{
			From: utc(2026, month, 1), Summary: report.Summary{Days: days, CompleteDays: complete},
		}
	}
	r := report.Report{Granularity: report.ByMonth, Buckets: []report.Bucket{
		bucket(5, 31, 0), bucket(6, 30, 22), bucket(7, 31, 31),
	}}
	assert.Equal(t, "May 2026", bucketRow(r.Buckets[0], report.ByMonth), "no data shows as –")
	assert.Equal(t, "Jun 2026 (22/30 days)", bucketRow(r.Buckets[1], report.ByMonth))
	assert.Equal(t, "Jul 2026", bucketRow(r.Buckets[2], report.ByMonth))
	cut := bucket(10, 29, 20)
	cut.Partial = true
	assert.Equal(t, "Oct 2026 (partial, 20/29 days)", bucketRow(cut, report.ByMonth))
	assert.Equal(t, "Not all days have data: Jun 2026 22/30 days.", gapNote(r))
	assert.Contains(t, tableFootnote(r), "n/m days: only n of the m days have data.")

	r.Buckets = append(r.Buckets, bucket(8, 31, 1), bucket(9, 30, 2), bucket(10, 31, 3))
	assert.Equal(t, "Not all days have data: Jun 2026 22/30 days, Aug 2026 1/31 days, "+
		"Sep 2026 2/30 days and 1 more.", gapNote(r))
	r.Buckets = r.Buckets[2:3]
	assert.Empty(t, gapNote(r))
	assert.NotContains(t, tableFootnote(r), "n/m")
	r.Granularity = report.ByDay
	assert.Equal(t, "Gaps are days without data.", gapNote(r))
}

func TestTiles(t *testing.T) {
	t.Parallel()
	r := octoberReport(true)
	ts := tiles(r)
	assert.Len(t, ts, 8)
	assert.Equal(t, tile{
		label: "PV yield", value: "403.7", unit: "kWh", delta: "↑ 8.7 %",
		base: "vs 371.4 kWh", deltaColor: colorBetter,
	}, ts[0])
	assert.Equal(t, tile{label: "Battery charged", value: "82.1", unit: "kWh"}, tiles(octoberReport(false))[6])
	assert.Empty(t, tiles(octoberReport(false))[0].delta)
}

func TestNotes(t *testing.T) {
	t.Parallel()
	ns := notes(octoberReport(false))
	require.Len(t, ns, 2)
	assert.Contains(t, ns[0], "On 28 Oct the battery took in more")
	assert.Contains(t, ns[0], "at least 1.9 kWh came from the grid")
	assert.Contains(t, ns[1], "and the battery charge from the grid")

	short := report.Report{
		Range:   report.Range{From: utc(2026, 10, 1), To: utc(2026, 10, 10)},
		Summary: report.Summary{Days: 10, CompleteDays: 7}, TodayExcluded: true,
	}
	ns = notes(short)
	assert.Len(t, ns, 5)
	assert.Equal(t, "Today is still in progress and not part of this report: it ends on "+
		"10 Oct 2026.", ns[4])
	assert.NotContains(t, ns[0], "grid")
	assert.Contains(t, ns[2], "at least 28 complete days")
	assert.Equal(t, "Data for 7 of 10 days; missing days are left out of every figure.", ns[3])

	many := report.Summary{}
	for d := 1; d <= 7; d++ {
		many.GridChargingDays = append(many.GridChargingDays, utc(2026, 10, d))
	}
	assert.Equal(t, "1 Oct, 2 Oct, 3 Oct, 4 Oct, 5 Oct and 2 more days", dayList(many))
}

func TestHighlightsAndCaption(t *testing.T) {
	t.Parallel()
	r := octoberReport(true)
	assert.Equal(t, []highlight{
		{"Average per day", "13.0 kWh PV · 10.7 kWh used"},
		{"Best PV day", "Sat 10 Oct · 23.9 kWh"},
		{"Days under 0.5 kWh grid import", "10 of 31 days"},
	}, highlights(r))
	assert.Contains(t, comparisonCaption(r), "Both periods are complete")
	assert.Equal(t, "Compared with 1 Oct 2025 – 31 Oct 2025 (same period last year)",
		compareLine(r))
	assert.Equal(t, "No comparison period", compareLine(octoberReport(false)))
}

func TestMethodology(t *testing.T) {
	t.Parallel()
	r := octoberReport(true)
	m := meta()
	data := dataNotes(r, m)
	assert.Contains(t, data[0], "(UTC)")
	assert.Equal(t, "A day is used only when all eight counters were stored for it. This "+
		"report uses 31 of 31 days, the comparison period 31 of 31 days.", data[1])
	assert.Contains(t, data[2], "net grid value")
	rules := ruleNotes(r)
	assert.Contains(t, rules[1], "group the days by day")
	assert.Contains(t, rules[2], "the same dates one year earlier")
	assert.Len(t, rules, 4)
	assert.Equal(t, "No comparison period was selected.", comparisonRule(nil))
	assert.Contains(t, comparisonRule(&report.Comparison{
		Mode:    report.ComparePrevious,
		Matched: true,
	}), "only the days both periods have data for")
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	assert.Equal(t, "Europe/Berlin", zoneName(time.Date(2026, 10, 29, 0, 0, 0, 0, berlin)))
	assert.Equal(t, "CET", zoneName(time.Date(2026, 10, 29, 0, 0, 0, 0, time.FixedZone("CET", 3600))))
	// time.Local without a TZ name: the abbreviation, which depends on the machine.
	assert.True(t, strings.HasPrefix(zoneName(time.Date(2026, 10, 29, 0, 0, 0, 0, time.Local)), "time zone ") ||
		time.Local.String() != "Local")
	for _, f := range append(standardFormulas(), derivedFormulas()...) {
		assert.NotEmpty(t, f.expr, f.name)
	}
}

func TestWrapKeepsNoBreakSpace(t *testing.T) {
	t.Parallel()
	d, err := newDoc(meta())
	require.NoError(t, err)
	d.newPage()
	d.font(fontRegular, sizeBody, hex(colorInk))
	for _, l := range d.wrap(strings.Repeat("word ", 30)+"100\u00a0%", 200) {
		assert.NotEqual(t, "%", l)
	}
	assert.Equal(t, []string{"100\u00a0%"}, d.wrap("100\u00a0%", 5))
}

func TestMethodPageFitsWorstCase(t *testing.T) {
	t.Parallel()
	r := octoberReport(true)
	r.TodayExcluded = true
	r.Comparison.Mode = report.ComparePrevious
	r.Comparison.Matched = true
	d, err := newDoc(meta())
	require.NoError(t, err)
	d.newPage()
	bottom := methodPage(d, r, meta())
	require.NoError(t, d.err)
	assert.Less(t, bottom, contentBotY, "the methodology page must not run into the footer")
}

func TestNiceMaxAndTables(t *testing.T) {
	t.Parallel()
	for in, want := range map[float64]float64{
		0: 1, 0.4: 1, 1.5: 2, 23.9: 25, 26: 50, 412: 500, 4100: 5000, 100: 100,
	} {
		assert.InDelta(t, want, niceMax(in), 1e-9, in)
	}
	assert.Equal(t, 1, tablePages(0))
	assert.Equal(t, 1, tablePages(31))
	assert.Equal(t, 2, tablePages(62))
	assert.Equal(t, "2.5", axisNumber(2.5))
	assert.Equal(t, "25", axisNumber(25))
}
