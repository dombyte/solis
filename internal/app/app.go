// Package app is the composition root: the only package that names concrete types and
// wires them together. It builds the root logger children, creates the non-restartable
// parts (storage, cache, event bus, HTTP server), registers the restartable components
// with the health supervisor and runs until the root context is cancelled.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/aggregator"
	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/http/handlers"
	"github.com/dombyte/solis/internal/http/routes"
	"github.com/dombyte/solis/internal/http/server"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/maintenance"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/utils"
	"github.com/dombyte/solis/internal/websocket"
)

// probeTimeout bounds the storage health probe (runs inside the supervisor sweep).
const probeTimeout = 2 * time.Second

// App holds the long-lived parts of one application run.
type App struct {
	cfg   *config.AppConfig
	log   zerolog.Logger
	clock utils.Clock

	lock    *maintenance.Lock
	reg     *solis.Registry
	decoder *solis.Decoder
	dbm     *database.Manager
	store   *storage.Storage
	bus     *eventbus.Bus
	cache   *cache.Cache
	sup     *health.Supervisor
	http    *server.Server
}

// Run builds the application, serves until a signal or a fatal health escalation and
// shuts down gracefully. It returns nil on a clean shutdown and the wrapped
// health.ErrHealthFatal (or a startup error) otherwise, so main() restarts the app.
func Run(ctx context.Context, cfg *config.AppConfig, root zerolog.Logger) (err error) {
	a := &App{cfg: cfg, log: logging.Component(root, "app"), clock: utils.NewRealClock()}
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

func (a *App) buildStorage(ctx context.Context, root zerolog.Logger) error {
	a.log.Debug().Str("path", a.cfg.Storage.Path).Msg("building storage")
	lock, err := maintenance.AcquireShared(a.cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("app: %w (is a maintenance job running?)", err)
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
	a.store, err = storage.New(StorageSettings(*st), a.reg, a.clock,
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
	reader := &utils.Slot[poller.Reader]{}
	a.sup.Manage(nameModbus, CreateModbus(a.cfg.Modbus, iv, reader, a.clock,
		logging.Component(root, nameModbus)))
	a.sup.Manage(namePoller, CreatePoller(poller.Deps{
		Settings: a.cfg.Poller, Rollover: roll,
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
	hubs := &utils.Slot[*websocket.Hub]{}
	a.sup.Manage(nameHub, CreateHub(websocket.HubDeps{
		Bus: a.bus, Cache: a.cache, Keys: a.reg,
		Clock: a.clock, PollInterval: a.cfg.Poller.Interval,
		Log: logging.Component(root, nameHub),
	}, hubs))
	svc := service.NewReadService(service.Deps{
		Store: a.store, Cache: a.cache, Health: a.sup,
		Registry: a.reg, Decoder: a.decoder, Log: logging.Component(root, "service"),
	})
	httpLog := logging.Component(root, "http")
	router := routes.SetupRoutes(routes.Deps{
		Handlers: handlers.HandlerDeps{
			Service: svc, Errors: handlers.NewErrorMapper(httpLog),
			Clock: a.clock, Timeout: a.cfg.App.Timeout,
		},
		WebSocket: websocket.NewHandler(hubs, httpLog), Log: httpLog,
	})
	a.http = server.New(&a.cfg.App, router, httpLog)
	a.sup.Watch("storage", a.storageProbe)
	a.sup.Watch("eventbus", a.busProbe)
	a.sup.Watch("http", a.http.Probe)
	return a.http.Start()
}

func (a *App) storageProbe() (health.State, string) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
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

// serve runs the supervisor until a signal (clean) or a fatal escalation.
func (a *App) serve() error {
	ctx := a.sup.Context()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		select {
		case s := <-sig:
			a.log.Info().Str("signal", s.String()).Msg("shutdown requested")
			a.sup.Shutdown()
		case <-ctx.Done():
		}
	}()
	go a.dbm.RunPeriodicBackups(ctx)
	go a.dbm.RunPeriodicCleanup(ctx, a.store)
	a.logStartup()

	a.sup.Run() // returns after stopping components in reverse order
	cause := context.Cause(ctx)
	if errors.Is(cause, health.ErrHealthFatal) {
		a.log.Error().Err(cause).Msg("health fatal: restarting application")
		return cause
	}
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
