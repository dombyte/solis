// Package server manages the HTTP server lifecycle. The server is a non-restartable part:
// if ListenAndServe fails unexpectedly, its health probe reports failed and the
// supervisor escalates to a whole-app restart.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/health"
)

// ShutdownTimeout bounds graceful shutdown.
const ShutdownTimeout = 5 * time.Second

// idleTimeoutFactor scales the request timeout to the keep-alive idle timeout.
const idleTimeoutFactor = 2

// Settings are the listen port and the request timeout.
type Settings struct {
	// Port is the TCP listen port.
	Port int
	// Timeout bounds reading and writing a request (idle connections get twice that).
	Timeout time.Duration
}

// Server wraps http.Server.
type Server struct {
	srv *http.Server
	log zerolog.Logger

	mu     sync.Mutex
	failed error
}

// New creates a server listening on cfg.Port.
func New(cfg Settings, handler http.Handler, log zerolog.Logger) *Server {
	return &Server{
		log: log,
		srv: &http.Server{
			Addr:              ":" + strconv.Itoa(cfg.Port),
			Handler:           handler,
			ReadTimeout:       cfg.Timeout,
			ReadHeaderTimeout: cfg.Timeout,
			WriteTimeout:      cfg.Timeout,
			IdleTimeout:       cfg.Timeout * idleTimeoutFactor,
		},
	}
}

// Start binds the listener synchronously (so a busy port fails startup) and serves in
// the background.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("http server: listen %s: %w", s.srv.Addr, err)
	}
	return s.Serve(ln)
}

// Serve serves on ln in the background.
func (s *Server) Serve(ln net.Listener) error {
	s.log.Info().Str("addr", ln.Addr().String()).Msg("HTTP server listening")
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.failed = err
			s.mu.Unlock()
			s.log.Error().Err(err).Msg("HTTP server failed")
		}
	}()
	return nil
}

// Stop shuts the server down gracefully within ShutdownTimeout.
func (s *Server) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, ShutdownTimeout)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("http server: shutdown: %w", err)
	}
	return nil
}

// Probe reports failed once the server stopped serving unexpectedly.
func (s *Server) Probe() (health.State, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed != nil {
		return health.Failed, s.failed.Error()
	}
	return health.Healthy, ""
}
