//go:build !unix

package maintenance

import (
	"errors"
	"path/filepath"
)

// Lock is unsupported on this platform (releases target linux and darwin).
type Lock struct{}

var errUnsupported = errors.New("maintenance: file locking is only supported on unix")

// LockPath returns the lock file path for a database path.
func LockPath(dbPath string) string {
	// Resolve a symlinked database so the server and a job lock the same file, next to
	// the real database (a missing file keeps the given path).
	if resolved, err := filepath.EvalSymlinks(dbPath); err == nil {
		dbPath = resolved
	}
	return dbPath + ".lock"
}

// AcquireShared is unsupported on this platform.
func AcquireShared(string) (*Lock, error) { return nil, errUnsupported }

// AcquireExclusive is unsupported on this platform.
func AcquireExclusive(string) (*Lock, error) { return nil, errUnsupported }

// Release is a no-op.
func (l *Lock) Release() error { return nil }
