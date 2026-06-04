// Package websocket provides WebSocket communication for real-time updates.
package websocket

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512 * 1024 // 512KB

	// Time to wait before closing connection after write error.
	closeGracePeriod = 10 * time.Second
)

// Client is a middleman between the WebSocket connection and the hub.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Send sends a message to this client.
func (c *Client) Send(message []byte) bool {
	select {
	case c.send <- message:
		return true
	default:
		return false
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
// It also handles ping/pong messages and client requests.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				logger.Warn().Msgf("WebSocket error: %v", err)
			}
			break
		}

		// Update last activity time for this client
		c.hub.UpdateLastActivity(c)

		if messageType == websocket.TextMessage {
			var msg ClientMessage
			if err := json.Unmarshal(message, &msg); err == nil {
				switch msg.Type {
				case MessageTypeRequestInitial:
					// Client requests initial data
					if c.hub.onInitialDataRequest != nil {
						c.hub.onInitialDataRequest(c)
					}
				default:
					logger.Debug().Msgf("Received unknown message type: %s", msg.Type)
				}
			} else {
				logger.Debug().Msgf("Failed to parse client message: %v", err)
			}
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		// Graceful close: wait for write to complete or timeout
		time.Sleep(closeGracePeriod)
		c.conn.Close()
	}()

	for {
		select {
		case message := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				logger.Error().Msgf("Failed to get writer: %v", err)
				return
			}
			if _, err := w.Write(message); err != nil {
				logger.Error().Msgf("Failed to write message: %v", err)
				return
			}
			if err := w.Close(); err != nil {
				logger.Error().Msgf("Failed to close writer: %v", err)
				return
			}
			// Update last activity time for this client
			c.hub.UpdateLastActivity(c)

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.Debug().Msgf("Failed to write ping: %v", err)
				return
			}
		}
	}
}

// ServeWebSocket handles WebSocket requests from the peer.
func ServeWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) *Client {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error().Msgf("WebSocket upgrade failed: %v", err)
		return nil
	}

	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}

	hub.register <- client
	go client.writePump()
	go client.readPump()

	return client
}
