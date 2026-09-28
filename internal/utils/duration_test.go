package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"1d", Day},
		{"1w", Week},
		{"1y", Year},
		{"1.5d", 36 * time.Hour},
		{"1y2w3d4h", Year + 2*Week + 3*Day + 4*time.Hour},
		{"8760h", 8760 * time.Hour},
		{"30s", 30 * time.Second},
		{"1h30m", 90 * time.Minute},
		{" 2d ", 2 * Day},
		{"0s", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDuration(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	for _, in := range []string{"", "d", "1", "-1d", "1x", "1..2d", "1d-2h"} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseDuration(in)
			assert.ErrorIs(t, err, ErrInvalidDuration)
		})
	}
}
