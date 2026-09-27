// Package cache holds the latest register values. It is created once and never
// recreated; every write publishes a ValuesUpdated event on the injected bus so
// consumers (WebSocket hub, aggregator) never need a reference to the writers.
package cache

import (
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/solis"
)

// Cache is a thread-safe map of the latest values, partitioned into write domains.
type Cache struct {
	mu      sync.RWMutex
	data    map[string]*solis.Value
	owner   map[string]string    // key -> domain
	updated map[string]time.Time // domain -> last write
	pub     eventbus.Publisher
	log     zerolog.Logger
}

// New creates an empty cache publishing on pub with the injected logger.
func New(pub eventbus.Publisher, log zerolog.Logger) *Cache {
	return &Cache{
		data:    make(map[string]*solis.Value),
		owner:   make(map[string]string),
		updated: make(map[string]time.Time),
		pub:     pub,
		log:     log,
	}
}

// ReplaceDomain atomically replaces all keys owned by domain with values: keys of the
// domain that are absent from values are removed; other domains are untouched.
// Values must not be mutated after the call.
func (c *Cache) ReplaceDomain(domain string, values map[string]*solis.Value, at time.Time) {
	c.mu.Lock()
	removed := 0
	for k, d := range c.owner {
		if d == domain {
			if _, keep := values[k]; !keep {
				delete(c.data, k)
				delete(c.owner, k)
				removed++
			}
		}
	}
	keys := c.put(domain, values, at)
	c.mu.Unlock()
	c.publish(domain, keys, at)
	c.log.Debug().Str("domain", domain).Int("added", len(keys)).Int("removed", removed).Msg("cache replace domain")
}

// Merge upserts values into domain without removing other keys of the domain.
func (c *Cache) Merge(domain string, values map[string]*solis.Value, at time.Time) {
	c.mu.Lock()
	keys := c.put(domain, values, at)
	c.mu.Unlock()
	c.publish(domain, keys, at)
	c.log.Debug().Str("domain", domain).Int("keys", len(keys)).Msg("cache merge")
}

// put stores values; caller holds mu.
func (c *Cache) put(domain string, values map[string]*solis.Value, at time.Time) []string {
	keys := make([]string, 0, len(values))
	for k, v := range values {
		c.data[k] = v
		c.owner[k] = domain
		keys = append(keys, k)
	}
	c.updated[domain] = at
	sort.Strings(keys)
	return keys
}

func (c *Cache) publish(domain string, keys []string, at time.Time) {
	c.pub.Publish(eventbus.Event{
		Kind: eventbus.ValuesUpdated, Domain: domain, Keys: keys, At: at,
	})
}

// Get returns the value of key or nil.
func (c *Cache) Get(key string) *solis.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[key]
}

// GetMultiple returns the cached values of keys that are present.
func (c *Cache) GetMultiple(keys []string) map[string]*solis.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]*solis.Value, len(keys))
	for _, k := range keys {
		if v, ok := c.data[k]; ok {
			out[k] = v
		}
	}
	return out
}

// LastUpdate returns when domain was last written (zero if never). For the poller
// domain this is the last successful poll time.
func (c *Cache) LastUpdate(domain string) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.updated[domain]
}

// Size returns the number of cached keys.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}
