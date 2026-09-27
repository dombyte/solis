package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	sv "github.com/simonvetter/modbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/maintenance"
)

// inverter is a fake Modbus device.
type inverter struct {
	mu   sync.Mutex
	regs map[uint16]uint16
}

func (d *inverter) HandleCoils(*sv.CoilsRequest) ([]bool, error) {
	return nil, sv.ErrIllegalFunction
}
func (d *inverter) HandleDiscreteInputs(*sv.DiscreteInputsRequest) ([]bool, error) {
	return nil, sv.ErrIllegalFunction
}
func (d *inverter) HandleHoldingRegisters(*sv.HoldingRegistersRequest) ([]uint16, error) {
	return nil, sv.ErrIllegalFunction
}
func (d *inverter) HandleInputRegisters(req *sv.InputRegistersRequest) ([]uint16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]uint16, req.Quantity)
	for i := range out {
		out[i] = d.regs[req.Addr+uint16(i)]
	}
	return out, nil
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func testConfig(t *testing.T, modbusPort, httpPort int) *config.AppConfig {
	return &config.AppConfig{
		App: config.AppSettings{Debug: "ERROR", Port: httpPort, Timeout: 5 * time.Second},
		Poller: config.PollerSettings{Interval: 300 * time.Millisecond, BlockAttempts: 1,
			BlockRetryDelay: 10 * time.Millisecond, PollTimeout: 2 * time.Second},
		Modbus: config.ModbusSettings{Address: "tcp://127.0.0.1:" + strconv.Itoa(modbusPort),
			SlaveID: 1, Timeout: time.Second},
		Rollover: config.RolloverSettings{Time: "23:59"},
		Storage: config.StorageSettings{Path: filepath.Join(t.TempDir(), "solis.db"),
			DailyRetention: time.Hour * 24 * 365, MonthlyRetention: time.Hour * 24 * 365,
			YearlyRetention: time.Hour * 24 * 365, ErrorRetention: time.Hour * 24 * 30,
			WalMode: true, Synchronous: "NORMAL", TempStore: "MEMORY", EnableBackup: false,
			CleanupInterval: time.Hour},
	}
}

func getJSON(t *testing.T, url string, v any) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	_ = json.NewDecoder(resp.Body).Decode(v)
	return resp.StatusCode
}

func TestEndToEnd_PollAggregateServe(t *testing.T) {
	dev := &inverter{regs: map[uint16]uint16{
		33035: 125,  // pv_energy_daily 12.5 kWh
		33058: 4200, // pv_total_power
		33095: 3,    // solis_status Generating
		33175: 20,   // grid_export_daily 2.0
		33171: 5,    // grid_import_daily 0.5
	}}
	mport := freePort(t)
	srv, err := sv.NewServer(&sv.ServerConfiguration{URL: "tcp://127.0.0.1:" + strconv.Itoa(mport),
		Timeout: time.Second, MaxClients: 2}, dev)
	require.NoError(t, err)
	require.NoError(t, srv.Start())
	defer func() { _ = srv.Stop() }()

	hport := freePort(t)
	cfg := testConfig(t, mport, hport)
	base := fmt.Sprintf("http://127.0.0.1:%d", hport)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, zerolog.Nop()) }()

	var snap health.Snapshot
	require.Eventually(t, func() bool {
		return getJSON(t, base+"/health", &snap) == http.StatusOK &&
			snap.Components["modbus"].State == "healthy" &&
			snap.Components["poller"].State == "healthy"
	}, 10*time.Second, 20*time.Millisecond)

	// Aggregator computed the month from the stored daily row.
	var monthly map[string]any
	require.Eventually(t, func() bool {
		return getJSON(t, base+"/api/data/pv_energy_monthly", &monthly) == http.StatusOK
	}, 10*time.Second, 20*time.Millisecond)
	assert.InDelta(t, 12.5, monthly["value"], 1e-9)
	var net map[string]any
	require.Equal(t, http.StatusOK, getJSON(t, base+"/api/data/grid_energy_daily", &net))
	assert.InDelta(t, 1.5, net["value"], 1e-9)

	// WebSocket: subscribe, snapshot, then a diff update after the device changes.
	expectWSUpdate(t, dev, hport)

	// A running server blocks maintenance jobs.
	_, err = maintenance.AcquireExclusive(cfg.Storage.Path)
	assert.ErrorIs(t, err, maintenance.ErrLocked)

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err, "cancelled parent context is a clean shutdown")
	case <-time.After(15 * time.Second):
		t.Fatal("app did not shut down")
	}
}

// expectWSUpdate subscribes to pv_total_power and waits for the update that follows a
// register change on the fake device.
func expectWSUpdate(t *testing.T, dev *inverter, hport int) {
	t.Helper()
	conn, resp, err := gws.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", hport), nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.WriteJSON(map[string]any{"type": "subscribe",
		"keys": []string{"pv_total_power"}}))
	var frame map[string]any
	require.NoError(t, conn.ReadJSON(&frame))
	assert.Equal(t, "snapshot", frame["type"])

	dev.mu.Lock()
	dev.regs[33058] = 1234
	dev.mu.Unlock()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	for {
		require.NoError(t, conn.ReadJSON(&frame))
		v, _ := frame["values"].(map[string]any)["pv_total_power"].(map[string]any)
		if frame["type"] == "update" && v != nil && v["value"] == 1234.0 {
			return
		}
	}
}

func TestRun_StartupErrorsAbort(t *testing.T) {
	cfg := testConfig(t, freePort(t), freePort(t))
	cfg.Rollover.Time = "25:00"
	err := Run(context.Background(), cfg, zerolog.Nop())
	require.Error(t, err)

	// Busy HTTP port aborts startup before any component starts.
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	cfg = testConfig(t, freePort(t), l.Addr().(*net.TCPAddr).Port)
	assert.Error(t, Run(context.Background(), cfg, zerolog.Nop()))

	// A maintenance job holding the lock prevents startup.
	cfg = testConfig(t, freePort(t), freePort(t))
	job, err := maintenance.AcquireExclusive(cfg.Storage.Path)
	require.NoError(t, err)
	defer func() { _ = job.Release() }()
	assert.True(t, errors.Is(Run(context.Background(), cfg, zerolog.Nop()), maintenance.ErrLocked))
}
