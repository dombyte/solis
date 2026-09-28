// Package app is the composition root: the only package that names concrete types and
// wires them together. It builds the root logger children, creates the non-restartable
// parts (storage, cache, event bus, HTTP server), registers the restartable components
// with the health supervisor and runs until the root context is cancelled. It never
// restarts itself: a fatal health escalation makes Run return an error so the process
// exits and the container runtime (Docker/Podman restart policy) restarts it.
package app

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/aggregator"
	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/http/httphandler"
	"github.com/dombyte/solis/internal/http/router"
	"github.com/dombyte/solis/internal/http/server"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/maintenance"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/util"
	"github.com/dombyte/solis/internal/websocket"
)

// probeTimeout bounds the storage health probe (runs inside the supervisor sweep).
const probeTimeout = 2 * time.Second

// App holds the long-lived parts of one application run.
type App struct {
	cfg   *config.AppConfig
	log   zerolog.Logger
	clock util.Clock

	lock    *maintenance.Lock
	reg     *solis.Registry
	decoder *solis.Decoder
	dbm     *database.Manager
	store   *storage.Storage
	bus     *eventbus.Bus
	cache   *cache.Cache
	sup     *health.Supervisor
	http    *server.Server

	stopping     chan struct{} // closed once the shutdown began (signal or fatal)
	stoppingOnce sync.Once
}

// Run builds the application and serves until ctx is cancelled (a signal: clean
// shutdown, nil) or a fatal health escalation (the wrapped health.ErrHealthFatal); a
// startup error is returned as is. Once the shutdown began it is bounded by
// ShutdownTimeout; on ErrShutdownTimeout the caller must exit the process. Run never
// restarts the app: every non-nil result means "exit non-zero".
func Run(ctx context.Context, cfg *config.AppConfig, root zerolog.Logger) error {
	a := &App{
		cfg: cfg, log: logging.Component(root, "app"), clock: util.NewRealClock(),
		stopping: make(chan struct{}),
	}
	stop := context.AfterFunc(ctx, a.beginShutdown)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- a.runRecovered(ctx, root) }()
	return awaitShutdown(done, a.stopping, a.clock, ShutdownTimeout)
}

// beginShutdown starts the ShutdownTimeout clock (idempotent).
func (a *App) beginShutdown() {
	a.stoppingOnce.Do(func() { close(a.stopping) })
}

// runRecovered runs one application lifetime and turns a panic on this goroutine into
// an error. The deferred close runs first while unwinding, so the lock and the database
// are released either way.
func (a *App) runRecovered(ctx context.Context, root zerolog.Logger) (err error) {
	defer func() {
		if p := recover(); p != nil {
			a.log.Error().Interface("panic", p).Bytes("stack", debug.Stack()).Msg("app panic")
			err = errors.Join(fmt.Errorf("app: panic: %v", p), err)
		}
	}()
	defer func() { err = errors.Join(err, a.close()) }()
	if err := a.build(ctx, root); err != nil {
		return err
	}
	return a.serve()
}

// build creates every part; any error aborts startup.
func (a *App) build(ctx context.Context, root zerolog.Logger) error {
	steps := []func(context.Context, zerolog.Logger) error{
		a.buildStorage, a.buildCore, a.buildSupervisor, a.buildHTTP,
	}
	for _, step := range steps {
		if err := step(ctx, root); err != nil {
			return err
		}
	}
	return nil
}

// acquireLock takes the server's shared lock next to the database, with a hint that
// tells a running maintenance job apart from an unwritable data directory.
func acquireLock(dbPath string) (*maintenance.Lock, error) {
	lock, err := maintenance.AcquireShared(dbPath)
	if errors.Is(err, maintenance.ErrLocked) {
		return nil, fmt.Errorf("app: %w (is a maintenance job running?)", err)
	}
	if err != nil {
		return nil, fmt.Errorf("app: %w (is the data directory writable?)", err)
	}
	return lock, nil
}

func (a *App) buildStorage(ctx context.Context, root zerolog.Logger) error {
	a.log.Debug().Str("path", a.cfg.Storage.Path).Msg("building storage")
	lock, err := acquireLock(a.cfg.Storage.Path)
	if err != nil {
		return err
	}
	a.lock = lock
	if a.reg, err = solis.NewRegistry(); err != nil {
		return err
	}
	a.log.Debug().Msg("registry created")
	st := &a.cfg.Storage
	a.dbm = database.NewManager(DatabaseSettings(*st), &database.BackupConfig{
		Enabled:    st.EnableBackup,
		MaxBackups: st.MaxBackups, BackupInterval: st.BackupInterval,
	}, a.clock,
		logging.Component(root, "database"))
	if err := a.dbm.Prepare(ctx); err != nil {
		return fmt.Errorf("app: prepare database: %w", err)
	}
	a.log.Debug().Msg("database prepared")
	a.store, err = storage.New(ctx, StorageSettings(*st), a.reg, a.clock,
		logging.Component(root, "storage"))
	if err != nil {
		return err
	}
	a.log.Debug().Msg("storage created")
	day, created, err := a.store.EnsureCutover(ctx, period.Of(a.clock.Now()))
	if err != nil {
		return fmt.Errorf("app: record v3 cutover: %w", err)
	}
	if created {
		a.log.Info().Str("cutover", day).Msg("v3 cutover recorded; earlier periods are frozen")
	}
	return nil
}

func (a *App) buildCore(_ context.Context, root zerolog.Logger) error {
	a.log.Debug().Msg("building core components")
	a.decoder = solis.NewDecoder(a.reg, logging.Component(root, "decoder"))
	a.log.Debug().Msg("decoder created")
	a.bus = eventbus.New()
	a.cache = cache.New(a.bus, logging.Component(root, "cache"))
	a.log.Debug().Msg("cache created")
	return nil
}

// Supervised component names (also the /health keys).
const (
	nameModbus     = "modbus"
	namePoller     = "poller"
	nameAggregator = "aggregator"
	nameHub        = "websocket"
)

func (a *App) buildSupervisor(ctx context.Context, root zerolog.Logger) error {
	roll, err := period.ParseRollover(a.cfg.Rollover.Time)
	if err != nil {
		return err
	}
	iv := a.cfg.Poller.Interval
	a.sup = health.New(ctx, iv, a.clock, logging.Component(root, "health"))
	context.AfterFunc(a.sup.Context(), a.beginShutdown) // fatal escalation or signal
	reader := &util.Slot[poller.Reader]{}
	a.sup.Manage(nameModbus, CreateModbus(modbusSettings(a.cfg.Modbus), iv, reader, a.clock,
		logging.Component(root, nameModbus)))
	a.sup.Manage(namePoller, CreatePoller(poller.Deps{
		Settings: pollerSettings(a.cfg.Poller), Rollover: roll,
		Source: reader, Store: a.store, Cache: a.cache, Bus: a.bus, Decoder: a.decoder,
		Registry: a.reg, Clock: a.clock, Timeout: a.cfg.App.Timeout,
		Log: logging.Component(root, namePoller),
	}))
	a.sup.Manage(nameAggregator, CreateAggregator(aggregator.Deps{
		Store: a.store, Cache: a.cache,
		Bus: a.bus, Registry: a.reg, Clock: a.clock, PollInterval: iv, Timeout: a.cfg.App.Timeout,
		Log: logging.Component(root, nameAggregator),
	}))
	return nil
}

func (a *App) buildHTTP(_ context.Context, root zerolog.Logger) error {
	hubs := &util.Slot[*websocket.Hub]{}
	a.sup.Manage(nameHub, CreateHub(websocket.HubDeps{
		Bus: a.bus, Cache: a.cache, Keys: a.reg,
		Clock: a.clock, PollInterval: a.cfg.Poller.Interval,
		Log: logging.Component(root, nameHub),
	}, hubs))
	svc, err := service.NewReadService(service.Deps{
		Store: a.store, Cache: a.cache, Health: a.sup,
		Registry: a.reg, Decoder: a.decoder, Log: logging.Component(root, "service"),
	})
	if err != nil {
		return err
	}
	httpLog := logging.Component(root, "http")
	ws, err := websocket.NewHandler(hubs, httpLog)
	if err != nil {
		return err
	}
	mux := router.SetupRoutes(router.Deps{
		Handlers: httphandler.HandlerDeps{
			Service: svc, Errors: httphandler.NewErrorMapper(httpLog),
			Clock: a.clock, Timeout: a.cfg.App.Timeout,
		},
		WebSocket: ws, Log: httpLog,
	})
	a.http = server.New(serverSettings(a.cfg.App), mux, httpLog)
	a.sup.Watch("storage", a.storageProbe)
	a.sup.Watch("eventbus", a.busProbe)
	a.sup.Watch("http", a.http.Probe)
	return a.http.Start()
}

func (a *App) storageProbe() (health.State, string) {
	// Bound to the root context: during shutdown the probe stops at once instead of
	// holding the supervisor lock for the full timeout (review RT-L6).
	ctx, cancel := context.WithTimeout(a.sup.Context(), probeTimeout)
	defer cancel()
	if err := a.store.Ping(ctx); err != nil {
		return health.Failed, err.Error()
	}
	return health.Healthy, ""
}

func (a *App) busProbe() (health.State, string) {
	if a.bus.Closed() {
		return health.Failed, "event bus closed"
	}
	return health.Healthy, ""
}

// serve runs the supervisor until the parent context is cancelled (clean) or a fatal
// escalation, then waits for the periodic database jobs so close() never closes the
// store or releases the lock under a running backup or cleanup.
func (a *App) serve() error {
	ctx := a.sup.Context()
	var jobs sync.WaitGroup
	jobs.Go(func() { a.dbm.RunPeriodicBackups(ctx) })
	jobs.Go(func() { a.dbm.RunPeriodicCleanup(ctx, a.store) })
	a.logStartup()

	a.sup.Run() // returns after stopping components in reverse order
	jobs.Wait()
	cause := context.Cause(ctx)
	if errors.Is(cause, health.ErrHealthFatal) {
		a.log.Error().Err(cause).
			Msg("health fatal: exiting so the container runtime restarts the app")
		return cause
	}
	a.log.Info().Msg("shutdown complete")
	return nil
}

func (a *App) logStartup() {
	a.log.Info().Int("port", a.cfg.App.Port).Dur("poll_interval", a.cfg.Poller.Interval).
		Str("modbus", a.cfg.Modbus.Address).
		Str("rollover", a.cfg.Rollover.Time).Str("tz", time.Local.String()).
		Msg("Solis Monitor started (/api, /ws, /health, /docs)")
}

// close releases everything in reverse creation order (safe on partial builds).
func (a *App) close() error {
	var errs []error
	if a.http != nil {
		errs = append(errs, a.http.Stop(context.Background()))
	}
	if a.bus != nil {
		a.bus.Close()
	}
	if a.store != nil {
		errs = append(errs, a.store.Close())
	}
	if a.lock != nil {
		errs = append(errs, a.lock.Release())
	}
	return errors.Join(errs...)
}
