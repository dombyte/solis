package websocket

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// websocket buffer sizes.
const (
	readBufferSize  = 1024
	writeBufferSize = 1024
)

// HubSource yields the current hub (it is replaced when the supervisor restarts it).
type HubSource interface {
	Load() (*Hub, bool)
}

// Handler upgrades same-origin requests and registers the connection with the current
// hub. It owns its upgrader (no package-level state).
type Handler struct {
	hubs     HubSource
	upgrader websocket.Upgrader
	log      zerolog.Logger
}

// NewHandler returns the /ws handler.
func NewHandler(hubs HubSource, log zerolog.Logger) *Handler {
	return &Handler{
		hubs: hubs,
		log:  log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  readBufferSize,
			WriteBufferSize: writeBufferSize,
			CheckOrigin:     SameOrigin,
		},
	}
}

// SameOrigin accepts requests without an Origin header (non-browser clients) and browser
// requests whose Origin host equals the request host. Mismatches are rejected before
// the upgrade.
func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// ServeHTTP upgrades the connection and hands it to the hub.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hub, ok := h.hubs.Load()
	if !ok {
		writeUnavailable(w)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Debug().Err(err).Msg("websocket upgrade rejected")
		return
	}
	c := newClient(hub, conn)
	if err := hub.Register(c); err != nil {
		_ = conn.Close()
		return
	}
	go c.writePump()
	go c.readPump()
}

// writeUnavailable answers 503 with the API's JSON error body while the hub restarts.
func writeUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // best effort
		"error":   http.StatusText(http.StatusServiceUnavailable),
		"message": "websocket hub restarting",
		"code":    http.StatusServiceUnavailable,
	})
}
