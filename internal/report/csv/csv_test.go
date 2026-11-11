package csv

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/report"
)

func d(day int) time.Time { return time.Date(2026, time.October, day, 0, 0, 0, 0, time.UTC) }

func TestWrite(t *testing.T) {
	t.Parallel()
	r := report.Build(report.Input{
		Range:       report.Range{From: d(1), To: d(3)},
		Granularity: report.ByDay,
		Days: []report.Day{
			{Date: d(1), Energy: report.Energy{
				PV: 10.004, Consumption: 8, Household: 7.5, Backup: 0.5, Import: 2,
				Export: 3, Charge: 1, Discharge: 1,
			}},
			{Date: d(3), Energy: report.Energy{Consumption: 1, Household: 1, Import: 1}},
		},
	})
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, r))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, "period,from,to,partial,days,complete_days,pv_kwh,consumption_kwh,"+
		"household_kwh,backup_kwh,grid_import_kwh,grid_export_kwh,battery_charge_kwh,"+
		"battery_discharge_kwh,pv_direct_use_est_kwh,battery_charge_from_pv_est_kwh,"+
		"self_sufficiency_pct,self_consumption_pct", lines[0])
	assert.Equal(t, "2026-10-01,2026-10-01,2026-10-01,false,1,1,10.00,8.00,7.50,0.50,"+
		"2.00,3.00,1.00,1.00,6.00,1.00,75.00,70.01", lines[1])
	assert.Equal(t, "2026-10-02,2026-10-02,2026-10-02,false,1,0,,,,,,,,,,,,", lines[2],
		"missing day: empty cells, not zeros")
	assert.Equal(t, "2026-10-03,2026-10-03,2026-10-03,false,1,1,0.00,1.00,1.00,0.00,"+
		"1.00,0.00,0.00,0.00,0.00,0.00,0.00,", lines[3], "no PV: no self-consumption")
}

func TestWrite_MonthBuckets(t *testing.T) {
	t.Parallel()
	r := report.Build(report.Input{
		Range:       report.Range{From: d(15), To: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
		Granularity: report.ByMonth,
	})
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, r))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[1], "2026-10,2026-10-15,2026-10-31,true,17,0,"))
	assert.True(t, strings.HasPrefix(lines[2], "2026-11,2026-11-01,2026-11-02,true,2,0,"))
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWrite_Error(t *testing.T) {
	t.Parallel()
	r := report.Build(report.Input{Range: report.Range{From: d(1), To: d(1)}})
	require.ErrorContains(t, Write(failWriter{}, r), "disk full")
}
