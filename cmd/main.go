// main.go is the application entry point for the Solis monitor.
// It initializes all components and starts the background poller and HTTP server.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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

	dbManager, st, err := initializeDatabase(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := dbManager.Close(); err != nil {
			logger.Error().Msgf("Error closing database manager: %v", err)
		}
	}()

	startBackgroundServices(cfg, dbManager, st)

	ca, wsHub, agg, err := initializeApplicationServices(cfg, st)
	if err != nil {
		return err
	}

	modbusClient, pl, err := initializeModbusAndPoller(cfg, st, ca, agg)
	if err != nil {
		return err
	}

	httpServer, err := initializeHTTPServices(cfg, st, ca, wsHub, pl, agg, modbusClient)
	if err != nil {
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
func initializeDatabase(cfg *config.AppConfig) (*database.DatabaseManager, *storage.Storage, error) {
	dbManager := database.NewDatabaseManager(
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

// startBackgroundServices starts background database services
func startBackgroundServices(cfg *config.AppConfig, dbManager *database.DatabaseManager, st *storage.Storage) {
	if cfg.Storage.EnableBackup && cfg.Storage.BackupInterval > 0 {
		ctx := context.Background()
		go func() {
			if err := dbManager.StartPeriodicBackups(ctx); err != nil {
				logger.Error().Msgf("Failed to start periodic backups: %v", err)
			}
		}()
		logger.Info().Msgf("Periodic backups started (interval: %s)", cfg.Storage.BackupInterval)
	}

	if cfg.App.ServeOnly && cfg.Storage.CleanupInterval > 0 {
		ctx := context.Background()
		go func() {
			if err := dbManager.StartPeriodicCleanup(ctx); err != nil {
				logger.Error().Msgf("Failed to start periodic cleanup: %v", err)
			}
		}()
		logger.Info().Msgf("Periodic retention cleanup started (interval: %s)", cfg.Storage.CleanupInterval)
	}
}

// initializeApplicationServices initializes cache, websocket hub, and aggregator
func initializeApplicationServices(cfg *config.AppConfig, st *storage.Storage) (*cache.Cache, *websocket.Hub, *aggregator.Aggregator, error) {
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
func initializeModbusAndPoller(cfg *config.AppConfig, st *storage.Storage, ca *cache.Cache, agg *aggregator.Aggregator) (*modbus.Client, *poller.Poller, error) {
	var modbusClient *modbus.Client
	var pl *poller.Poller

	if !cfg.App.ServeOnly {
		var err error
		modbusClient, err = modbus.NewClient(&cfg.Modbus)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Modbus client: %v", err)
		}

		// Pass aggregator to poller so poller can signal when first poll completes
		pl = poller.New(&cfg.Poller, modbusClient, poller.WithStorage(st), poller.WithCache(ca), poller.WithAggregator(agg))
		pl.Start()

		// Start aggregator now that poller has a reference to it
		if agg != nil {
			agg.Start()
			logger.Info().Msgf("Aggregator started with interval: %s", cfg.Aggregator.Interval)
		}

		go modbusClient.StartReconnectionLoop(context.Background())

		logger.Info().Msg("Triggering first poll...")
		go func() {
			if _, err := pl.PollNow(); err != nil {
				logger.Error().Msgf("First poll failed: %v", err)
			} else {
				logger.Info().Msg("First poll completed")
			}
		}()
		logger.Info().Msg("Poller and reconnection loop started")
	} else {
		// Start aggregator even in serve-only mode (it will compute from existing storage)
		if agg != nil {
			agg.Start()
			logger.Info().Msgf("Aggregator started with interval: %s (serve-only mode)", cfg.Aggregator.Interval)
		}
		logger.Info().Msg("Running in serve-only mode - Modbus and poller disabled")
	}

	return modbusClient, pl, nil
}

// initializeHTTPServices initializes HTTP server
func initializeHTTPServices(
	cfg *config.AppConfig,
	st *storage.Storage,
	ca *cache.Cache,
	wsHub *websocket.Hub,
	pl *poller.Poller,
	agg *aggregator.Aggregator,
	modbusClient *modbus.Client,
) (*server.Server, error) {
	readService := service.NewReadService(cfg, modbusClient, st, pl, ca, agg)

	handlerDeps := routes.HandlerDeps{
		Service:      readService,
		WebSocketHub: wsHub,
	}

	router := routes.SetupRoutes(handlerDeps)
	httpServer := server.New(&cfg.App, router)

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
		pl.Stop()
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
	// Run app in a loop with panic recovery - app never exits unless signal received
	for {
		// Recover from panics in runApp itself
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Re-initialize logging in case it failed
					logging.Init(os.Stderr, true, "ERROR")
					logger.Error().Msgf("PANIC in runApp: %v - restarting in 5 seconds...", r)
					time.Sleep(5 * time.Second)
				}
			}()

			if err := runApp(); err != nil {
				logger.Error().Msgf("App failed: %v - restarting in 5 seconds...", err)
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
