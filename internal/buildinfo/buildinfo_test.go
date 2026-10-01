package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfo_String(t *testing.T) {
	i := Info{Version: "3.1.0", Commit: "abc1234", BuildDate: "2026-10-01", GoVersion: "go1.26"}
	assert.Equal(t, "solis 3.1.0 (commit abc1234, built 2026-10-01, go1.26)", i.String())
}
