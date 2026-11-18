package pdf

import (
	"bytes"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/report"
)

// october is the example month of the report design.
func october() []report.Day {
	rows := [][9]float64{
		{1, 13.5, 9.4, 8.8, 0.6, 0.8, 4.3, 2.2, 2.0},
		{2, 11.7, 10.4, 10.1, 0.3, 1.1, 1.5, 3.0, 2.5},
		{3, 17.8, 12.9, 12.5, 0.4, 0.4, 4.6, 2.5, 2.3},
		{4, 18.7, 13.3, 13.0, 0.3, 0.0, 5.6, 3.9, 3.3},
		{5, 18.5, 11.3, 10.9, 0.4, 0.9, 7.2, 3.3, 3.0},
		{6, 16.0, 9.2, 8.8, 0.4, 0.7, 6.6, 3.5, 3.1},
		{7, 22.6, 10.3, 9.9, 0.4, 0.8, 12.2, 3.7, 3.4},
		{8, 21.5, 11.5, 10.8, 0.7, 0.0, 10.5, 2.6, 2.5},
		{9, 23.7, 9.4, 8.9, 0.5, 0.4, 13.8, 2.1, 1.9},
		{10, 23.9, 11.5, 10.9, 0.6, 0.8, 12.2, 3.3, 3.0},
		{11, 23.2, 11.9, 11.3, 0.6, 0.7, 11.1, 2.1, 1.9},
		{12, 19.8, 9.8, 9.3, 0.5, 0.0, 9.5, 3.5, 2.9},
		{13, 8.9, 9.2, 8.5, 0.7, 1.3, 0.4, 2.3, 2.0},
		{14, 16.4, 9.2, 8.7, 0.5, 0.3, 6.8, 3.2, 3.0},
		{15, 14.2, 9.8, 9.3, 0.5, 0.3, 4.0, 2.8, 2.6},
		{16, 3.4, 9.5, 9.1, 0.4, 6.4, 0.1, 1.2, 1.1},
		{17, 3.2, 10.6, 10.1, 0.5, 7.6, 0, 1.1, 1.0},
		{18, 7.3, 12.0, 11.4, 0.6, 5.3, 0, 2.6, 2.2},
		{19, 7.6, 11.4, 10.7, 0.7, 4.8, 0.4, 2.7, 2.4},
		{20, 5.3, 9.2, 8.9, 0.3, 4.6, 0.2, 1.9, 1.6},
		{21, 3.1, 9.0, 8.6, 0.4, 6.3, 0.2, 1.1, 1.0},
		{22, 8.9, 10.7, 10.3, 0.4, 2.9, 0.4, 2.6, 2.2},
		{23, 3.1, 11.4, 10.6, 0.8, 8.7, 0.2, 1.1, 1.0},
		{24, 3.1, 11.6, 11.2, 0.4, 9.0, 0.2, 1.1, 0.9},
		{25, 13.3, 12.1, 11.7, 0.4, 0.8, 1.0, 3.2, 2.6},
		{26, 15.2, 11.4, 10.8, 0.6, 1.2, 4.2, 2.6, 2.3},
		{27, 14.2, 10.5, 9.8, 0.7, 0.4, 3.3, 2.7, 2.3},
		{28, 2.1, 11.4, 10.7, 0.7, 11.3, 0.0, 4.0, 3.5},
		{29, 11.7, 9.1, 8.8, 0.3, 0.5, 2.3, 2.6, 2.2},
		{30, 20.2, 10.3, 9.5, 0.8, 0.0, 9.8, 4.2, 4.0},
		{31, 11.6, 11.2, 10.8, 0.4, 0.4, 0.2, 3.4, 3.2},
	}
	out := make([]report.Day, len(rows))
	for i, r := range rows {
		out[i] = report.Day{
			Date: time.Date(2026, time.October, int(r[0]), 0, 0, 0, 0, time.UTC),
			Energy: report.Energy{
				PV: r[1], Consumption: r[2], Household: r[3], Backup: r[4],
				Import: r[5], Export: r[6], Charge: r[7], Discharge: r[8],
			},
		}
	}
	return out
}

func octoberReport(compare bool) report.Report {
	in := report.Input{
		Range: report.Range{
			From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		Granularity: report.ByDay, Days: october(),
	}
	if compare {
		base := october()
		for i := range base {
			base[i].Date = base[i].Date.AddDate(-1, 0, 0)
			base[i].PV *= 0.92
			base[i].Import *= 1.15
		}
		in.Compare = &report.CompareInput{
			Mode: report.CompareLastYear,
			Range: report.Range{
				From: in.Range.From.AddDate(-1, 0, 0),
				To:   in.Range.To.AddDate(-1, 0, 0),
			},
			Days: base,
		}
	}
	return report.Build(in)
}

func meta() Meta {
	return Meta{Generated: time.Date(2026, 11, 1, 8, 14, 0, 0, time.UTC), Version: "3.4.0"}
}

// pageCount counts the page objects of a PDF.
func pageCount(b []byte) int {
	return len(regexp.MustCompile(`/Type /Page\b[^s]`).FindAll(b, -1))
}

// render writes r and, with SOLIS_PDF_OUT set, keeps a copy for a visual check.
func render(t *testing.T, r report.Report, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, r, meta()))
	require.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
	if dir := os.Getenv("SOLIS_PDF_OUT"); dir != "" {
		require.NoError(t, os.WriteFile(dir+"/"+name+".pdf", buf.Bytes(), 0o600))
	}
	return buf.Bytes()
}

func TestWrite_ExampleMonth(t *testing.T) {
	t.Parallel()
	b := render(t, octoberReport(true), "october")
	assert.Equal(t, 5, pageCount(b))
}

func TestWrite_NoComparison(t *testing.T) {
	t.Parallel()
	b := render(t, octoberReport(false), "october-nocompare")
	assert.Equal(t, 5, pageCount(b))
}

func TestWrite_LongDailyTableContinues(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	r := report.Build(report.Input{
		Range:       report.Range{From: from, To: from.AddDate(0, 0, 61)},
		Granularity: report.ByDay, Days: october(),
	})
	b := render(t, r, "62days")
	assert.Equal(t, 6, pageCount(b))
}

func TestWrite_NoData(t *testing.T) {
	t.Parallel()
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	r := report.Build(report.Input{
		Range:       report.Range{From: from, To: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)},
		Granularity: report.ByYear,
		Compare: &report.CompareInput{
			Mode:  report.ComparePrevious,
			Range: report.Range{From: from.AddDate(-7, 0, 0), To: from.AddDate(0, 0, -1)},
		},
	})
	b := render(t, r, "empty-years")
	assert.Equal(t, 5, pageCount(b))
}

func TestWrite_Months(t *testing.T) {
	t.Parallel()
	r := report.Build(report.Input{
		Range: report.Range{
			From: time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		Granularity: report.ByMonth, Days: october(),
	})
	render(t, r, "months")
}
