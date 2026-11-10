package app

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dombyte/solis/internal/health"
)

// One slow ping (the single connection is busy with a VACUUM) must not stop the app;
// only a storage that keeps failing is fatal.
func TestPingTolerance(t *testing.T) {
	t.Parallel()
	p := pingTolerance{limit: storageProbeFailures}
	boom := errors.New("context deadline exceeded")

	st, reason := p.observe(nil)
	assert.Equal(t, health.Healthy, st)
	assert.Empty(t, reason)

	st, reason = p.observe(boom)
	assert.Equal(t, health.Recovering, st)
	assert.Equal(t, "ping failed 1 of 3 times: context deadline exceeded", reason)
	st, _ = p.observe(boom)
	assert.Equal(t, health.Recovering, st)

	st, _ = p.observe(nil)
	assert.Equal(t, health.Healthy, st, "a good ping resets the count")
	for range storageProbeFailures - 1 {
		st, _ = p.observe(boom)
		assert.Equal(t, health.Recovering, st)
	}
	st, reason = p.observe(boom)
	assert.Equal(t, health.Failed, st)
	assert.Contains(t, reason, "3 of 3")
}
