package poller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
)

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
