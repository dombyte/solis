package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/health/mocks"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util"
	"github.com/dombyte/solis/internal/util/clocktest"
)

var t0 = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

type env struct {
	t     *testing.T
	clk   *clocktest.Clock
	cache *cache.Cache
	hub   *Hub
	srv   *httptest.Server
	slot  *util.Slot[*Hub]
	sent  int64 // cache writes (= bus events) so far
}

func newEnv(t *testing.T) *env {
	t.Helper()
	reg, err := solis.NewRegistry()
	require.NoError(t, err)
	bus := eventbus.New()
	clk := clocktest.New(t0)
	rep := mocks.NewMockReporter(t)
	rep.EXPECT().Report(health.Healthy, "").Maybe()
	hub, err := NewHub(HubDeps{
		Bus: bus, Cache: cache.New(eventbus.New(), zerolog.Nop()), Keys: reg,
		Clock: clk, PollInterval: 5 * time.Second, Reporter: rep, Log: zerolog.Nop(),
	})
	require.NoError(t, err)
	e := &env{t: t, clk: clk, cache: cache.New(bus, zerolog.Nop()), hub: hub, slot: &util.Slot[*Hub]{}}
	hub.d.Cache = e.cache // the hub reads the same cache that publishes on the bus
	require.NoError(t, hub.Start(context.Background()))
	t.Cleanup(func() { _ = hub.Stop() })
	e.slot.Store(hub)
	e.srv = httptest.NewServer(NewHandler(e.slot, zerolog.Nop()))
	t.Cleanup(e.srv.Close)
	require.True(t, clk.BlockUntil(1), "beat ticker")
	return e
}

func (e *env) dial(header http.Header) *websocket.Conn {
	e.t.Helper()
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	require.NoError(e.t, err)
	_ = resp.Body.Close()
	e.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (e *env) set(domain string, at time.Time, kv map[string]float64) {
	vals := make(map[string]*solis.Value, len(kv))
	for k, v := range kv {
		vals[k] = &solis.Value{Key: k, DecodedValue: v, Unit: "W", Timestamp: at}
	}
	e.cache.Merge(domain, vals, at)
	e.sent++
}

// flush waits until the hub consumed every event, then fires the (possibly already
// fired) coalescing timer.
func (e *env) flush() {
	require.Eventually(e.t, func() bool { return e.hub.events.Load() >= e.sent }, 2*time.Second,
		time.Millisecond)
	e.clk.Advance(FlushDelay)
}

func send(t *testing.T, c *websocket.Conn, msg ClientMessage) {
	t.Helper()
	require.NoError(t, c.WriteJSON(msg))
}

func read(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
	var m map[string]any
	require.NoError(t, c.ReadJSON(&m))
	return m
}

func values(m map[string]any) map[string]any {
	v, _ := m["values"].(map[string]any)
	return v
}

func TestSubscribeSnapshotUpdateUnsubscribe(t *testing.T) {
	e := newEnv(t)
	e.set(eventbus.DomainPoller, t0, map[string]float64{
		"pv_total_power": 5230.456,
		"grid_power":     -2,
	})
	c := e.dial(nil)

	send(t, c, ClientMessage{Type: TypeSubscribe, Keys: []string{
		"pv_total_power",
		"battery_soc",
	}})
	snap := read(t, c)
	assert.Equal(t, TypeSnapshot, snap["type"])
	vals := values(snap)
	require.Len(t, vals, 1, "exactly the subscribed keys that exist")
	pv := vals["pv_total_power"].(map[string]any)
	assert.InDelta(t, 5230.46, pv["value"], 1e-9)
	assert.Equal(t, "W", pv["unit"])
	assert.Equal(t, t0.Format(time.RFC3339), pv["timestamp"])

	// Unchanged pv, changed unsubscribed grid: no frame. Then a pv change arrives alone.
	e.set(eventbus.DomainPoller, t0.Add(time.Second), map[string]float64{
		"pv_total_power": 5230.456, "grid_power": 100,
	})
	e.flush()
	e.set(eventbus.DomainPoller, t0.Add(2*time.Second), map[string]float64{
		"pv_total_power": 4000,
	})
	e.flush()
	upd := read(t, c)
	assert.Equal(t, TypeUpdate, upd["type"])
	assert.NotEmpty(t, upd["ts"])
	assert.Equal(t, map[string]any{"pv_total_power": map[string]any{"value": 4000.0}},
		values(upd))

	// Late-arriving subscribed key is pushed as an update.
	e.set(eventbus.DomainPoller, t0.Add(3*time.Second), map[string]float64{"battery_soc": 55})
	e.flush()
	assert.Contains(t, values(read(t, c)), "battery_soc")

	send(t, c, ClientMessage{Type: TypeUnsubscribe, Keys: []string{"pv_total_power"}})
	send(t, c, ClientMessage{Type: TypeSubscribe, Keys: []string{"grid_power"}})
	assert.Contains(t, values(read(t, c)), "grid_power") // snapshot sync point
	e.set(eventbus.DomainPoller, t0.Add(4*time.Second), map[string]float64{
		"pv_total_power": 1, "grid_power": 7,
	})
	e.flush()
	assert.Equal(t, map[string]any{"grid_power": map[string]any{"value": 7.0}}, values(read(t, c)))
}

func TestRemovedKeyIsPushed(t *testing.T) {
	e := newEnv(t)
	e.set(eventbus.DomainPoller, t0, map[string]float64{
		"battery_power_signed": 100,
		"grid_power":           5,
	})
	c := e.dial(nil)
	send(t, c, ClientMessage{Type: TypeSubscribe, Keys: []string{
		"battery_power_signed",
		"grid_power",
	}})
	require.Len(t, values(read(t, c)), 2)

	// The next poll no longer derives battery_power_signed: clients must drop it.
	e.cache.ReplaceDomain(eventbus.DomainPoller, map[string]*solis.Value{
		"grid_power": {Key: "grid_power", DecodedValue: 5, Timestamp: t0},
	}, t0.Add(time.Second))
	e.sent++
	e.flush()
	upd := read(t, c)
	assert.Equal(t, TypeUpdate, upd["type"])
	assert.Equal(t, []any{"battery_power_signed"}, upd["removed"])
	assert.Empty(t, values(upd), "unchanged grid_power is not repeated")
}

func TestCoalescesPollerAndAggregatorEvents(t *testing.T) {
	e := newEnv(t)
	c := e.dial(nil)
	send(t, c, ClientMessage{Type: TypeSubscribe, Keys: []string{
		"pv_energy_daily",
		"pv_energy_monthly",
	}})
	read(t, c) // empty snapshot
	e.set(eventbus.DomainPoller, t0, map[string]float64{"pv_energy_daily": 1})
	e.set(eventbus.DomainAggregator, t0, map[string]float64{"pv_energy_monthly": 2})
	e.flush()
	vals := values(read(t, c))
	assert.Len(t, vals, 2, "both events in one frame")
}

func TestUnknownKeysAndBadMessagesKeepConnection(t *testing.T) {
	e := newEnv(t)
	c := e.dial(nil)
	send(t, c, ClientMessage{Type: TypeSubscribe, Keys: []string{"foo", "pv_total_power"}})
	errFrame := read(t, c)
	assert.Equal(t, TypeError, errFrame["type"])
	assert.Equal(t, CodeUnknownKeys, errFrame["code"])
	assert.Equal(t, []any{"foo"}, errFrame["keys"])
	assert.Equal(t, TypeSnapshot, read(t, c)["type"])

	send(t, c, ClientMessage{Type: TypePing}) // accepted, no reply
	require.NoError(t, c.WriteMessage(websocket.TextMessage, []byte("{not json")))
	bad := read(t, c)
	assert.Equal(t, CodeBadRequest, bad["code"])
	send(t, c, ClientMessage{Type: "request_initial_data"})
	assert.Equal(t, CodeBadRequest, read(t, c)["code"])
}

func TestOriginValidation(t *testing.T) {
	e := newEnv(t)
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http")
	_, resp, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {"http://evil.example"}})
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	same := http.Header{"Origin": {e.srv.URL}}
	c := e.dial(same)
	send(t, c, ClientMessage{Type: TypeSubscribe, Keys: []string{"grid_power"}})
	assert.Equal(t, TypeSnapshot, read(t, c)["type"])

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Host = "solis.local:8080"
	assert.True(t, SameOrigin(r))
	r.Header.Set("Origin", "http://SOLIS.local:8080")
	assert.True(t, SameOrigin(r))
	r.Header.Set("Origin", "http://other:8080")
	assert.False(t, SameOrigin(r))
	r.Header.Set("Origin", "://bad")
	assert.False(t, SameOrigin(r))
}

func TestHubStopClosesClientsAndRestartingHub503(t *testing.T) {
	e := newEnv(t)
	c := e.dial(nil)
	send(t, c, ClientMessage{Type: TypeSubscribe, Keys: []string{"grid_power"}})
	read(t, c)
	e.slot.Clear()
	require.NoError(t, e.hub.Stop())
	require.NoError(t, e.hub.Stop())
	require.NoError(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err := c.ReadMessage()
	assert.True(t, websocket.IsCloseError(err, websocket.CloseGoingAway), "%v", err)

	resp, err := http.Get(e.srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	// Same JSON error body as the REST API (review HTTP-L9).
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "websocket hub restarting", body["message"])
	assert.InDelta(t, 503, body["code"], 0)
	_ = resp.Body.Close()

	assert.ErrorIs(t, e.hub.Register(&Client{}), ErrHubStopped)
}

func TestNewHubValidationAndUnstartedStop(t *testing.T) {
	_, err := NewHub(HubDeps{})
	assert.ErrorIs(t, err, ErrMissingDependency)
	assert.EqualError(t, err, "websocket: missing dependency: Bus")
	e := newEnv(t)
	h, err := NewHub(e.hub.d)
	require.NoError(t, err)
	require.NoError(t, h.Stop())
	assert.ErrorIs(t, h.Register(&Client{}), ErrHubStopped)
	// A late Start after Stop is a no-op (it used to close h.done twice and panic).
	require.NoError(t, h.Start(context.Background()))
	require.NoError(t, h.Stop())
	assert.ErrorIs(t, h.Register(&Client{}), ErrHubStopped)
}

func TestFullBufferDisconnects(t *testing.T) {
	e := newEnv(t)
	c := &Client{send: make(chan []byte, 1), closed: make(chan struct{})}
	clients := map[*Client]*clientState{c: {}}
	e.hub.send(clients, c, ErrorMessage{})
	assert.Len(t, clients, 1)
	e.hub.send(clients, c, ErrorMessage{})
	assert.Empty(t, clients)
	assert.False(t, c.enqueue([]byte("x")))
}

func TestStaleClientsAreDropped(t *testing.T) {
	e := newEnv(t)
	c := &Client{clock: e.clk, closed: make(chan struct{})}
	c.touch()
	clients := map[*Client]*clientState{c: {}}
	e.clk.Advance(StaleClientTimeout + time.Second)
	e.hub.dropStale(clients)
	assert.Empty(t, clients)
}

func TestDTOEncoding(t *testing.T) {
	v := &solis.Value{
		DecodedValue: 1.005, Unit: "kWh", Timestamp: t0,
		StatusDecoded: []string{"No grid"},
	}
	b, err := json.Marshal(fullDTO(v))
	require.NoError(t, err)
	assert.JSONEq(t, `{"value":1.00,"unit":"kWh","timestamp":"2026-08-05T12:00:00Z",
		"status_decoded":["No grid"]}`, string(b))
	assert.True(t, stateOf(v).equal(stateOf(&solis.Value{
		DecodedValue:  1.001,
		StatusDecoded: []string{"No grid"},
	})))
	assert.False(t, stateOf(v).equal(stateOf(&solis.Value{DecodedValue: 1.001})))
}
