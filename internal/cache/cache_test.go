package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/eventbus/mocks"
	"github.com/dombyte/solis/internal/solis"
)

var t0 = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func val(v float64) *solis.Value { return &solis.Value{DecodedValue: v} }

func TestReplaceDomain_KeepsOtherDomains(t *testing.T) {
	c := New(eventbus.New())
	c.ReplaceDomain(eventbus.DomainPoller, map[string]*solis.Value{"a": val(1), "b": val(2)}, t0)
	c.Merge(eventbus.DomainAggregator, map[string]*solis.Value{"m": val(9)}, t0)

	// Next poll no longer contains "b": it is removed, the aggregator key survives.
	c.ReplaceDomain(eventbus.DomainPoller, map[string]*solis.Value{"a": val(3)}, t0.Add(time.Second))

	assert.InDelta(t, 3.0, c.Get("a").DecodedValue, 0)
	assert.Nil(t, c.Get("b"))
	assert.InDelta(t, 9.0, c.Get("m").DecodedValue, 0)
	assert.Equal(t, 2, c.Size())
	assert.Equal(t, t0.Add(time.Second), c.LastUpdate(eventbus.DomainPoller))
	assert.Equal(t, t0, c.LastUpdate(eventbus.DomainAggregator))
	assert.True(t, c.LastUpdate("none").IsZero())
}

func TestMerge_DoesNotRemove(t *testing.T) {
	c := New(eventbus.New())
	c.Merge(eventbus.DomainAggregator, map[string]*solis.Value{"m": val(1)}, t0)
	c.Merge(eventbus.DomainAggregator, map[string]*solis.Value{"y": val(2)}, t0)
	got := c.GetMultiple([]string{"m", "y", "missing"})
	assert.Len(t, got, 2)
}

func TestWritesPublishEvents(t *testing.T) {
	pub := mocks.NewMockPublisher(t)
	pub.EXPECT().Publish(mock.MatchedBy(func(e eventbus.Event) bool {
		return e.Kind == eventbus.ValuesUpdated && e.Domain == eventbus.DomainPoller &&
			assert.ObjectsAreEqual([]string{"a", "b"}, e.Keys) && e.At.Equal(t0)
	})).Once()
	pub.EXPECT().Publish(mock.MatchedBy(func(e eventbus.Event) bool {
		return e.Domain == eventbus.DomainAggregator
	})).Once()

	c := New(pub)
	c.ReplaceDomain(eventbus.DomainPoller, map[string]*solis.Value{"b": val(1), "a": val(2)}, t0)
	c.Merge(eventbus.DomainAggregator, map[string]*solis.Value{"m": val(1)}, t0)
}

func TestEventsReachBusSubscriber(t *testing.T) {
	bus := eventbus.New()
	ch, cancel, err := bus.Subscribe("test", 4, eventbus.Coalesce)
	require.NoError(t, err)
	defer cancel()

	c := New(bus)
	c.Merge(eventbus.DomainAggregator, map[string]*solis.Value{"m": val(1)}, t0)
	select {
	case e := <-ch:
		assert.Equal(t, []string{"m"}, e.Keys)
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New(eventbus.New())
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 200 {
				c.ReplaceDomain(eventbus.DomainPoller, map[string]*solis.Value{"a": val(float64(i))}, t0)
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				_ = c.Get("a")
				_ = c.GetMultiple([]string{"a"})
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, c.Size())
}
