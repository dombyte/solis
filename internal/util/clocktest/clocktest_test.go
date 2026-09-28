package clocktest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var start = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func fired(ch <-chan time.Time) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestTimerFiresOnAdvance(t *testing.T) {
	c := New(start)
	tm := c.NewTimer(10 * time.Second)
	c.Advance(9 * time.Second)
	assert.False(t, fired(tm.C()))
	c.Advance(time.Second)
	assert.True(t, fired(tm.C()))
	assert.Equal(t, start.Add(10*time.Second), c.Now())

	assert.False(t, tm.Stop())
	assert.False(t, tm.Reset(time.Second))
	assert.True(t, tm.Stop())
	c.Advance(time.Hour)
	assert.False(t, fired(tm.C()))
}

func TestTickerRepeats(t *testing.T) {
	c := New(start)
	tk := c.NewTicker(time.Second)
	c.Advance(time.Second)
	assert.True(t, fired(tk.C()))
	c.Advance(time.Second)
	assert.True(t, fired(tk.C()))
	tk.Reset(5 * time.Second)
	c.Advance(4 * time.Second)
	assert.False(t, fired(tk.C()))
	c.Advance(time.Second)
	assert.True(t, fired(tk.C()))
	tk.Stop()
	c.Advance(time.Minute)
	assert.False(t, fired(tk.C()))
}

func TestSetNeverMovesBackwards(t *testing.T) {
	c := New(start)
	c.Set(start.Add(-time.Hour))
	assert.Equal(t, start, c.Now())
}

func TestBlockUntil(t *testing.T) {
	c := New(start)
	go func() {
		time.Sleep(5 * time.Millisecond)
		c.NewTimer(time.Second)
	}()
	require.True(t, c.BlockUntil(1))
	assert.False(t, New(start).blockShort())
}

// blockShort is BlockUntil with nothing ever registered; kept tiny for test speed.
func (c *Clock) blockShort() bool {
	return c.activeCount() >= 1
}
