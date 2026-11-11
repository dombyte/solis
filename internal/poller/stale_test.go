package poller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
)

// A dashboard must not show the last power reading as current while the inverter cannot
// be read: after staleAfter failed cycles the live keys leave the cache, the daily
// counters stay, and the next good poll brings everything back.
func TestPoll_FailedPollsDropLiveValues(t *testing.T) {
	start := time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local)
	e := newEnv(t, start)
	e.dev.set(33035, 250)  // pv_energy_daily 25.0
	e.dev.set(33058, 5230) // pv_total_power
	e.dev.set(33095, 3)    // solis_status
	e.p.pollCycle(bg, start)
	require.NotNil(t, e.cache.Get("pv_total_power"))

	e.dev.connected.Store(false)
	for i := 1; i < staleAfter; i++ {
		e.p.pollCycle(bg, start.Add(time.Duration(i)*pollEvery))
		require.NotNil(t, e.cache.Get("pv_total_power"), "failure %d: still tolerated", i)
	}
	e.p.pollCycle(bg, start.Add(staleAfter*pollEvery))
	assert.Nil(t, e.cache.Get("pv_total_power"))
	assert.Nil(t, e.cache.Get("solis_status"))
	if v := e.cache.Get("pv_energy_daily"); assert.NotNil(t, v, "daily counters stay") {
		assert.InDelta(t, 25.0, v.DecodedValue, 1e-9)
	}
	replaced := e.polls.Load()
	e.p.pollCycle(bg, start.Add((staleAfter+1)*pollEvery))
	assert.Equal(t, replaced, e.polls.Load(), "dropped once, not on every failed cycle")

	e.dev.connected.Store(true)
	e.p.pollCycle(bg, start.Add((staleAfter+2)*pollEvery))
	assert.NotNil(t, e.cache.Get("pv_total_power"))
	assert.Zero(t, e.p.failures)
}

// A link that is up but does not answer (RTU: the serial port opens without an inverter)
// must still write the due day closes.
func TestPoll_ReadFailureStillClosesDays(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.Local)
	e := newEnv(t, now)
	_, _, err := e.st.EnsureCutover(bg, period.Of(time.Date(2026, 8, 2, 12, 0, 0, 0, time.Local)))
	require.NoError(t, err)
	e.dev.failNext.Store(1)
	e.dev.dropOnFail.Store(true) // fail without waiting for the block retry delay
	e.p.pollCycle(bg, now)
	assert.Positive(t, e.dev.reads.Load(), "the link was up and read")
	st, err := e.st.CloseState(bg)
	require.NoError(t, err)
	assert.Equal(t, "2026-08-05", st.ClosedThrough)
}
