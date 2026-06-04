// Package cache provides an in-memory caching layer for the latest register values.
// It is designed to serve reads from HTTP handlers without blocking and be updated
// asynchronously by the poller when new data is available.
// It also supports WebSocket notifications for real-time updates.
package cache

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/websocket"
)

// logger is the package-level logger for cache operations.
var logger = logging.NewComponentLogger("cache")

// Cache is a thread-safe in-memory cache for the latest register values.
// It uses separate locks for reads and writes to ensure non-blocking read operations.
type Cache struct {
	// mu protects writes to the data map.
	// Reads use RWMutex for concurrent access.
	mu sync.RWMutex
	// data stores the latest values for each register key.
	data map[string]*solis.Value
	// wsHub is the WebSocket hub for broadcasting cache updates.
	wsHub *websocket.Hub
	// lastUpdate tracks when the cache was last updated.
	lastUpdate time.Time
	// notifyChan is used for throttling WebSocket notifications.
	// Only one notification goroutine is allowed at a time.
	notifyChan chan struct{}
}

// New creates a new Cache instance.
func New() *Cache {
	return &Cache{
		data:       make(map[string]*solis.Value),
		notifyChan: make(chan struct{}, 1),
	}
}

// SetWebSocketHub configures the WebSocket hub for real-time notifications.
// When set, the cache will broadcast updates to all connected WebSocket clients.
func (c *Cache) SetWebSocketHub(hub *websocket.Hub) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wsHub = hub
}

// GetWebSocketHub returns the configured WebSocket hub.
func (c *Cache) GetWebSocketHub() *websocket.Hub {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.wsHub
}

// SendInitialData sends the current cache state to a specific WebSocket client.
// This is used when a new WebSocket client connects and requests the current cache state.
func (c *Cache) SendInitialData(client *websocket.Client) {
	if client == nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Serialize directly from c.data - safe because we hold RLock
	message, err := json.Marshal(WebSocketCacheUpdate{
		Type:      "cache_update",
		Timestamp: c.lastUpdate.Format(time.RFC3339Nano),
		Data:      c.data,
	})
	if err != nil {
		logger.Error().Msgf("Failed to marshal initial WebSocket message: %v", err)
		return
	}

	// Send directly to this client
	if !client.Send(message) {
		logger.Warn().Msg("Failed to send initial cache data: client buffer full")
	}
}

// WebSocketCacheUpdate represents a cache update message sent to WebSocket clients.
type WebSocketCacheUpdate struct {
	Type      string                  `json:"type"`
	Timestamp string                  `json:"timestamp"`
	Data      map[string]*solis.Value `json:"data"`
}

// Get retrieves a value from the cache.
// Returns nil if the key is not found.
func (c *Cache) Get(key string) *solis.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if val, ok := c.data[key]; ok {
		return val
	}
	return nil
}

// GetMultiple retrieves multiple values from the cache.
// Returns a map of key to value for keys that exist in the cache.
func (c *Cache) GetMultiple(keys []string) map[string]*solis.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*solis.Value, len(keys))
	for _, key := range keys {
		if val, ok := c.data[key]; ok {
			result[key] = val
		}
	}
	return result
}

// GetAll retrieves all values from the cache.
func (c *Cache) GetAll() map[string]*solis.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create a copy to avoid holding the lock during iteration
	result := make(map[string]*solis.Value, len(c.data))
	for k, v := range c.data {
		result[k] = v
	}
	return result
}

// Set updates the cache with new values.
// This is called by the poller after successfully storing data.
// The entire update is atomic to ensure consistency.
// Only the latest values are kept - old entries are removed.
func (c *Cache) Set(values map[string]*solis.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Replace entire cache with new values to ensure only latest entries are kept
	// This prevents unbounded cache growth
	c.data = make(map[string]*solis.Value, len(values))
	for key, value := range values {
		c.data[key] = value
	}
	c.lastUpdate = time.Now()

	logger.Debug().Msgf("Cache updated with %d values", len(values))

	// Notify WebSocket clients if hub is configured and has clients
	// Use throttling to prevent unbounded goroutine creation
	if c.wsHub != nil && c.wsHub.ClientCount() > 0 {
		select {
		case c.notifyChan <- struct{}{}:
			go c.notifyWebSocketClients()
		default:
			// A notification is already in progress, skip this one
			logger.Debug().Msg("Skipping WebSocket notification - previous notification still in progress")
		}
	}
}

// notifyWebSocketClients broadcasts the current cache state to all connected WebSocket clients.
func (c *Cache) notifyWebSocketClients() {
	// Release the notification slot when we're done
	defer func() {
		<-c.notifyChan
	}()

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Serialize directly from c.data - safe because we hold RLock
	message, err := json.Marshal(WebSocketCacheUpdate{
		Type:      "cache_update",
		Timestamp: c.lastUpdate.Format(time.RFC3339Nano),
		Data:      c.data,
	})
	if err != nil {
		logger.Error().Msgf("Failed to marshal WebSocket message: %v", err)
		return
	}

	c.wsHub.Broadcast(message)
	logger.Debug().Msgf("Broadcast cache update to %d WebSocket clients", c.wsHub.ClientCount())
}

// SetSingle updates a single value in the cache.
func (c *Cache) SetSingle(key string, value *solis.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = value
}

// Keys returns all keys currently in the cache.
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.data))
	for k := range c.data {
		keys = append(keys, k)
	}
	return keys
}

// Size returns the number of entries in the cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.data)
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*solis.Value)
	logger.Debug().Msg("Cache cleared")
}
