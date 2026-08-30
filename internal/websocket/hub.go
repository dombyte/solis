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
	ticker := h.setupTicker()
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.handleRegister(client)
		case client := <-h.unregister:
			h.handleUnregister(client)
		case message := <-h.broadcast:
			h.handleBroadcast(message)
		case <-ticker.C:
			h.cleanupStaleClients()
		}
	}
}

// setupTicker sets up the cleanup ticker
func (h *Hub) setupTicker() *time.Ticker {
	return time.NewTicker(h.staleClientTimeout / 2)
}

// handleRegister handles a new client registration
func (h *Hub) handleRegister(client *Client) {
	h.mu.Lock()
	h.clients[client] = true
	h.lastActivity[client] = time.Now()
	h.mu.Unlock()
	logger.Debug().Msgf("Client registered, total clients: %d", len(h.clients))
}

// handleUnregister handles a client unregistration
func (h *Hub) handleUnregister(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		client.Close()
		delete(h.clients, client)
		delete(h.lastActivity, client)
	}
	h.mu.Unlock()
	logger.Debug().Msgf("Client unregistered, total clients: %d", len(h.clients))
}

// handleBroadcast handles a broadcast message to all clients
func (h *Hub) handleBroadcast(message []byte) {
	clients := h.getAllClients()
	clientsWithFullBuffers := h.trySendToAll(clients, message)
	h.closeClientsWithFullBuffers(clientsWithFullBuffers)
}

// getAllClients returns a slice of all registered clients
func (h *Hub) getAllClients() []*Client {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	return clients
}

// trySendToAll tries to send a message to all clients and returns clients with full buffers
func (h *Hub) trySendToAll(clients []*Client, message []byte) []*Client {
	clientsWithFullBuffers := make([]*Client, 0)
	for _, client := range clients {
		select {
		case client.send <- message:
			h.updateClientActivity(client)
		default:
			clientsWithFullBuffers = append(clientsWithFullBuffers, client)
			logger.Warn().Msg("Client buffer full, closing connection")
		}
	}
	return clientsWithFullBuffers
}

// updateClientActivity updates the last activity time for a client
func (h *Hub) updateClientActivity(client *Client) {
	h.mu.Lock()
	h.lastActivity[client] = time.Now()
	h.mu.Unlock()
}

// closeClientsWithFullBuffers closes clients that have full buffers
func (h *Hub) closeClientsWithFullBuffers(clients []*Client) {
	for _, client := range clients {
		client.Close()
		h.mu.Lock()
		delete(h.clients, client)
		delete(h.lastActivity, client)
		h.mu.Unlock()
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
		client.Close()
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
