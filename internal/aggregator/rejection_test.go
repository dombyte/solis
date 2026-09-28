package aggregator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/storage"
)

func TestJoinedErrors(t *testing.T) {
	closed := &storage.PeriodClosedError{Level: "monthly", Key: "k", Period: "2026-08"}
	domain := &storage.WriteDomainError{Key: "k", Reason: "not computed"}
	assert.Equal(t, []error{closed, domain}, joinedErrors(errors.Join(closed, domain)))
	single := fmt.Errorf("write: %w", closed)
	assert.Equal(t, []error{single}, joinedErrors(single))
}

// rejectingStore commits the real write, then reports rejected rows on top, as storage
// does when some rows of a committed run were skipped.
type rejectingStore struct {
	Store
	rejected error
}

func (r *rejectingStore) WriteComputed(ctx context.Context, w storage.ComputedWrite) error {
	if err := r.Store.WriteComputed(ctx, w); err != nil {
		return err
	}
	return r.rejected
}

// A partly rejected run committed its data: it merges the cache and stays healthy
// instead of reporting Recovering on every run (review AGG-M3). The reporter mock only
// allows Healthy here, so a Recovering report fails the test.
func TestRun_RejectedRowsStillMergeAndStayHealthy(t *testing.T) {
	for name, rejected := range map[string]error{
		"unknown key": fmt.Errorf("storage: %w: zz", storage.ErrUnknownKey),
		"lag and domain": errors.Join(
			&storage.PeriodClosedError{Level: "monthly", Key: "k", Period: "2026-07"},
			&storage.WriteDomainError{Key: "k", Reason: "not computed"}),
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, at("2026-08-10 12:00"), at("2026-08-01 12:00"))
			e.daily("pv_energy_daily", "2026-08-09", 12.5)
			e.agg.d.Store = &rejectingStore{Store: e.st, rejected: rejected}
			e.agg.run(bg)
			assert.Equal(t, health.Healthy, e.agg.State())
			assert.Equal(t, int32(1), e.cache.merges.Load())
			v := e.cache.Get("pv_energy_monthly")
			require.NotNil(t, v)
			assert.InDelta(t, 12.5, v.DecodedValue, 1e-9)
		})
	}
}
