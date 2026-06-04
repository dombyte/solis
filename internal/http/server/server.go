// Package server provides HTTP server lifecycle management for the Solis monitor API.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
	"github.com/go-chi/chi/v5"
)

// logger is the package-level logger for server operations.
var logger = logging.NewComponentLogger("http.server")

// Server is the HTTP server for the Solis monitor API.
type Server struct {
	// config holds the app configuration (contains port and timeout).
	config *config.AppSettings
	// router is the Chi router with all routes configured.
	router *chi.Mux
	// httpServer is the underlying HTTP server.
	httpServer *http.Server
}

// New creates a new HTTP server instance.
func New(cfg *config.AppSettings, router *chi.Mux) *Server {
	return &Server{
		config: cfg,
		router: router,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Port),
			Handler:      router,
			ReadTimeout:  cfg.Timeout,
			WriteTimeout: cfg.Timeout,
			IdleTimeout:  cfg.Timeout * 2,
		},
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	logger.Info().Msgf("Starting HTTP server on %s", s.httpServer.Addr)

	// Start server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				logger.Error().Msgf("HTTP server error: %v", err)
			}
		}
	}()

	logger.Info().Msgf("HTTP server is running on port %d", s.config.Port)
	return nil
}

// Stop stops the HTTP server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	logger.Info().Msg("Stopping HTTP server...")

	// Create a context with timeout
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(stopCtx); err != nil {
		logger.Error().Msgf("HTTP server shutdown error: %v", err)
		return err
	}

	logger.Info().Msg("HTTP server stopped")
	return nil
}
