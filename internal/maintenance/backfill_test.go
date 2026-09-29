package maintenance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/storage/mocks"
	"github.com/dombyte/solis/internal/util/clocktest"
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
	cfg := storage.Settings{Path: f.path, Synchronous: "NORMAL", TempStore: "MEMORY"}
	st, err := storage.New(context.Background(), cfg, f.reg, clocktest.New(now), zerolog.Nop())
	require.NoError(t, err)
	return st
}

func (f *fixture) env(t *testing.T, backup func(context.Context) (string, error)) Env {
	return Env{
		DBPath: f.path, Backup: backup, Registry: f.reg, Now: now, Out: &f.out,
		OpenStore: func(context.Context) (Store, func() error, error) {
			st := f.open(t)
			return st, st.Close, nil
		},
	}
}

func okBackup(context.Context) (string, error) { return "backups/solis.db.x.backup", nil }

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
	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), Options{Years: 0}))
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
	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), Options{Years: 0}))
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

// --years 1 over a closed year with partial pre-cutover history: months from the oldest
// daily row on are recomputed, earlier months and the year keep their stored value
// (DB-H1), and pre-cutover years are never folded into the baseline (DB-H2).
func TestBackfill_ClosedYearKeepsIncompletePeriodsAndBaseline(t *testing.T) {
	f := newFixture(t)
	st := f.open(t)
	require.NoError(t, st.Backfill(bg, func(tx storage.BackfillTx) error {
		return tx.PutPeriod(period.Yearly, "pv_energy_yearly", "2025", 1234)
	}))
	require.NoError(t, st.Close())

	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), Options{Years: 1}))
	out := f.out.String()
	assert.Contains(t, out, "monthly  pv_energy_monthly        2025-06      n/a kWh -> 100.00 kWh")
	assert.Contains(t, out, "skipped  monthly  2025-05  daily history starts 2025-06-01")
	assert.Contains(t, out, "skipped  yearly   2025     daily history starts 2025-06-01")
	assert.NotContains(t, out, "yearly   pv_energy_yearly         2025")
	assert.NotContains(t, out, "baseline", "baseline year 2025 is before the cutover year")
	assert.Contains(t, out, "6 periods skipped (incomplete daily history)")

	st = f.open(t)
	defer func() { _ = st.Close() }()
	y2025 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.Local)
	pts, err := st.GetYearlyHistory(bg, "pv_energy_yearly", y2025, y2025)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	assert.InDelta(t, 1234.0, pts[0].Value, 1e-9, "inverter-reported year kept")
	cs, err := st.CloseState(bg)
	require.NoError(t, err)
	assert.Empty(t, cs.Baseline, "pre-cutover history is not carried into totals")
}

// Once a year after the cutover was folded, the baseline refresh sums exactly the
// cutover year through the baseline year, as the live aggregator does (DB-H2).
func TestBackfill_BaselineRefreshStartsAtCutoverYear(t *testing.T) {
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	f := &fixture{path: filepath.Join(t.TempDir(), "solis.db"), reg: reg}
	st := f.open(t)
	require.NoError(t, st.WritePoll(bg, storage.PollWrite{Daily: []storage.DailyRow{
		{Key: "pv_energy_daily", Day: "2024-06-01", Value: 1000}, // pre-cutover year
		{Key: "pv_energy_daily", Day: "2025-01-01", Value: 40},
		{Key: "pv_energy_daily", Day: "2025-02-01", Value: 60},
		{Key: "pv_energy_daily", Day: "2026-03-01", Value: 7},
	}}))
	_, _, err = st.EnsureCutover(bg, period.Of(time.Date(2025, 3, 1, 12, 0, 0, 0, time.Local)))
	require.NoError(t, err)
	require.NoError(t, st.WriteComputed(bg, storage.ComputedWrite{
		At:    now,
		Folds: []storage.BaselineFold{{Year: "2025", Add: map[string]float64{"pv_energy_total": 1}}},
	}))
	require.NoError(t, st.Close())

	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), Options{Years: 1}))
	assert.Contains(t, f.out.String(),
		"total    pv_energy_total          baseline     1.00 kWh -> 100.00 kWh")
	st = f.open(t)
	defer func() { _ = st.Close() }()
	cs, err := st.CloseState(bg)
	require.NoError(t, err)
	assert.InDelta(t, 100.0, cs.Baseline["pv_energy_total"], 1e-9, "2024 row excluded")
	assert.Equal(t, "2025", cs.BaselineYear)
}

func TestBackfill_RefusesWhileAppRuns(t *testing.T) {
	f := newFixture(t)
	server, err := AcquireShared(f.path)
	require.NoError(t, err)
	defer func() { _ = server.Release() }()

	backupCalled := false
	err = RunBackfill(bg, f.env(t, func(context.Context) (string, error) {
		backupCalled = true
		return okBackup(bg)
	}), Options{})
	require.ErrorIs(t, err, ErrLocked)
	var le *LockedError
	require.ErrorAs(t, err, &le)
	assert.Equal(t, LockPath(f.path), le.Path)
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
	env := f.env(t, func(context.Context) (string, error) { return "", errors.New("disk full") })
	env.OpenStore = func(context.Context) (Store, func() error, error) {
		opened = true
		return nil, nil, errors.New("unreachable")
	}
	err := RunBackfill(bg, env, Options{Years: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was written")
	assert.False(t, opened)
	assert.InDelta(t, 12.0, monthly(t, f, "pv_energy_monthly", "2026-08"), 1e-9)
}

func TestBackfill_InvalidArgsAndOpenError(t *testing.T) {
	f := newFixture(t)
	err := RunBackfill(bg, f.env(t, okBackup), Options{Years: -1})
	assert.ErrorIs(t, err, ErrInvalidArgs)
	var ae *ArgError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "--years", ae.Arg)
	assert.EqualError(t, ae, "invalid arguments: --years must be >= 0, got -1")
	env := f.env(t, okBackup)
	env.OpenStore = func(context.Context) (Store, func() error, error) { return nil, nil, errors.New("x") }
	assert.Error(t, RunBackfill(bg, env, Options{Years: 0}))
}

func TestReportFormatAndCounts(t *testing.T) {
	r := Report{Lines: []Line{
		{
			Level: "monthly", Key: "pv_energy_monthly", Period: "2026-08", Old: 412.3, HadOld: true,
			New: 409.87, Unit: "kWh",
		},
		{
			Level: "monthly", Key: "grid_energy_monthly", Period: "2026-08", Old: -3.1, HadOld: true,
			New: -3.42, Unit: "kWh",
		},
		{
			Level: "yearly", Key: "pv_energy_yearly", Period: "2025", Old: 5230, HadOld: true,
			New: 4980.22, Unit: "kWh",
		},
		{
			Level: "yearly", Key: "grid_import_yearly", Period: "2025", Old: 1, HadOld: true,
			New: 1.001, Unit: "kWh",
		},
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

func TestCheckPurged(t *testing.T) {
	now := period.Of(time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local))
	tests := []struct {
		purged  string
		years   int
		refused bool
	}{
		{"", 3, false},
		{"2025-08-05", 0, false},
		{"2025-08-05", 1, true},  // closed years and the baseline need all daily rows
		{"2026-03-01", 0, true},  // defensive: the current year itself is incomplete
		{"2026-01-01", 0, false}, // cleanup never passes the first open day
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%d", tt.purged, tt.years), func(t *testing.T) {
			tx := mocks.NewMockBackfillTx(t)
			tx.EXPECT().PurgedBefore().Return(tt.purged)
			err := checkPurged(tx, now, tt.years)
			if tt.refused {
				assert.ErrorIs(t, err, ErrPurgedHistory)
				var pe *PurgedHistoryError
				require.ErrorAs(t, err, &pe)
				assert.Equal(t, tt.purged, pe.PurgedBefore)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestRecompute_Errors(t *testing.T) {
	reg := newFixture(t).reg

	tx := mocks.NewMockBackfillTx(t)
	tx.EXPECT().PurgedBefore().Return("9999-01-01")
	_, err := Recompute(tx, reg, period.Of(now), Options{Years: 0})
	assert.ErrorIs(t, err, ErrPurgedHistory)

	tx = mocks.NewMockBackfillTx(t)
	tx.EXPECT().PurgedBefore().Return("")
	tx.EXPECT().FirstDailyDay().Return("2020-01-01", nil)
	_, err = Recompute(tx, reg, period.Period{Year: "x"}, Options{Years: 0})
	assert.ErrorIs(t, err, period.ErrInvalidKey)

	tx = mocks.NewMockBackfillTx(t)
	tx.EXPECT().PurgedBefore().Return("")
	tx.EXPECT().FirstDailyDay().Return("", errors.New("disk I/O error"))
	_, err = Recompute(tx, reg, period.Of(now), Options{Years: 0})
	assert.ErrorContains(t, err, "disk I/O error")
}

// Periods whose start lies before the oldest daily row (v2 retention deleted it, or
// logging began later) are skipped and keep their stored value (review DB-H1).
func TestRecompute_SkipsPeriodsWithoutCompleteDailyHistory(t *testing.T) {
	reg := newFixture(t).reg
	tx := mocks.NewMockBackfillTx(t)
	tx.EXPECT().PurgedBefore().Return("")
	tx.EXPECT().FirstDailyDay().Return("", nil) // no daily rows at all
	rep, err := Recompute(tx, reg, period.Of(now), Options{Years: 0})
	require.NoError(t, err)
	assert.Empty(t, rep.Lines, "nothing written without daily history")
	assert.Len(t, rep.Skipped, 10, "Jan..Sep + the year")
	assert.Equal(t, "skipped  yearly   2026     no daily history; stored value kept",
		rep.Skipped[9].String())
}

func TestPurgedHistoryError_Message(t *testing.T) {
	err := error(&PurgedHistoryError{PurgedBefore: "2025-01-01"})
	assert.ErrorIs(t, err, ErrPurgedHistory)
	assert.Contains(t, err.Error(), "daily rows before 2025-01-01 were removed")
}

// A symlinked database locks next to its real file, so the server and a job using the
// link or the target always contend for the same lock (review nit).
func TestLockPath_ResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.db")
	require.NoError(t, os.WriteFile(target, nil, 0o600))
	link := filepath.Join(dir, "link.db")
	require.NoError(t, os.Symlink(target, link))
	assert.Equal(t, LockPath(target), LockPath(link))
	missing := filepath.Join(dir, "missing.db")
	assert.Equal(t, missing+".lock", LockPath(missing))
}

// --force restores the pre-guard behaviour on purpose: periods without complete daily
// history are recomputed (overwriting the stored value) and the baseline sums all daily
// rows, pre-cutover years included.
func TestBackfill_ForceOverridesIncompletePeriodsAndBaseline(t *testing.T) {
	f := newFixture(t)
	st := f.open(t)
	require.NoError(t, st.Backfill(bg, func(tx storage.BackfillTx) error {
		return tx.PutPeriod(period.Yearly, "pv_energy_yearly", "2025", 1234)
	}))
	require.NoError(t, st.Close())

	require.NoError(t, RunBackfill(bg, f.env(t, okBackup), Options{Years: 1, Force: true}))
	out := f.out.String()
	assert.Contains(t, out, "force: periods without complete daily history are overwritten")
	assert.Contains(t, out, "yearly   pv_energy_yearly         2025     1234.00 kWh -> 100.00 kWh")
	assert.Contains(t, out, "monthly  pv_energy_monthly        2025-01      n/a kWh ->   0.00 kWh")
	assert.Contains(t, out, "0 periods skipped")
	assert.Contains(t, out, "total    pv_energy_total          baseline      n/a kWh -> 100.00 kWh")

	st = f.open(t)
	defer func() { _ = st.Close() }()
	cs, err := st.CloseState(bg)
	require.NoError(t, err)
	assert.InDelta(t, 100.0, cs.Baseline["pv_energy_total"], 1e-9, "pre-cutover 2025 included")
}

func TestRecompute_ForceStillRefusesPurgedHistory(t *testing.T) {
	reg := newFixture(t).reg
	tx := mocks.NewMockBackfillTx(t)
	tx.EXPECT().PurgedBefore().Return("2026-01-01")
	_, err := Recompute(tx, reg, period.Of(now), Options{Years: 1, Force: true})
	assert.ErrorIs(t, err, ErrPurgedHistory)
}
