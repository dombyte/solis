// Package main is the application entry point for the Solis monitor.
// It initializes all components and starts the background poller and HTTP server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dombyte/solis/internal/aggregator"
	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/http/routes"
	"github.com/dombyte/solis/internal/http/server"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/websocket"
)

// logger is the application logger.
var logger = logging.NewComponentLogger("main")

// runApp contains the main application logic and can be restarted on panic.
// Returns an error if initialization fails (non-recoverable).
// Returns nil if shutdown was requested via signal.
func runApp() error {
	cfg, err := loadAndInitConfig()
	if err != nil {
		return err
	}

	// Create root context for the application
	// This allows proper cancellation of all background services
	appCtx, appCancel := context.WithCancel(context.Background())

	// WaitGroup for tracking background services
	bgWg := &sync.WaitGroup{}

	dbManager, st, err := initializeDatabase(cfg)
	if err != nil {
		appCancel()
		return err
	}
	defer func() {
		// Cancel context first to stop background services
		appCancel()
		// Then wait for them to finish
		logger.Info().Msg("Waiting for background services to complete...")
		bgWg.Wait()
		logger.Info().Msg("All background services completed")
		// Finally close database
		if err := dbManager.Close(); err != nil {
			logger.Error().Msgf("Error closing database manager: %v", err)
		}
	}()

	// Start background services with cancellable context and wait group for tracking
	bgErrs := startBackgroundServices(appCtx, bgWg, cfg, dbManager, st)
	// Log any background service initialization errors
	for _, err := range bgErrs {
		logger.Error().Msgf("Background service initialization error: %v", err)
	}

	ca, wsHub, agg, err := initializeApplicationServices(cfg, st)
	if err != nil {
		appCancel()
		return err
	}
	// Ensure aggregator is stopped on exit
	defer func() {
		if agg != nil {
			agg.Stop()
		}
	}()

	modbusClient, pl, err := initializeModbusAndPoller(cfg, st, ca, agg)
	if err != nil {
		appCancel()
		return err
	}
	// Ensure poller and modbus client are stopped on exit
	defer func() {
		if pl != nil {
			if err := pl.Stop(); err != nil {
				logger.Error().Msgf("Error stopping poller: %v", err)
			}
		}
		if modbusClient != nil {
			modbusClient.StopReconnectionLoop()
			if err := modbusClient.Close(); err != nil {
				logger.Error().Msgf("Error closing modbus client: %v", err)
			}
		}
	}()

	httpServer, err := initializeHTTPServices(HTTPDeps{
		Config:       cfg,
		Storage:      st,
		Cache:        ca,
		WebSocketHub: wsHub,
		Poller:       pl,
		Aggregator:   agg,
		ModbusClient: modbusClient,
	})
	if err != nil {
		appCancel()
		return err
	}

	logStartupInfo(cfg, modbusClient, pl)

	err = waitForShutdown(pl, httpServer, cfg)
	return err
}

// loadAndInitConfig loads configuration and initializes logging
func loadAndInitConfig() (*config.AppConfig, error) {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	logging.Init(os.Stderr, true, cfg.App.Debug)
	logger.Info().Msg("Solis Monitor starting...")

	if err := os.MkdirAll("./data", 0750); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	return cfg, nil
}

// initializeDatabase initializes the database manager and storage
func initializeDatabase(cfg *config.AppConfig) (*database.Manager, *storage.Storage, error) {
	dbManager := database.NewManager(
		&cfg.Storage,
		&database.BackupConfig{
			Enabled:        cfg.Storage.EnableBackup,
			MaxBackups:     cfg.Storage.MaxBackups,
			BackupInterval: cfg.Storage.BackupInterval,
		},
	)

	st, err := dbManager.Initialize()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize database: %v", err)
	}

	logger.Info().Msg("Database initialized with migrations and backup support")
	return dbManager, st, nil
}

// startBackgroundServices starts background database services with proper context and tracking.
// The context allows graceful cancellation of background operations.
// The wait group allows the caller to wait for all background goroutines to complete.
// Returns a list of errors encountered during initialization (for services that
// don't start goroutines).
func startBackgroundServices(
	ctx context.Context,
	wg *sync.WaitGroup,
	cfg *config.AppConfig,
	dbManager *database.Manager,
	st *storage.Storage,
) []error {
	var errors []error

	startBackgroundTask := func(
		enabled, configured bool,
		interval time.Duration,
		taskName string,
		startFunc func(context.Context) error,
	) {
		if !enabled {
			logger.Info().Msgf("Periodic %s disabled by configuration", taskName)
			return
		}
		if !configured {
			logger.Info().Msgf("Periodic %s disabled: %s interval not configured", taskName, taskName)
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info().Msgf("Periodic %s started (interval: %s)", taskName, interval)
			if err := startFunc(ctx); err != nil {
				logger.Error().Msgf("Periodic %s failed: %v", taskName, err)
			}
		}()
	}

	startBackgroundTask(
		cfg.Storage.EnableBackup,
		cfg.Storage.BackupInterval > 0,
		cfg.Storage.BackupInterval,
		"backups",
		dbManager.StartPeriodicBackups,
	)

	startBackgroundTask(
		true,
		cfg.Storage.CleanupInterval > 0,
		cfg.Storage.CleanupInterval,
		"retention cleanup",
		dbManager.StartPeriodicCleanup,
	)

	return errors
}

// initializeApplicationServices initializes cache, websocket hub, and aggregator
func initializeApplicationServices(
	cfg *config.AppConfig,
	st *storage.Storage,
) (*cache.Cache, *websocket.Hub, *aggregator.Aggregator, error) {
	ca := cache.New()

	wsHub := websocket.NewHub()
	go wsHub.Run()
	ca.SetWebSocketHub(wsHub)

	wsHub.SetOnInitialDataRequest(func(client *websocket.Client) {
		ca.SendInitialData(client)
	})

	var agg *aggregator.Aggregator
	if cfg.Aggregator.Interval > 0 {
		// Create aggregator but don't start it yet - we need to pass it to poller first
		agg = aggregator.New(st, ca, &cfg.Aggregator)
		logger.Info().Msgf("Aggregator created with interval: %s", cfg.Aggregator.Interval)
	} else {
		logger.Info().Msg("Aggregator disabled (interval not configured)")
	}

	return ca, wsHub, agg, nil
}

// initializeModbusAndPoller initializes Modbus client and poller if not in serve-only mode
func initializeModbusAndPoller(
	cfg *config.AppConfig,
	st *storage.Storage,
	ca *cache.Cache,
	agg *aggregator.Aggregator,
) (*modbus.Client, *poller.Poller, error) {
	var modbusClient *modbus.Client
	var pl *poller.Poller

	if !cfg.App.ServeOnly {
		var err error
		modbusClient, err = modbus.NewClient(&cfg.Modbus)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Modbus client: %v", err)
		}

		// Pass aggregator to poller so poller can signal when first poll completes
		pl = poller.New(
			&cfg.Poller,
			modbusClient,
			poller.WithStorage(st),
			poller.WithCache(ca),
			poller.WithAggregator(agg),
		)
		if err := pl.Start(); err != nil {
			return nil, nil, fmt.Errorf("failed to start poller: %v", err)
		}

		// Start aggregator now that poller has a reference to it
		if agg != nil {
			agg.Start()
			logger.Info().Msgf("Aggregator started with interval: %s",
				cfg.Aggregator.Interval)
		}

		go modbusClient.StartReconnectionLoop(context.Background())

		// Note: First poll will be triggered automatically by the poller's run loop
		// We don't use PollNow() to avoid sharing Modbus connection with background poller
		// (see AGENTS.md Lesson #1: separate connections for HTTP and poller)
		logger.Info().Msg("Poller and reconnection loop started")
	} else {
		// Start aggregator even in serve-only mode (it will compute from existing storage)
		if agg != nil {
			agg.Start()
			logger.Info().Msgf("Aggregator started with interval: %s (serve-only mode)",
				cfg.Aggregator.Interval)
		}
		logger.Info().Msg("Running in serve-only mode - Modbus and poller disabled")
	}

	return modbusClient, pl, nil
}

// HTTPDeps holds dependencies for HTTP service initialization.
type HTTPDeps struct {
	Config       *config.AppConfig
	Storage      *storage.Storage
	Cache        *cache.Cache
	WebSocketHub *websocket.Hub
	Poller       *poller.Poller
	Aggregator   *aggregator.Aggregator
	ModbusClient *modbus.Client
}

// initializeHTTPServices initializes HTTP server
func initializeHTTPServices(deps HTTPDeps) (*server.Server, error) {
	readService := service.NewReadService(service.ReadServiceConfig{
		Config:       deps.Config,
		ModbusClient: deps.ModbusClient,
		Storage:      deps.Storage,
		Poller:       deps.Poller,
		Cache:        deps.Cache,
		Aggregator:   deps.Aggregator,
	})

	handlerDeps := routes.HandlerDeps{
		Service:      readService,
		WebSocketHub: deps.WebSocketHub,
	}

	router := routes.SetupRoutes(handlerDeps)
	httpServer := server.New(&deps.Config.App, router)

	go func() {
		if err := httpServer.Start(); err != nil {
			logger.Error().Msgf("HTTP server failed: %v", err)
		}
	}()

	return httpServer, nil
}

// logStartupInfo logs information about the started services
func logStartupInfo(cfg *config.AppConfig, modbusClient *modbus.Client, pl *poller.Poller) {
	logger.Info().Msgf("Solis Monitor started successfully!")
	logger.Info().Msgf("  - HTTP server: http://localhost:%d", cfg.App.Port)
	logger.Info().Msgf("  - WebSocket: ws://localhost:%d/ws", cfg.App.Port)
	logger.Info().Msgf("  - API endpoints: /api/*")
	logger.Info().Msgf("  - Health check: /health")
	logger.Info().Msgf("  - API Documentation: /docs")
	if pl != nil {
		logger.Info().Msgf("  - Poller interval: %s", cfg.Poller.Interval)
		logger.Info().Msgf("  - Modbus: %s:%d", cfg.Modbus.Host, cfg.Modbus.Port)
	} else {
		logger.Info().Msg("  - Mode: serve-only (no Modbus polling)")
	}
}

// waitForShutdown waits for shutdown signal and performs cleanup
func waitForShutdown(pl *poller.Poller, httpServer *server.Server, cfg *config.AppConfig) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	logger.Info().Msg("Shutdown signal received")

	if pl != nil {
		logger.Info().Msg("Stopping poller...")
		if err := pl.Stop(); err != nil {
			logger.Error().Msgf("Error stopping poller: %v", err)
		}
	}

	logger.Info().Msg("Stopping HTTP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Stop(ctx); err != nil {
		logger.Error().Msgf("HTTP server shutdown error: %v", err)
	}

	logger.Info().Msg("Solis Monitor stopped")
	return nil
}

func main() {
	// Maximum number of restarts to prevent infinite restart loops
	// This helps with debugging during development
	const maxRestarts = 100
	var restartCount int

	// Run app in a loop with panic recovery - app never exits unless signal received
	for {
		// Check restart count
		if restartCount >= maxRestarts {
			// Re-initialize logging in case it failed
			logging.Init(os.Stderr, true, "ERROR")
			logger.Error().Msgf("Maximum restart count (%d) reached - stopping to prevent "+
				"infinite loop", maxRestarts)
			os.Exit(1)
		}
		restartCount++

		// Recover from panics in runApp itself
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Re-initialize logging in case it failed
					logging.Init(os.Stderr, true, "ERROR")
					logger.Error().Msgf("PANIC in runApp (restart #%d): %v - restarting in "+
						"5 seconds...", restartCount, r)
					time.Sleep(5 * time.Second)
				}
			}()

			if err := runApp(); err != nil {
				logger.Error().Msgf("App failed (restart #%d): %v - restarting in "+
					"5 seconds...", restartCount, err)
				// Re-initialize logging in case it failed
				logging.Init(os.Stderr, true, "ERROR")
				logger.Error().Msg("Waiting 5 seconds before restart...")
				time.Sleep(5 * time.Second)
				return
			}
			// If runApp returns nil, it means shutdown signal was received
			logger.Info().Msg("Clean shutdown")
			os.Exit(0)
		}()
	}
}
