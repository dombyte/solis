package app

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/util/clocktest"
)

func TestAwaitShutdown_ReturnsResultBeforeShutdownBegins(t *testing.T) {
	done := make(chan error, 1)
	errStartup := errors.New("startup failed")
	done <- errStartup
	err := awaitShutdown(done, make(chan struct{}), clocktest.New(time.Now()), time.Second)
	assert.ErrorIs(t, err, errStartup)
}

func TestAwaitShutdown_ReturnsResultWithinDeadline(t *testing.T) {
	done, stopping := make(chan error, 1), make(chan struct{})
	close(stopping)
	done <- nil
	assert.NoError(t, awaitShutdown(done, stopping, clocktest.New(time.Now()), time.Second))
}

// A shutdown that hangs (e.g. a component Stop or a backup ignoring its context) must
// not keep the process alive: after the deadline the caller gets ErrShutdownTimeout and
// exits non-zero.
func TestAwaitShutdown_HungShutdownTimesOut(t *testing.T) {
	clk := clocktest.New(time.Now())
	done, stopping := make(chan error), make(chan struct{})
	res := make(chan error, 1)
	go func() { res <- awaitShutdown(done, stopping, clk, ShutdownTimeout) }()
	close(stopping)
	require.True(t, clk.BlockUntil(1), "deadline armed once the shutdown began")
	clk.Advance(ShutdownTimeout - time.Millisecond)
	select {
	case err := <-res:
		t.Fatalf("returned before the deadline: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	clk.Advance(time.Millisecond)
	select {
	case err := <-res:
		assert.ErrorIs(t, err, ErrShutdownTimeout)
	case <-time.After(time.Second):
		t.Fatal("deadline did not fire")
	}
}
