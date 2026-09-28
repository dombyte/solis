package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/util"
)

// ShutdownTimeout bounds the whole shutdown (stopping components, the HTTP server, the
// periodic database jobs and closing storage) once it has begun. It covers the worst
// case of every component using its full health.StopTimeout, with margin.
const ShutdownTimeout = 30 * time.Second

// ErrShutdownTimeout means the shutdown did not finish in time. The caller must exit
// the process: whatever is still running can only be ended by the process exit.
var ErrShutdownTimeout = errors.New("shutdown timed out")

// awaitShutdown returns the result of the application run on done. Once stopping is
// closed (a signal or a fatal health escalation began the shutdown) it waits at most
// timeout for that result, and returns ErrShutdownTimeout otherwise.
func awaitShutdown(done <-chan error, stopping <-chan struct{}, clock util.Clock,
	timeout time.Duration,
) error {
	select {
	case err := <-done:
		return err
	case <-stopping:
	}
	timer := clock.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C():
		return fmt.Errorf("app: %w after %s", ErrShutdownTimeout, timeout)
	}
}
