package aggregator

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dombyte/solis/internal/storage"
)

func TestIsLag(t *testing.T) {
	closed := &storage.PeriodClosedError{Level: "monthly", Key: "k", Period: "2026-08"}
	domain := &storage.WriteDomainError{Key: "k", Reason: "not computed"}
	assert.True(t, isLag(closed))
	assert.True(t, isLag(fmt.Errorf("write: %w", closed)))
	assert.False(t, isLag(domain))
	assert.False(t, isLag(errors.Join(closed, domain)), "a domain violation is a real failure")
	assert.False(t, isLag(errors.New("disk full")))
}
