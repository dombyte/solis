package eventbus

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recv(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("no event received")
		return Event{}
	}
}

func empty(ch <-chan Event) bool {
	select {
	case <-ch:
		return false
	case <-time.After(20 * time.Millisecond):
		return true
	}
}

func TestSubscribePublishUnsubscribe(t *testing.T) {
	b := New()
	ch, cancel, err := b.Subscribe("a", 4, Coalesce)
	require.NoError(t, err)

	b.Publish(Event{Kind: ValuesUpdated, Domain: DomainPoller, Keys: []string{"k"}})
	e := recv(t, ch)
	assert.Equal(t, DomainPoller, e.Domain)
	assert.Equal(t, []string{"k"}, e.Keys)

	cancel()
	b.Publish(Event{Kind: ValuesUpdated})
	assert.True(t, empty(ch))
	cancel() // idempotent
}

func TestPublishNeverBlocksOnStuckConsumer(t *testing.T) {
	b := New()
	_, cancel, err := b.Subscribe("stuck", 1, Coalesce)
	require.NoError(t, err)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for range 1000 {
			b.Publish(Event{Kind: ValuesUpdated})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber")
	}
	assert.Equal(t, int64(999), b.Dropped())
}

func TestCoalesceKeepsPeriodClosed(t *testing.T) {
	b := New()
	ch, cancel, err := b.Subscribe("agg", 1, Coalesce)
	require.NoError(t, err)
	defer cancel()

	b.Publish(Event{Kind: ValuesUpdated})
	b.Publish(Event{Kind: ValuesUpdated}) // dropped
	b.Publish(Event{Kind: PeriodClosed, Day: "2026-08-05"})

	assert.Equal(t, ValuesUpdated, recv(t, ch).Kind)
	e := recv(t, ch)
	assert.Equal(t, PeriodClosed, e.Kind)
	assert.Equal(t, "2026-08-05", e.Day)
	assert.Equal(t, int64(1), b.Dropped())
}

func TestLosslessDeliversEverythingInOrder(t *testing.T) {
	b := New()
	ch, cancel, err := b.Subscribe("lossless", 1, Lossless)
	require.NoError(t, err)
	defer cancel()

	const n = 50
	for i := range n {
		b.Publish(Event{Kind: ValuesUpdated, Keys: []string{string(rune('a' + i%26))}})
	}
	for i := range n {
		assert.Equal(t, string(rune('a'+i%26)), recv(t, ch).Keys[0])
	}
	assert.Zero(t, b.Dropped())
}

func TestConcurrentPublishers(t *testing.T) {
	b := New()
	ch, cancel, err := b.Subscribe("c", 1024, Lossless)
	require.NoError(t, err)
	defer cancel()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				b.Publish(Event{Kind: PeriodClosed})
			}
		}()
	}
	wg.Wait()
	for range 800 {
		recv(t, ch)
	}
}

func TestClose(t *testing.T) {
	b := New()
	_, cancel, err := b.Subscribe("x", 0, Coalesce)
	require.NoError(t, err)
	assert.False(t, b.Closed())
	b.Close()
	assert.True(t, b.Closed())
	b.Publish(Event{}) // no-op, no panic
	cancel()
	_, _, err = b.Subscribe("y", 1, Coalesce)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestKindString(t *testing.T) {
	assert.Equal(t, "ValuesUpdated", ValuesUpdated.String())
	assert.Equal(t, "PeriodClosed", PeriodClosed.String())
	assert.Equal(t, "unknown", Kind(99).String())
}
