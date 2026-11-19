package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util/clocktest"
)

// failOn makes every <event> (INSERT, UPDATE, DELETE) on table fail.
func failOn(t *testing.T, s *Storage, table, event string) {
	t.Helper()
	insertRows(t, s, `CREATE TRIGGER fail_`+table+`_`+event+` BEFORE `+event+` ON `+table+
		` BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
}

// A closed connection fails every read and write with a storage error.
func TestStorage_ClosedDatabase(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t, day("2026-08-05"))
	require.NoError(t, s.Close())
	from, to := day("2026-08-01"), day("2026-08-05")
	keys := []string{"pv_energy_daily"}

	_, err := s.HasDailyData(ctx)
	assert.ErrorContains(t, err, "has daily data")
	_, err = s.SumDaily(ctx, keys, "2026-08-01", "2026-08-05")
	assert.ErrorContains(t, err, "sum daily")
	assert.ErrorContains(t, s.WritePoll(ctx, PollWrite{}), "begin transaction")
	assert.ErrorContains(t, s.WriteComputed(ctx, ComputedWrite{}), "begin transaction")
	assert.ErrorContains(t, s.Backfill(ctx, func(BackfillTx) error { return nil }),
		"begin transaction")
	_, _, err = s.EnsureCutover(ctx, period.Of(day("2026-08-05")))
	assert.ErrorContains(t, err, "begin transaction")
	_, err = s.Seed(ctx, []string{"2026-08-05"})
	assert.ErrorContains(t, err, "seed daily")
	_, err = s.Seed(ctx, nil)
	assert.ErrorContains(t, err, "seed status")
	_, err = s.GetErrorHistory(ctx, "solis_status", from, to)
	assert.ErrorContains(t, err, "query error history")
	_, err = s.GetDailyValues(ctx, keys, from, to)
	assert.ErrorContains(t, err, "query daily values")
	_, err = s.GetTotalHistory(ctx, "pv_energy_total")
	assert.ErrorContains(t, err, "query total")
	assert.Error(t, s.loadMeta(ctx))
	assert.ErrorContains(t, s.CleanupAll(ctx), "cleanup error_data")
}

func TestNew_SchemaAndMetaErrors(t *testing.T) {
	t.Parallel()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	tests := map[string]struct {
		setup string
		want  string
	}{
		// IF NOT EXISTS skips the view, the index on it fails.
		"schema": {`CREATE VIEW daily_values AS SELECT 1 AS date`, "init schema"},
		"meta":   {`CREATE TABLE meta (key TEXT)`, "load meta"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "solis.db")
			db, err := open(ctx, testConfig(path), zerolog.Nop())
			require.NoError(t, err)
			_, err = db.Exec(tt.setup)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			_, err = New(ctx, testConfig(path), reg, clocktest.New(time.Now()), zerolog.Nop())
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestOpen_DirectoryError(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	_, err := open(ctx, testConfig(filepath.Join(file, "sub", "x.db")), zerolog.Nop())
	assert.ErrorContains(t, err, "create directory")
}

// A failing statement inside a write transaction is a storage error, not a rejection:
// nothing of the write is committed.
func TestWrites_StatementErrors(t *testing.T) {
	t.Parallel()
	now := day("2026-08-05")
	tests := []struct {
		name, table, event, want string
		write                    func(*Storage) error
	}{
		{"daily", "daily_values", "INSERT", "write daily", func(s *Storage) error {
			return s.WritePoll(ctx, PollWrite{Daily: []DailyRow{
				{Key: "pv_energy_daily", Day: "2026-08-05", Value: 1},
			}})
		}},
		{"status", "error_data", "INSERT", "write status", func(s *Storage) error {
			return s.WritePoll(ctx, PollWrite{Status: []StatusRow{
				{Key: "solis_status", Raw: 1, At: now},
			}})
		}},
		{"close", "meta", "INSERT", "write meta", func(s *Storage) error {
			return s.WritePoll(ctx, PollWrite{Close: []DayClose{
				{Key: "pv_energy_daily", Day: "2026-08-05"},
			}})
		}},
		{"computed", "monthly_values", "INSERT", "upsert", func(s *Storage) error {
			return s.WriteComputed(ctx, ComputedWrite{Rows: []PeriodRow{
				{Level: period.Monthly, Key: "pv_energy_monthly", Period: "2026-08", Value: 1},
			}})
		}},
		{"freeze", "meta", "INSERT", "write meta", func(s *Storage) error {
			return s.WriteComputed(ctx, ComputedWrite{Freezes: []Freeze{
				{Level: period.Monthly, Period: "2026-07"},
			}})
		}},
		{"fold", "meta", "INSERT", "write meta", func(s *Storage) error {
			return s.WriteComputed(ctx, ComputedWrite{Folds: []BaselineFold{
				{Year: "2026", Add: map[string]float64{"pv_energy_total": 1}},
			}})
		}},
		{"fold year", "meta", "INSERT", "write meta", func(s *Storage) error {
			return s.WriteComputed(ctx, ComputedWrite{Folds: []BaselineFold{{Year: "2026"}}})
		}},
		{"cutover", "meta", "INSERT", "write meta", func(s *Storage) error {
			_, _, err := s.EnsureCutover(ctx, period.Of(now))
			return err
		}},
		{"baseline", "meta", "INSERT", "write meta", func(s *Storage) error {
			return s.Backfill(ctx, func(tx BackfillTx) error {
				return tx.PutBaseline(map[string]float64{"pv_energy_total": 1})
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _, _ := newStore(t, now)
			failOn(t, s, tt.table, tt.event)
			err := tt.write(s)
			require.ErrorContains(t, err, tt.want)
			assert.False(t, IsRejection(err))
		})
	}
}

func TestWriteComputed_InvalidFreezeLevel(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t, day("2026-08-05"))
	err := s.WriteComputed(ctx, ComputedWrite{Freezes: []Freeze{{Level: period.Total}}})
	assert.ErrorContains(t, err, "cannot be frozen")
	err = s.WriteComputed(ctx, ComputedWrite{Rows: []PeriodRow{{Key: "nope"}}})
	assert.ErrorIs(t, err, ErrUnknownKey)
}

func TestWritePoll_UnknownKeysAreRejections(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t, day("2026-08-05"))
	err := s.WritePoll(ctx, PollWrite{
		Status: []StatusRow{{Key: "nope"}},
		Close:  []DayClose{{Key: "nope"}},
	})
	require.ErrorIs(t, err, ErrUnknownKey)
	assert.True(t, IsRejection(err))
}

// Cutover failures inside the transaction leave the meta state unchanged.
func TestEnsureCutover_StatementErrors(t *testing.T) {
	t.Parallel()
	now := day("2026-08-05")
	s, _, _ := newStore(t, now)
	insertRows(t, s, `INSERT INTO monthly_values (month, register_key, value, raw_value)
		VALUES ('2026-08', 'pv_energy_monthly', 5, 5)`)
	// The offset row (offset:...) is the only meta insert that fails.
	insertRows(t, s, `CREATE TRIGGER fail_offset BEFORE INSERT ON meta
		WHEN NEW.key LIKE 'offset:%' BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	_, _, err := s.EnsureCutover(ctx, period.Of(now))
	require.ErrorContains(t, err, "write meta offset:")
	st, err := s.CloseState(ctx)
	require.NoError(t, err)
	assert.Empty(t, st.Cutover, "nothing recorded")

	s2, _, _ := newStore(t, now)
	insertRows(t, s2, `CREATE TRIGGER fail_mark BEFORE INSERT ON meta
		WHEN NEW.key LIKE 'closed:daily:%' BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	_, _, err = s2.EnsureCutover(ctx, period.Of(now))
	require.ErrorContains(t, err, "write meta closed:daily:")

	s3, _, _ := newStore(t, now)
	insertRows(t, s3, `CREATE TRIGGER fail_by BEFORE INSERT ON meta
		WHEN NEW.key = 'baseline_year' BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	_, _, err = s3.EnsureCutover(ctx, period.Of(now))
	require.ErrorContains(t, err, "write meta baseline_year")

	s4, _, _ := newStore(t, now)
	insertRows(t, s4, `CREATE TRIGGER fail_month BEFORE INSERT ON meta
		WHEN NEW.key = 'closed:monthly' BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	_, _, err = s4.EnsureCutover(ctx, period.Of(now))
	require.ErrorContains(t, err, "write meta closed:monthly")
}

func TestEnsureCutover_InvalidPeriod(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t, day("2026-08-05"))
	_, _, err := s.EnsureCutover(ctx, period.Period{Day: "bad"})
	assert.Error(t, err)
	_, _, err = s.EnsureCutover(ctx, period.Period{Day: "2026-08-05", Month: "x", Year: "2026"})
	assert.Error(t, err, "invalid month")
	_, _, err = s.EnsureCutover(ctx, period.Period{Day: "2026-08-05", Month: "2026-08", Year: "x"})
	assert.Error(t, err, "invalid year")
}

// Statements run after the context was cancelled fail inside the backfill transaction.
func TestBackfillTx_CancelledStatements(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t, day("2026-09-27"))
	_, _, err := s.EnsureCutover(ctx, period.Of(day("2026-09-27")))
	require.NoError(t, err)
	// An offset to clear: PutPeriod deletes it first.
	insertRows(t, s, `INSERT INTO meta (key, value) VALUES ('offset:`+
		OffsetKey(period.Monthly, "2026-09", "pv_energy_monthly")+`', '1')`)
	require.NoError(t, s.loadMeta(ctx))
	cctx, cancel := context.WithCancel(ctx)
	_ = s.Backfill(ctx, func(tx BackfillTx) error {
		cancel()
		tx.(*backfillTx).ctx = cctx
		_, err := tx.SumDaily([]string{"pv_energy_daily"}, "", "2026-09-30")
		assert.Error(t, err)
		_, _, err = tx.PeriodValue(period.Monthly, "pv_energy_monthly", "2026-09")
		assert.ErrorContains(t, err, "storage: read")
		err = tx.PutPeriod(period.Monthly, "pv_energy_monthly", "2026-09", 1)
		assert.ErrorContains(t, err, "clear offset")
		assert.ErrorIs(t, tx.PutPeriod(period.Monthly, "nope", "2026-09", 1), ErrUnknownKey)
		return nil
	})
}

func TestCleanupAll_StatementErrors(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t, day("2026-08-05"))
	s.cfg.DailyRetention = 24 * time.Hour
	_, _, err := s.EnsureCutover(ctx, period.Of(day("2026-08-05")))
	require.NoError(t, err)
	insertRows(t, s, `INSERT INTO daily_values (date, register_key, value, raw_value)
		VALUES ('2020-01-01', 'pv_energy_daily', 1, 1)`)
	failOn(t, s, "daily_values", "DELETE")
	assert.ErrorContains(t, s.CleanupAll(ctx), "cleanup daily_values")

	s2, _, _ := newStore(t, day("2026-08-05"))
	_, _, err = s2.EnsureCutover(ctx, period.Of(day("2026-08-05")))
	require.NoError(t, err)
	insertRows(t, s2, `INSERT INTO daily_values (date, register_key, value, raw_value)
		VALUES ('2020-01-01', 'pv_energy_daily', 1, 1)`)
	failOn(t, s2, "meta", "INSERT")
	assert.ErrorContains(t, s2.CleanupAll(ctx), "write meta purged_before")
}

func TestFirstOpenDay_InvalidMarks(t *testing.T) {
	t.Parallel()
	for _, lvl := range []period.Level{period.Daily, period.Monthly, period.Yearly} {
		s, _, _ := newStore(t, day("2026-08-05"))
		s.meta.frozen = map[period.Level]string{
			period.Daily: "2026-08-01", period.Monthly: "2026-07", period.Yearly: "2025",
		}
		s.meta.baselineYear = "2025"
		s.meta.frozen[lvl] = "bad"
		_, err := s.firstOpenDay()
		assert.Error(t, err, lvl.String())
		assert.Error(t, s.CleanupAll(ctx), lvl.String())
	}
}

func TestOpenSpans_InvalidMarks(t *testing.T) {
	t.Parallel()
	p := period.Of(day("2026-08-05"))
	m := newMetaState()
	m.frozen[period.Monthly] = "bad"
	_, err := openSpans(&m, p)
	assert.Error(t, err)
	m.frozen[period.Monthly] = "2026-06"
	m.frozen[period.Yearly] = "bad"
	_, err = openSpans(&m, p)
	assert.Error(t, err)
}

func TestReads_ScanErrors(t *testing.T) {
	t.Parallel()
	s, _, _ := newStore(t, day("2026-08-05"))
	// Text where a number is scanned fails the scan, not the query.
	insertRows(t, s,
		`INSERT INTO daily_values (date, register_key, value, raw_value)
			VALUES ('2026-08-05', 'pv_energy_daily', 'x', 'x')`,
		`INSERT INTO error_data (timestamp, register_key, raw_value)
			VALUES ('2026-08-05T10:00:00.000Z', 'solis_status', 'x')`,
		`INSERT INTO total_values (register_key, value, raw_value, timestamp)
			VALUES ('pv_energy_total', 'x', 'x', '2026-08-05T10:00:00Z')`)
	keys := []string{"pv_energy_daily"}
	from, to := day("2026-08-01"), day("2026-08-05")
	_, err := s.SumDaily(ctx, keys, "2026-08-01", "2026-08-05")
	assert.NoError(t, err, "SUM of text is 0, not an error")
	_, err = s.Seed(ctx, []string{"2026-08-05"})
	assert.ErrorContains(t, err, "seed daily")
	_, err = s.GetDailyValues(ctx, keys, from, to)
	assert.ErrorContains(t, err, "scan daily values")
	_, err = s.GetDailyHistory(ctx, "pv_energy_daily", from, to)
	assert.ErrorContains(t, err, "scan daily history")
	_, err = s.GetTotalHistory(ctx, "pv_energy_total")
	assert.ErrorContains(t, err, "query total")
	insertRows(t, s, `DELETE FROM daily_values`)
	_, err = s.Seed(ctx, nil)
	assert.ErrorContains(t, err, "seed status")
}
