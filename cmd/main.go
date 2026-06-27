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

	"github.com/dombyte/solis/internal/cache"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/http/routes"
	"github.com/dombyte/solis/internal/http/server"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/metrics"
	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/poller"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/websocket"
)

// logger is the application logger.
var logger = logging.NewComponentLogger("main")

// runApp contains the main application logic and can be restarted on panic.
// Returns an error if initialization fails (non-recoverable).
// Returns nil if shutdown was requested via signal.
func runApp() error {
	// Load configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logging based on config
	logging.Init(false, os.Stderr, true, cfg.App.Debug)
	logger.Info().Msg("Solis Monitor starting...")

	// Create data directory if it doesn't exist
	if err := os.MkdirAll("./data", 0755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	// Initialize database manager for migrations, backups, and cleanup
	dbManager := database.NewDatabaseManager(
		&cfg.Storage,
		&database.BackupConfig{
			Enabled:        cfg.Storage.EnableBackup,
			MaxBackups:     cfg.Storage.MaxBackups,
			BackupInterval: cfg.Storage.BackupInterval,
		},
	)

	// Initialize storage with migration support
	st, err := dbManager.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %v", err)
	}
	defer func() {
		if err := dbManager.Close(); err != nil {
			logger.Error().Msgf("Error closing database manager: %v", err)
		}
	}()
	logger.Info().Msg("Database initialized with migrations and backup support")

	// Start periodic online backups in background
	if cfg.Storage.EnableBackup && cfg.Storage.BackupInterval > 0 {
		ctx := context.Background()
		go func() {
			if err := dbManager.StartPeriodicBackups(ctx); err != nil {
				logger.Error().Msgf("Failed to start periodic backups: %v", err)
			}
		}()
		logger.Info().Msgf("Periodic backups started (interval: %s)", cfg.Storage.BackupInterval)
	}

	// Initialize cache for latest register values
	ca := cache.New()

	// Initialize WebSocket hub for real-time updates
	wsHub := websocket.NewHub()
	go wsHub.Run()
	ca.SetWebSocketHub(wsHub)

	// Set up callback for when clients request initial data
	wsHub.SetOnInitialDataRequest(func(client *websocket.Client) {
		ca.SendInitialData(client)
	})

	// Initialize register filter for disabled registers
	// Validate disabled keys against known registers
	if len(cfg.Registers.DisabledKeys) > 0 {
		for _, key := range cfg.Registers.DisabledKeys {
			if _, ok := solis.RegisterMapByKey[key]; !ok {
				return fmt.Errorf("unknown register key in disabled_keys: %s. Available keys: %v", key, solis.AllRegisters)
			}
		}
		logger.Info().Msgf("Disabled %d registers: %v", len(cfg.Registers.DisabledKeys), cfg.Registers.DisabledKeys)
	}
	registerFilter := solis.NewRegisterFilter(cfg.Registers.DisabledKeys)

	// Initialize shared Modbus client (Solis inverter only handles one connection at a time)
	// Use AllowDisconnected to allow app to start even if modbus is unavailable
	modbusClient, err := modbus.NewClient(&cfg.Modbus, modbus.WithAllowDisconnected(true))
	if err != nil {
		return fmt.Errorf("failed to create Modbus client: %v", err)
	}
	defer modbusClient.Close()

	// Initialize poller and service only if modbus is connected
	// If modbus is not connected, start reconnection loop but don't start poller
	var pl *poller.Poller

	if modbusClient.IsConnected() {
		pl = poller.New(&cfg.Poller, modbusClient, poller.WithStorage(st), poller.WithCache(ca), poller.WithRegisterFilter(registerFilter))
		pl.Start()
		defer pl.Stop()

		// First poll - trigger immediate poll before HTTP server starts (non-blocking)
		logger.Info().Msg("Triggering first poll...")
		go func() {
			if _, err := pl.PollNow(); err != nil {
				logger.Error().Msgf("First poll failed: %v", err)
			} else {
				logger.Info().Msg("First poll completed")
			}
		}()
	} else {
		logger.Warn().Msg("Modbus not connected, starting background reconnection loop")
		logger.Warn().Msg("Poller will not start until Modbus is connected")
		go modbusClient.StartReconnectionLoop(context.Background())
		// Don't start poller - it needs modbus connection
	}

	// Initialize service (always needed for HTTP endpoints)
	readService := service.NewReadService(cfg, modbusClient, st, pl, ca, registerFilter)

	// Initialize HTTP handlers
	handlerDeps := routes.HandlerDeps{
		Service:        readService,
		MetricsEnabled: cfg.Metrics.Enabled,
		WebSocketHub:   wsHub,
	}

	// Set up routes
	router := routes.SetupRoutes(handlerDeps)

	// Create HTTP server
	httpServer := server.New(&cfg.App, router)

	// Start HTTP server
	go func() {
		if err := httpServer.Start(); err != nil {
			logger.Error().Msgf("HTTP server failed: %v", err)
			// Don't exit, just log - the panic recovery in main will handle it
		}
	}()

	// Initialize Prometheus metrics
	if cfg.Metrics.Enabled {
		metrics.Init(readService)
		logger.Info().Msgf("  - Prometheus metrics: http://localhost:%d/metrics", cfg.App.Port)
	}

	logger.Info().Msgf("Solis Monitor started successfully!")
	logger.Info().Msgf("  - HTTP server: http://localhost:%d", cfg.App.Port)
	logger.Info().Msgf("  - WebSocket: ws://localhost:%d/ws", cfg.App.Port)
	logger.Info().Msgf("  - API endpoints: /api/*")
	logger.Info().Msgf("  - Health check: /health")
	logger.Info().Msgf("  - API Documentation: /docs")
	logger.Info().Msgf("  - Poller interval: %s", cfg.Poller.Interval)
	logger.Info().Msgf("  - Modbus: %s:%d", cfg.Modbus.Host, cfg.Modbus.Port)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	logger.Info().Msg("Shutdown signal received")

	// Shutdown sequence
	// Stop poller if it was started
	if pl != nil {
		logger.Info().Msg("Stopping poller...")
		pl.Stop()
	}

	// Stop Prometheus metrics
	if cfg.Metrics.Enabled {
		logger.Info().Msg("Stopping Prometheus metrics...")
		metrics.Shutdown()
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
					logging.Init(false, os.Stderr, true, "ERROR")
					logger.Error().Msgf("PANIC in runApp: %v - restarting in 5 seconds...", r)
					time.Sleep(5 * time.Second)
				}
			}()

			if err := runApp(); err != nil {
				logger.Error().Msgf("App failed: %v - restarting in 5 seconds...", err)
				// Re-initialize logging in case it failed
				logging.Init(false, os.Stderr, true, "ERROR")
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
