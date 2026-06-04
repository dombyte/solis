// Package websocket provides WebSocket communication for real-time updates.
package websocket

import (
	"sync"
	"time"

	"github.com/dombyte/solis/internal/logging"
)

// logger is the package-level logger for websocket operations.
var logger = logging.NewComponentLogger("websocket")

// Message types for client-server communication.
const (
	MessageTypeRequestInitial = "request_initial_data"
	MessageTypeCacheUpdate    = "cache_update"
)

// ClientMessage represents messages received from clients.
type ClientMessage struct {
	Type string `json:"type"`
}

// Hub maintains the set of active clients and broadcasts messages to the clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Mutex for concurrent access to clients map.
	mu sync.RWMutex

	// Callback for when a client requests initial data.
	onInitialDataRequest func(*Client)

	// lastActivity tracks the last time each client was active (read or write).
	// Used for cleaning up stale connections.
	lastActivity map[*Client]time.Time

	// staleClientTimeout is the duration after which a client is considered stale.
	staleClientTimeout time.Duration
}

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		clients:            make(map[*Client]bool),
		broadcast:          make(chan []byte, 256),
		register:           make(chan *Client),
		unregister:         make(chan *Client),
		lastActivity:       make(map[*Client]time.Time),
		staleClientTimeout: 5 * time.Minute,
	}
}

// Run starts the hub's main loop.
func (h *Hub) Run() {
	// Start a ticker for stale client cleanup
	ticker := time.NewTicker(h.staleClientTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.lastActivity[client] = time.Now()
			h.mu.Unlock()
			logger.Debug().Msgf("Client registered, total clients: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				close(client.send)
				delete(h.clients, client)
				delete(h.lastActivity, client)
			}
			h.mu.Unlock()
			logger.Debug().Msgf("Client unregistered, total clients: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := make([]*Client, 0, len(h.clients))
			for client := range h.clients {
				clients = append(clients, client)
			}
			h.mu.RUnlock()

			// Try to send to all clients
			clientsWithFullBuffers := make([]*Client, 0)
			for _, client := range clients {
				select {
				case client.send <- message:
					// Update last activity on successful send
					h.mu.Lock()
					h.lastActivity[client] = time.Now()
					h.mu.Unlock()
				default:
					// Client buffer full, mark for closing
					clientsWithFullBuffers = append(clientsWithFullBuffers, client)
					logger.Warn().Msg("Client buffer full, closing connection")
				}
			}

			// Close clients with full buffers
			for _, client := range clientsWithFullBuffers {
				close(client.send)
				h.mu.Lock()
				delete(h.clients, client)
				delete(h.lastActivity, client)
				h.mu.Unlock()
			}

		case <-ticker.C:
			// Clean up stale clients
			h.cleanupStaleClients()
		}
	}
}

// cleanupStaleClients removes clients that haven't been active for longer than the timeout.
func (h *Hub) cleanupStaleClients() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	staleClients := make([]*Client, 0)
	for client, lastActive := range h.lastActivity {
		if now.Sub(lastActive) > h.staleClientTimeout {
			staleClients = append(staleClients, client)
		}
	}

	for _, client := range staleClients {
		close(client.send)
		delete(h.clients, client)
		delete(h.lastActivity, client)
		logger.Debug().Msgf("Removed stale client, total clients: %d", len(h.clients))
	}

	if len(staleClients) > 0 {
		logger.Info().Msgf("Cleaned up %d stale WebSocket clients", len(staleClients))
	}
}

// UpdateLastActivity updates the last activity time for a client.
// This should be called whenever a client sends or receives a message.
func (h *Hub) UpdateLastActivity(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastActivity[client] = time.Now()
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(message []byte) {
	select {
	case h.broadcast <- message:
	default:
		logger.Warn().Msg("Broadcast channel full, dropping message")
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// SetOnInitialDataRequest sets the callback for when a client requests initial data.
func (h *Hub) SetOnInitialDataRequest(callback func(*Client)) {
	h.onInitialDataRequest = callback
}
