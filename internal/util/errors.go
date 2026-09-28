package util

import "fmt"

// DependencyError names the nil or invalid dependency a constructor rejected; Err is the
// constructor package's sentinel (e.g. poller.ErrMissingDependency).
type DependencyError struct {
	// Dependency is the Deps field that is missing or invalid.
	Dependency string
	// Err is the package sentinel.
	Err error
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("%v: %s", e.Err, e.Dependency)
}

// Unwrap returns the package sentinel.
func (e *DependencyError) Unwrap() error { return e.Err }

// Requirement is one named dependency check for RequireAll.
type Requirement struct {
	// Name is the Deps field name.
	Name string
	// OK reports whether the dependency is present and valid.
	OK bool
}

// RequireAll returns a DependencyError wrapping sentinel for the first failed requirement.
func RequireAll(sentinel error, reqs ...Requirement) error {
	for _, r := range reqs {
		if !r.OK {
			return &DependencyError{Dependency: r.Name, Err: sentinel}
		}
	}
	return nil
}

// DurationError reports input that ParseDuration rejected.
type DurationError struct {
	// Input is the rejected text ("" when it was empty).
	Input string
}

func (e *DurationError) Error() string {
	if e.Input == "" {
		return fmt.Sprintf("%v: empty", ErrInvalidDuration)
	}
	return fmt.Sprintf("%v: %q", ErrInvalidDuration, e.Input)
}

// Unwrap returns ErrInvalidDuration.
func (e *DurationError) Unwrap() error { return ErrInvalidDuration }
