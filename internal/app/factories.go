package app

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/aggregator"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/utils"
	"github.com/dombyte/solis/internal/websocket"
)

// CreateModbus returns the factory of the restartable Modbus component. Each instance
// publishes its client through slot so the poller never holds a stale reference (D11).
func CreateModbus(settings modbus.Settings, interval time.Duration,
	slot *utils.Slot[poller.Reader], clock utils.Clock, log zerolog.Logger,
) health.Factory {
	return func(rep health.Reporter) (health.Component, error) {
		c, err := modbus.New(settings, clock, log)
		if err != nil {
			return nil, err
		}
		return &modbusComponent{
			Status: health.NewStatus(rep, clock), client: c, slot: slot,
			interval: interval,
		}, nil
	}
}

// modbusComponent adapts modbus.Client (an external layer without health imports) to the
// supervision contract: connected = healthy, reconnecting = recovering.
type modbusComponent struct {
	*health.Status
	client   *modbus.Client
	slot     *utils.Slot[poller.Reader]
	interval time.Duration

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool
}

// Start publishes the client and runs the reconnect loop; a no-op once started or
// stopped, so a late Start never publishes a closed client.
func (m *modbusComponent) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped || m.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	m.cancel, m.done = cancel, make(chan struct{})
	m.slot.Store(m.client)
	go func() {
		defer close(m.done)
		m.client.Run(ctx, m.interval, m.beat)
	}()
	return nil
}

func (m *modbusComponent) beat() {
	m.Beat()
	if m.client.IsConnected() {
		m.Set(health.Healthy, "")
		return
	}
	m.Set(health.Recovering, "modbus disconnected, reconnecting")
}

// Stop ends the loop, withdraws this instance's client (never a successor's) and closes
// it (idempotent).
func (m *modbusComponent) Stop() error {
	m.mu.Lock()
	cancel, done, wasStopped := m.cancel, m.done, m.stopped
	m.stopped = true
	m.mu.Unlock()
	if cancel == nil || wasStopped {
		return nil
	}
	cancel()
	<-done
	m.slot.ClearIf(m.owns)
	return m.client.Close()
}

func (m *modbusComponent) owns(r poller.Reader) bool { return r == poller.Reader(m.client) }

// CreatePoller returns the poller factory.
func CreatePoller(deps poller.Deps) health.Factory {
	return func(rep health.Reporter) (health.Component, error) {
		deps.Reporter = rep
		return poller.New(deps)
	}
}

// CreateAggregator returns the aggregator factory.
func CreateAggregator(deps aggregator.Deps) health.Factory {
	return func(rep health.Reporter) (health.Component, error) {
		deps.Reporter = rep
		return aggregator.New(deps)
	}
}

// CreateHub returns the WebSocket hub factory; a started hub is published through slot
// for the /ws handler and withdrawn on stop (clients reconnect to the next instance).
func CreateHub(deps websocket.HubDeps, slot *utils.Slot[*websocket.Hub]) health.Factory {
	return func(rep health.Reporter) (health.Component, error) {
		deps.Reporter = rep
		h, err := websocket.NewHub(deps)
		if err != nil {
			return nil, err
		}
		return &hubComponent{Hub: h, slot: slot}, nil
	}
}

type hubComponent struct {
	*websocket.Hub
	slot *utils.Slot[*websocket.Hub]

	mu      sync.Mutex
	stopped bool
}

// Start runs the hub and publishes it unless it was already stopped.
func (h *hubComponent) Start(ctx context.Context) error {
	if err := h.Hub.Start(ctx); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.stopped {
		h.slot.Store(h.Hub)
	}
	return nil
}

// Stop withdraws this hub (never a successor) and stops it.
func (h *hubComponent) Stop() error {
	h.mu.Lock()
	h.stopped = true
	h.mu.Unlock()
	h.slot.ClearIf(func(v *websocket.Hub) bool { return v == h.Hub })
	return h.Hub.Stop()
}
