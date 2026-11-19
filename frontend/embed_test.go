package frontend

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Dist works whether or not dist/ was built (CI runs the Go tests without npm); an unbuilt
// tree is just empty.
func TestDist(t *testing.T) {
	t.Parallel()
	fsys, err := Dist()
	require.NoError(t, err)
	require.NotNil(t, fsys)
}
