package websocket

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util"
)

// Hub timing constants.
const (
	// FlushDelay coalesces poller and aggregator events into one frame.
	FlushDelay = 75 * time.Millisecond
	// StaleClientTimeout drops clients without any activity.
	StaleClientTimeout = 5 * time.Minute
	eventBuffer        = 16
	subscriberName     = "websocket"
)

var (
	// ErrHubStopped is returned when registering with a stopped hub.
	ErrHubStopped = errors.New("websocket: hub stopped")
	// ErrMissingDependency is returned by NewHub when a dependency is nil.
	ErrMissingDependency = errors.New("websocket: missing dependency")
)

// Snapshotter reads current values from the cache.
type Snapshotter interface {
	GetMultiple(keys []string) map[string]*solis.Value
}

// KeySet validates subscription keys.
type KeySet interface {
	ByKey(key string) (solis.Register, bool)
}

// HubDeps are the hub's dependencies.
type HubDeps struct {
	Bus          eventbus.Subscriber
	Cache        Snapshotter
	Keys         KeySet
	Clock        util.Clock
	PollInterval time.Duration
	Reporter     health.Reporter
	Log          zerolog.Logger
}

// Hub fans cache changes out to subscribed clients. All client state is owned by the
// hub goroutine; clients talk to it through channels.
type Hub struct {
	*health.Status
	d HubDeps

	ops chan func(*loopState)

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool

	// events counts bus events consumed by the loop (observability, test sync point).
	events atomic.Int64
}

// loopState is owned by the hub goroutine; other goroutines reach it only through ops.
type loopState struct {
	clients map[*Client]*clientState
	dirty   bool
	flush   util.Timer
}

type clientState struct {
	subs map[string]struct{}
	last map[string]pushed
}

// NewHub validates dependencies and returns a stopped hub.
func NewHub(d HubDeps) (*Hub, error) {
	if err := util.RequireAll(ErrMissingDependency,
		util.Requirement{Name: "Bus", OK: d.Bus != nil},
		util.Requirement{Name: "Cache", OK: d.Cache != nil},
		util.Requirement{Name: "Keys", OK: d.Keys != nil},
		util.Requirement{Name: "Clock", OK: d.Clock != nil},
		util.Requirement{Name: "Reporter", OK: d.Reporter != nil},
		util.Requirement{Name: "PollInterval", OK: d.PollInterval > 0},
	); err != nil {
		return nil, err
	}
	return &Hub{
		Status: health.NewStatus(d.Reporter, d.Clock), d: d,
		ops: make(chan func(*loopState)), done: make(chan struct{}),
	}, nil
}

// Start subscribes to the bus and runs the hub loop. It is a no-op once Stop ran, so a
// late Start never reuses the already closed done channel.
func (h *Hub) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped || h.cancel != nil {
		return nil
	}
	events, unsub, err := h.d.Bus.Subscribe(subscriberName, eventBuffer, eventbus.Coalesce)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.Beat()
	go func() {
		defer unsub()
		h.loop(ctx, events)
	}()
	return nil
}

// Stop disconnects every client and ends the loop (idempotent).
func (h *Hub) Stop() error {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return nil
	}
	if h.cancel == nil { // never started: release pending Register calls
		h.stopped = true
		close(h.done)
		h.mu.Unlock()
		return nil
	}
	h.stopped = true
	cancel := h.cancel
	h.mu.Unlock()
	cancel()
	<-h.done
	return nil
}

// Register hands a new client to the hub loop.
func (h *Hub) Register(c *Client) error {
	h.d.Log.Debug().Msg("websocket client registered")
	if !h.do(func(ls *loopState) {
		ls.clients[c] = &clientState{subs: map[string]struct{}{}, last: map[string]pushed{}}
	}) {
		return ErrHubStopped
	}
	return nil
}

func (h *Hub) unregisterClient(c *Client) {
	h.do(func(ls *loopState) {
		if _, ok := ls.clients[c]; ok {
			delete(ls.clients, c)
			c.close()
		}
	})
}

func (h *Hub) command(c *Client, msg ClientMessage) {
	h.do(func(ls *loopState) { h.handle(ls.clients, c, msg) })
}

// do runs op on the hub goroutine; false when the hub has stopped.
func (h *Hub) do(op func(*loopState)) bool {
	select {
	case h.ops <- op:
		return true
	case <-h.done:
		return false
	}
}

// loop owns all client state.
func (h *Hub) loop(ctx context.Context, events <-chan eventbus.Event) {
	h.d.Log.Debug().Dur("interval", h.d.PollInterval).Msg("websocket hub loop started")
	defer close(h.done)
	ls := &loopState{
		clients: make(map[*Client]*clientState),
		flush:   h.d.Clock.NewTimer(time.Hour),
	}
	ls.flush.Stop()
	defer ls.closeAll()
	beat := h.d.Clock.NewTicker(h.d.PollInterval)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case op := <-h.ops:
			op(ls)
		case e := <-events:
			h.d.Log.Debug().Stringer("kind", e.Kind).Str("domain", e.Domain).
				Int("keys", len(e.Keys)).Msg("websocket event received")
			ls.onEvent(e)
			h.events.Add(1)
		case <-ls.flush.C():
			ls.dirty = false
			h.flush(ls.clients)
		case <-beat.C():
			h.Beat()
			h.dropStale(ls.clients)
		}
	}
}

// onEvent marks the state dirty and arms one coalescing flush.
func (l *loopState) onEvent(e eventbus.Event) {
	if e.Kind == eventbus.ValuesUpdated && !l.dirty {
		l.dirty = true
		l.flush.Reset(FlushDelay)
	}
}

func (l *loopState) closeAll() {
	for c := range l.clients {
		c.close()
	}
}

// handle applies a subscribe/unsubscribe/ping command.
func (h *Hub) handle(clients map[*Client]*clientState, c *Client, msg ClientMessage) {
	st, ok := clients[c]
	if !ok {
		return
	}
	switch msg.Type {
	case TypeSubscribe:
		h.subscribe(clients, c, st, msg.Keys)
	case TypeUnsubscribe:
		for _, k := range msg.Keys {
			delete(st.subs, k)
			delete(st.last, k)
		}
	case TypePing:
	default:
		h.send(clients, c, ErrorMessage{
			Type: TypeError, Code: CodeBadRequest,
			Message: "unknown message type " + msg.Type,
		})
	}
}

// subscribe adds known keys and answers with a snapshot of the newly added ones;
// unknown keys get an error frame and never drop the connection.
func (h *Hub) subscribe(clients map[*Client]*clientState, c *Client, st *clientState,
	keys []string,
) {
	var added, unknown []string
	for _, k := range keys {
		if _, ok := h.d.Keys.ByKey(k); !ok {
			unknown = append(unknown, k)
			continue
		}
		if _, dup := st.subs[k]; !dup {
			st.subs[k] = struct{}{}
			added = append(added, k)
		}
	}
	if len(unknown) > 0 {
		h.send(clients, c, ErrorMessage{
			Type: TypeError, Code: CodeUnknownKeys,
			Message: "unknown keys ignored", Keys: unknown,
		})
	}
	snap := SnapshotMessage{Type: TypeSnapshot, Values: map[string]ValueDTO{}}
	for k, v := range h.d.Cache.GetMultiple(added) {
		snap.Values[k] = fullDTO(v)
		st.last[k] = stateOf(v)
	}
	h.send(clients, c, snap)
}

// flush pushes the changed subscribed keys to every client (one frame per client).
func (h *Hub) flush(clients map[*Client]*clientState) {
	current := h.d.Cache.GetMultiple(unionKeys(clients))
	ts := h.d.Clock.Now().Format(time.RFC3339)
	for c, st := range clients {
		upd := st.diff(current)
		if len(upd.Values) > 0 || len(upd.Removed) > 0 {
			upd.Type, upd.TS = TypeUpdate, ts
			h.send(clients, c, upd)
		}
	}
}

// diff returns the changed and removed subscribed keys and records them as pushed.
func (c *clientState) diff(current map[string]*solis.Value) UpdateMessage {
	upd := UpdateMessage{Values: map[string]ValueDTO{}}
	for k := range c.subs {
		v, ok := current[k]
		if !ok {
			if _, seen := c.last[k]; seen {
				delete(c.last, k)
				upd.Removed = append(upd.Removed, k)
			}
			continue
		}
		s := stateOf(v)
		if prev, seen := c.last[k]; seen && prev.equal(s) {
			continue
		}
		c.last[k] = s
		upd.Values[k] = updateDTO(v)
	}
	sort.Strings(upd.Removed)
	return upd
}

func unionKeys(clients map[*Client]*clientState) []string {
	set := make(map[string]struct{})
	for _, st := range clients {
		for k := range st.subs {
			set[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// send encodes and queues msg; a full buffer disconnects the client.
func (h *Hub) send(clients map[*Client]*clientState, c *Client, msg any) {
	b, err := encode(msg)
	if err != nil {
		h.d.Log.Error().Err(err).Msg("encode websocket frame")
		return
	}
	if !c.enqueue(b) {
		h.d.Log.Warn().Msg("websocket client buffer full, disconnecting")
		delete(clients, c)
		c.close()
	}
}

func (h *Hub) dropStale(clients map[*Client]*clientState) {
	now := h.d.Clock.Now()
	for c := range clients {
		if now.Sub(c.lastActivity()) > StaleClientTimeout {
			delete(clients, c)
			c.close()
		}
	}
}
