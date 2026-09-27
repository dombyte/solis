//go:build unix

package maintenance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	lockDirPerm  = 0o750
	lockFilePerm = 0o600
)

// Lock is an advisory flock on the lock file next to the database.
type Lock struct {
	f *os.File
}

// LockPath returns the lock file path for a database path.
func LockPath(dbPath string) string { return dbPath + ".lock" }

// AcquireShared takes the shared lock held by a running server; it fails with ErrLocked
// while a maintenance job holds the exclusive lock.
func AcquireShared(dbPath string) (*Lock, error) {
	return acquire(dbPath, syscall.LOCK_SH)
}

// AcquireExclusive takes the exclusive lock of a maintenance job; it fails with ErrLocked
// while the server (or another job) holds the lock.
func AcquireExclusive(dbPath string) (*Lock, error) {
	return acquire(dbPath, syscall.LOCK_EX)
}

func acquire(dbPath string, how int) (*Lock, error) {
	path := LockPath(dbPath)
	if err := os.MkdirAll(filepath.Dir(path), lockDirPerm); err != nil {
		return nil, fmt.Errorf("maintenance: lock dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_RDWR, lockFilePerm)
	if err != nil {
		return nil, fmt.Errorf("maintenance: open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB); err != nil { // #nosec G115
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %s", ErrLocked, path)
		}
		return nil, fmt.Errorf("maintenance: flock %s: %w", path, err)
	}
	return &Lock{f: f}, nil
}

// Release drops the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN) // #nosec G115
	return errors.Join(err, l.f.Close())
}
