package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/storage/mocks"
)

// errWriter fails every write after ok successful ones.
type errWriter struct{ ok int }

func (w *errWriter) Write(p []byte) (int, error) {
	if w.ok == 0 {
		return 0, errors.New("write failed")
	}
	w.ok--
	return len(p), nil
}

func TestWriteHeaderAndReport_WriteErrors(t *testing.T) {
	t.Parallel()
	assert.Error(t, writeHeader(&errWriter{}, "b", false))
	assert.Error(t, writeHeader(&errWriter{ok: 1}, "b", true))
	assert.NoError(t, writeHeader(&errWriter{ok: 2}, "b", true))
	assert.Error(t, Report{}.Write(&errWriter{}))
}

func TestRunBackfill_HeaderWriteError(t *testing.T) {
	f := newFixture(t)
	env := f.env(t, okBackup)
	env.Out = &errWriter{}
	assert.ErrorContains(t, RunBackfill(bg, env, Options{}), "write failed")
}

func TestAcquire_Errors(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	_, err := AcquireExclusive(filepath.Join(file, "sub", "solis.db"))
	assert.ErrorContains(t, err, "lock dir")

	dir := t.TempDir()
	// A directory where the lock file should be cannot be opened read-write.
	require.NoError(t, os.Mkdir(LockPath(filepath.Join(dir, "solis.db")), 0o750))
	_, err = AcquireExclusive(filepath.Join(dir, "solis.db"))
	assert.ErrorContains(t, err, "open lock file")

	var l *Lock
	assert.NoError(t, l.Release(), "a nil lock releases nothing")
}

func TestRecompute_StoreErrors(t *testing.T) {
	t.Parallel()
	reg := newFixture(t).reg
	boom := errors.New("boom")
	tests := map[string]func(tx *mocks.MockBackfillTx){
		"sum": func(tx *mocks.MockBackfillTx) {
			tx.EXPECT().SumDaily(mock.Anything, mock.Anything, mock.Anything).Return(nil, boom)
		},
		"read": func(tx *mocks.MockBackfillTx) {
			tx.EXPECT().SumDaily(mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]float64{}, nil)
			tx.EXPECT().PeriodValue(mock.Anything, mock.Anything, mock.Anything).
				Return(0, false, boom)
		},
		"write": func(tx *mocks.MockBackfillTx) {
			tx.EXPECT().SumDaily(mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]float64{}, nil)
			tx.EXPECT().PeriodValue(mock.Anything, mock.Anything, mock.Anything).
				Return(0, false, nil)
			tx.EXPECT().PutPeriod(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(boom)
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tx := mocks.NewMockBackfillTx(t)
			tx.EXPECT().PurgedBefore().Return("")
			tx.EXPECT().FirstDailyDay().Return("2020-01-01", nil)
			setup(tx)
			_, err := Recompute(tx, reg, period.Of(now), Options{})
			assert.ErrorIs(t, err, boom)
		})
	}
}

func TestRefreshBaseline_SumError(t *testing.T) {
	t.Parallel()
	reg := newFixture(t).reg
	tx := mocks.NewMockBackfillTx(t)
	tx.EXPECT().Baseline().Return("2025", nil)
	tx.EXPECT().Cutover().Return("2024-03-01")
	tx.EXPECT().SumDaily(mock.Anything, "2024-01-01", "2025-12-31").Return(nil, errors.New("boom"))
	r := &recomputer{tx: tx, reg: reg, now: period.Of(now)}
	assert.ErrorContains(t, r.refreshBaseline(), "boom")
}

func TestCheckPurged_InvalidYear(t *testing.T) {
	t.Parallel()
	tx := mocks.NewMockBackfillTx(t)
	tx.EXPECT().PurgedBefore().Return("2020-01-01")
	assert.ErrorIs(t, checkPurged(tx, period.Period{Year: "x"}, Options{}), period.ErrInvalidKey)
}

func TestMonthsOf_InvalidYear(t *testing.T) {
	t.Parallel()
	_, err := monthsOf("x", period.Of(now))
	assert.Error(t, err)
	r := &recomputer{now: period.Of(now)}
	assert.Error(t, r.year("x"))
}
