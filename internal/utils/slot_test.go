package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSlot(t *testing.T) {
	var s Slot[string]
	_, ok := s.Load()
	assert.False(t, ok)

	s.Store("a")
	v, ok := s.Load()
	assert.True(t, ok)
	assert.Equal(t, "a", v)

	s.Store("b")
	v, _ = s.Load()
	assert.Equal(t, "b", v)

	s.Clear()
	_, ok = s.Load()
	assert.False(t, ok)
}

func TestSlot_ClearIf(t *testing.T) {
	var s Slot[string]
	s.ClearIf(func(string) bool { return true }) // empty slot: no-op
	s.Store("new")
	s.ClearIf(func(v string) bool { return v == "old" })
	v, ok := s.Load()
	assert.True(t, ok, "a stale owner never clears its successor")
	assert.Equal(t, "new", v)
	s.ClearIf(func(v string) bool { return v == "new" })
	_, ok = s.Load()
	assert.False(t, ok)
}

func TestRealClock(t *testing.T) {
	c := NewRealClock()
	assert.WithinDuration(t, time.Now(), c.Now(), time.Second)

	tm := c.NewTimer(time.Millisecond)
	<-tm.C()
	assert.False(t, tm.Stop())
	tm.Reset(time.Millisecond)
	<-tm.C()

	tk := c.NewTicker(time.Millisecond)
	<-tk.C()
	tk.Reset(time.Millisecond)
	<-tk.C()
	tk.Stop()
}
