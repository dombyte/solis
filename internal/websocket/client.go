package websocket

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/utils"
)

const (
	// writeWait is the time allowed to write a frame.
	writeWait = 10 * time.Second
	// pongWait is the time allowed to read the next pong.
	pongWait = 60 * time.Second
	// pingPeriod must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// maxMessageSize bounds client frames (subscriptions are small).
	maxMessageSize = 64 * 1024
	// sendBuffer is the per-client outgoing queue; a full queue disconnects the client.
	sendBuffer = 256
)

// Client is one WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	clock  utils.Clock
	log    zerolog.Logger
	once   sync.Once
	closed chan struct{}
	active atomic.Int64
}

func newClient(h *Hub, conn *websocket.Conn) *Client {
	c := &Client{hub: h, conn: conn, send: make(chan []byte, sendBuffer), clock: h.d.Clock,
		log: h.d.Log, closed: make(chan struct{})}
	c.touch()
	return c
}

func (c *Client) touch() { c.active.Store(c.clock.Now().UnixNano()) }

func (c *Client) lastActivity() time.Time { return time.Unix(0, c.active.Load()) }

// enqueue queues a frame without blocking; false when the buffer is full or closed.
func (c *Client) enqueue(b []byte) bool {
	select {
	case <-c.closed:
		return false
	default:
	}
	select {
	case c.send <- b:
		return true
	default:
		return false
	}
}

// close stops the write pump (which closes the connection) exactly once.
func (c *Client) close() {
	c.once.Do(func() { close(c.closed) })
}

// readPump forwards client frames to the hub until the connection fails.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregisterClient(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait)) //nolint:errcheck // best effort
	c.conn.SetPongHandler(func(string) error {
		c.touch()
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway,
				websocket.CloseNormalClosure) {
				c.log.Debug().Err(err).Msg("websocket read")
			}
			return
		}
		c.touch()
		var msg ClientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			msg = ClientMessage{Type: "invalid json"}
		}
		c.hub.command(c, msg)
	}
}

// writePump writes queued frames and pings until the client is closed.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case <-c.closed:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait)) //nolint:errcheck // best effort
			_ = c.conn.WriteMessage(websocket.CloseMessage,        //nolint:errcheck // best effort
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "server closing"))
			return
		case b := <-c.send:
			if err := c.write(websocket.TextMessage, b); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) write(kind int, b []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	if err := c.conn.WriteMessage(kind, b); err != nil {
		c.log.Debug().Err(err).Msg("websocket write")
		return err
	}
	c.touch()
	return nil
}
