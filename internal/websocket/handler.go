// Package websocket provides WebSocket communication for real-time updates.
package websocket

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// upgrader is the WebSocket upgrader configuration.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// TODO: Add origin validation for production
		return true
	},
}

// Handler returns an HTTP handler that upgrades the connection to WebSocket
// and registers the client with the hub.
func Handler(hub *Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	})
}
