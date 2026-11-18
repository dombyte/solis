package aggregator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/storage"
)

// recordingStore keeps every write the aggregator sends.
type recordingStore struct {
	Store
	writes []storage.ComputedWrite
}

func (r *recordingStore) WriteComputed(ctx context.Context, w storage.ComputedWrite) error {
	r.writes = append(r.writes, w)
	return r.Store.WriteComputed(ctx, w)
}

// A run that computes the same values as the previous one must not write at all, and a
// changed daily value must write only the rows it affects.
func TestExecute_WritesOnlyChangedRows(t *testing.T) {
	now := at("2026-08-10 12:00")
	e := newEnv(t, now, now)
	rec := &recordingStore{Store: e.st}
	e.agg.d.Store = rec
	e.daily("pv_energy_daily", "2026-08-10", 4.2)

	require.NoError(t, e.agg.execute(bg, now))
	require.Len(t, rec.writes, 1)
	first := len(rec.writes[0].Rows)
	require.Positive(t, first)

	require.NoError(t, e.agg.execute(bg, now.Add(iv)))
	assert.Len(t, rec.writes, 1, "identical values: no transaction")
	assert.EqualValues(t, 2, e.cache.merges.Load(), "the run completes")
	assert.Zero(t, e.cache.last.Load(), "identical values: nothing merged, no cache event")

	e.daily("pv_energy_daily", "2026-08-10", 4.3)
	require.NoError(t, e.agg.execute(bg, now.Add(2*iv)))
	require.Len(t, rec.writes, 2)
	assert.EqualValues(t, 3, e.cache.last.Load(), "only the changed values are merged")
	var keys []string
	for _, r := range rec.writes[1].Rows {
		keys = append(keys, r.Key)
	}
	assert.ElementsMatch(t, []string{
		"pv_energy_monthly", "pv_energy_yearly", "pv_energy_total",
	}, keys)
	assert.Less(t, len(keys), first)
}
