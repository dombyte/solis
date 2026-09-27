package app

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/aggregator"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/utils"
	"github.com/dombyte/solis/internal/websocket"
)

// CreateModbus returns the factory of the restartable Modbus component. Each instance
// publishes its client through slot so the poller never holds a stale reference (D11).
func CreateModbus(cfg config.ModbusSettings, interval time.Duration,
	slot *utils.Slot[poller.Reader], clock utils.Clock, log zerolog.Logger) health.Factory {
	return func(rep health.Reporter) (health.Component, error) {
		c, err := modbus.New(modbus.Settings{Host: cfg.Host, Port: cfg.Port, UnitID: cfg.UnitID,
			Timeout: cfg.Timeout}, clock, log)
		if err != nil {
			return nil, err
		}
		return &modbusComponent{Status: health.NewStatus(rep, clock), client: c, slot: slot,
			interval: interval}, nil
	}
}

// modbusComponent adapts modbus.Client (an external layer without health imports) to the
// supervision contract: connected = healthy, reconnecting = recovering.
type modbusComponent struct {
	*health.Status
	client   *modbus.Client
	slot     *utils.Slot[poller.Reader]
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func (m *modbusComponent) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel, m.done = cancel, make(chan struct{})
	m.mu.Unlock()
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

func (m *modbusComponent) Stop() error {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.cancel = nil
	m.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	m.slot.Clear()
	return m.client.Close()
}

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
}

func (h *hubComponent) Start(ctx context.Context) error {
	if err := h.Hub.Start(ctx); err != nil {
		return err
	}
	h.slot.Store(h.Hub)
	return nil
}

func (h *hubComponent) Stop() error {
	h.slot.Clear()
	return h.Hub.Stop()
}
