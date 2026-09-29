package util

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireAll(t *testing.T) {
	errMissing := errors.New("pkg: missing dependency")
	assert.NoError(t, RequireAll(errMissing, Requirement{Name: "A", OK: true}))

	err := RequireAll(errMissing,
		Requirement{Name: "A", OK: true},
		Requirement{Name: "B", OK: false},
		Requirement{Name: "C", OK: false},
	)
	require.ErrorIs(t, err, errMissing)
	var de *DependencyError
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "B", de.Dependency, "first failed requirement wins")
	assert.EqualError(t, err, "pkg: missing dependency: B")
}
