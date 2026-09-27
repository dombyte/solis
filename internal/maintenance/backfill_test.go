package maintenance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/utils/clocktest"
)

var (
	bg  = context.Background()
	now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
)

type fixture struct {
	path string
	reg  *solis.Registry
	st   *storage.Storage
	out  bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	f := &fixture{path: filepath.Join(t.TempDir(), "solis.db"), reg: reg}
	f.st = f.open(t)
	// v2 history exists before the v3 cutover.
	rows := []storage.DailyRow{
		{Key: "pv_energy_daily", Day: "2025-06-01", Value: 100},
		{Key: "pv_energy_daily", Day: "2026-08-01", Value: 10},
		{Key: "pv_energy_daily", Day: "2026-09-02", Value: 5},
		{Key: "grid_export_daily", Day: "2026-09-02", Value: 1},
		{Key: "grid_import_daily", Day: "2026-09-02", Value: 3},
	}
	require.NoError(t, f.st.WritePoll(bg, storage.PollWrite{Daily: rows}))
	_, _, err = f.st.EnsureCutover(bg, period.Of(time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)))
	require.NoError(t, err)
	// Inverter-reported (pre-cutover) August value, higher than the daily sum.
	require.NoError(t, f.st.Backfill(bg, func(tx storage.BackfillTx) error {
		return tx.PutPeriod(period.Monthly, "pv_energy_monthly", "2026-08", 12)
	}))
	require.NoError(t, f.st.Close())
	return f
}

func (f *fixture) open(t *testing.T) *storage.Storage {
	t.Helper()
	cfg := &config.StorageSettings{Path: f.path, Synchronous: "NORMAL", TempStore: "MEMORY"}
	st, err := storage.New(cfg, f.reg, clocktest.New(now), zerolog.Nop())
	require.NoError(t, err)
	return st
}

func (f *fixture) env(t *testing.T, backup func() (string, error)) Env {
	return Env{
		DBPath: f.path, Backup: backup, Registry: f.reg, Now: now, Out: &f.out,
		Log: zerolog.Nop(),
		OpenStore: func() (Store, func() error, error) {
			st := f.open(t)
			return st, st.Close, nil
		},
	}
}

func okBackup() (string, error) { return "backups/solis.db.x.backup", nil }

func monthly(t *testing.T, f *fixture, key, month string) float64 {
	t.Helper()
	st := f.open(t)
	defer func() { _ = st.Close() }()
	m, _ := time.ParseInLocation("2006-01", month, time.Local)
	pts, err := st.GetMonthlyHistory(bg, key, m, m)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	return pts[0].Value
}

func TestBackfill_CurrentYear(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), 0))
	out := f.out.String()
	assert.Contains(t, out, "backup: backups/solis.db.x.backup")
	assert.Contains(t, out, "monthly  pv_energy_monthly        2026-08    12.00 kWh ->  10.00 kWh")
	assert.Contains(t, out, "monthly  grid_energy_monthly      2026-09      n/a kWh ->  -2.00 kWh")
	assert.Contains(t, out, "yearly   pv_energy_yearly         2026         n/a kWh ->  15.00 kWh")
	assert.NotContains(t, out, "2025", "closed years untouched with --years 0")
	assert.NotContains(t, out, "baseline")
	assert.Contains(t, out, "1 lower (daily-data gaps)")

	// Frozen pre-cutover month was rewritten inside the job's transaction.
	assert.InDelta(t, 10.0, monthly(t, f, "pv_energy_monthly", "2026-08"), 1e-9)
	assert.InDelta(t, -2.0, monthly(t, f, "grid_energy_monthly", "2026-09"), 1e-9)

	// Idempotent: a second run reports everything unchanged.
	f.out.Reset()
	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), 0))
	rec, same, lower := summary(t, f.out.String())
	assert.Positive(t, rec)
	assert.Equal(t, rec, same)
	assert.Zero(t, lower)
}

// summary extracts the counts of the report's summary line.
func summary(t *testing.T, out string) (rec, same, lower int) {
	t.Helper()
	i := strings.Index(out, "summary: ")
	require.GreaterOrEqual(t, i, 0)
	_, err := fmt.Sscanf(out[i:], "summary: %d rows recomputed, %d unchanged, %d lower",
		&rec, &same, &lower)
	require.NoError(t, err)
	return rec, same, lower
}

func TestBackfill_ClosedYearRefreshesBaseline(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), 1))
	out := f.out.String()
	assert.Contains(t, out, "yearly   pv_energy_yearly         2025")
	assert.Contains(t, out, "monthly  pv_energy_monthly        2025-06      n/a kWh -> 100.00 kWh")
	assert.Contains(t, out, "total    pv_energy_total          baseline      n/a kWh -> 100.00 kWh")

	st := f.open(t)
	defer func() { _ = st.Close() }()
	cs, err := st.CloseState(bg)
	require.NoError(t, err)
	assert.InDelta(t, 100.0, cs.Baseline["pv_energy_total"], 1e-9)
	assert.Equal(t, "2025", cs.BaselineYear)
}

func TestBackfill_RefusesWhileAppRuns(t *testing.T) {
	f := newFixture(t)
	server, err := AcquireShared(f.path)
	require.NoError(t, err)
	defer func() { _ = server.Release() }()

	backupCalled := false
	err = RunBackfill(bg, f.env(t, func() (string, error) {
		backupCalled = true
		return okBackup()
	}), 0)
	require.ErrorIs(t, err, ErrLocked)
	assert.False(t, backupCalled)

	// Conversely the server cannot start while a job holds the exclusive lock.
	require.NoError(t, server.Release())
	job, err := AcquireExclusive(f.path)
	require.NoError(t, err)
	_, err = AcquireShared(f.path)
	assert.ErrorIs(t, err, ErrLocked)
	require.NoError(t, job.Release())
	require.NoError(t, (*Lock)(nil).Release())
}

func TestBackfill_NoBackupNoWrite(t *testing.T) {
	f := newFixture(t)
	opened := false
	env := f.env(t, func() (string, error) { return "", errors.New("disk full") })
	env.OpenStore = func() (Store, func() error, error) {
		opened = true
		return nil, nil, errors.New("unreachable")
	}
	err := RunBackfill(bg, env, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was written")
	assert.False(t, opened)
	assert.InDelta(t, 12.0, monthly(t, f, "pv_energy_monthly", "2026-08"), 1e-9)
}

func TestBackfill_InvalidArgsAndOpenError(t *testing.T) {
	f := newFixture(t)
	assert.ErrorIs(t, RunBackfill(bg, f.env(t, okBackup), -1), ErrInvalidArgs)
	env := f.env(t, okBackup)
	env.OpenStore = func() (Store, func() error, error) { return nil, nil, errors.New("x") }
	assert.Error(t, RunBackfill(bg, env, 0))
}

func TestReportFormatAndCounts(t *testing.T) {
	r := Report{Lines: []Line{
		{Level: "monthly", Key: "pv_energy_monthly", Period: "2026-08", Old: 412.3, HadOld: true,
			New: 409.87, Unit: "kWh"},
		{Level: "monthly", Key: "grid_energy_monthly", Period: "2026-08", Old: -3.1, HadOld: true,
			New: -3.42, Unit: "kWh"},
		{Level: "yearly", Key: "pv_energy_yearly", Period: "2025", Old: 5230, HadOld: true,
			New: 4980.22, Unit: "kWh"},
		{Level: "yearly", Key: "grid_import_yearly", Period: "2025", Old: 1, HadOld: true,
			New: 1.001, Unit: "kWh"},
	}}
	var b bytes.Buffer
	require.NoError(t, r.Write(&b))
	golden := []string{
		"monthly  pv_energy_monthly        2026-08   412.30 kWh -> 409.87 kWh",
		"monthly  grid_energy_monthly      2026-08    -3.10 kWh ->  -3.42 kWh",
		"yearly   pv_energy_yearly         2025     5230.00 kWh -> 4980.22 kWh",
		"summary: 4 rows recomputed, 1 unchanged, 3 lower (daily-data gaps)",
	}
	for _, g := range golden {
		assert.Contains(t, b.String(), g)
	}
}
