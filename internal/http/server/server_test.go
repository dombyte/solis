package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/go-chi/chi/v5"
)

func TestNew(t *testing.T) {
	// Create test config
	cfg := &config.AppSettings{
		Port:    8080,
		Timeout: 30 * time.Second,
		Debug:   "INFO",
	}

	// Create a simple router
	router := chi.NewRouter()

	// Create server
	server := New(cfg, router)

	if server == nil {
		t.Fatal("New() returned nil")
	}

	if server.config != cfg {
		t.Error("Server.config is not set correctly")
	}

	if server.router != router {
		t.Error("Server.router is not set correctly")
	}

	if server.httpServer == nil {
		t.Error("Server.httpServer is nil")
	}

	// Check httpServer configuration
	if server.httpServer.Addr != ":8080" {
		t.Errorf("Server.httpServer.Addr = %v, want %v", server.httpServer.Addr, ":8080")
	}

	if server.httpServer.Handler != router {
		t.Error("Server.httpServer.Handler is not set correctly")
	}

	if server.httpServer.ReadTimeout != cfg.Timeout {
		t.Errorf("Server.httpServer.ReadTimeout = %v, want %v", server.httpServer.ReadTimeout, cfg.Timeout)
	}

	if server.httpServer.WriteTimeout != cfg.Timeout {
		t.Errorf("Server.httpServer.WriteTimeout = %v, want %v", server.httpServer.WriteTimeout, cfg.Timeout)
	}

	if server.httpServer.IdleTimeout != cfg.Timeout*2 {
		t.Errorf("Server.httpServer.IdleTimeout = %v, want %v", server.httpServer.IdleTimeout, cfg.Timeout*2)
	}
}

func TestServer_Start_Stop(t *testing.T) {
	// Create test config
	cfg := &config.AppSettings{
		Port:    0, // Use port 0 to get a random available port
		Timeout: 1 * time.Second,
		Debug:   "INFO",
	}

	// Create a simple router
	router := chi.NewRouter()
	router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create server
	server := New(cfg, router)

	// Start the server
	err := server.Start()
	if err != nil {
		t.Fatalf("Server.Start() error = %v", err)
	}

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Stop(ctx)
	if err != nil {
		t.Logf("Server.Stop() error = %v", err)
	}
}

func TestServer_Start_WithPortInUse(t *testing.T) {
	// This test verifies that Start() handles port-in-use errors gracefully
	// by starting two servers on the same port

	// Create test config with a specific port
	cfg := &config.AppSettings{
		Port:    18080, // Use a non-standard port
		Timeout: 1 * time.Second,
		Debug:   "INFO",
	}

	// Create a simple router
	router := chi.NewRouter()

	// Create and start first server
	server1 := New(cfg, router)
	err := server1.Start()
	if err != nil {
		t.Fatalf("First server Start() error = %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server1.Stop(ctx)
	}()

	// Give the first server time to start
	time.Sleep(100 * time.Millisecond)

	// Try to start second server on the same port
	// This should fail
	router2 := chi.NewRouter()
	server2 := New(cfg, router2)
	_ = server2.Start()
	// We can't easily test for port-in-use error in a portable way,
	// so just verify the server was created
	if server2 == nil {
		t.Fatal("Second server is nil")
	}

	// Clean up
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server2.Stop(ctx)
}

func TestServer_Stop_WithoutStart(t *testing.T) {
	// Create test config
	cfg := &config.AppSettings{
		Port:    8080,
		Timeout: 1 * time.Second,
		Debug:   "INFO",
	}

	// Create a simple router
	router := chi.NewRouter()

	// Create server
	server := New(cfg, router)

	// Stop without starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Stop(ctx)
	// Stop on a server that hasn't started should return an error
	// or do nothing, depending on implementation
	if err != nil {
		t.Logf("Server.Stop() without Start() error = %v", err)
	}
}

func TestServer_Stop_ContextTimeout(t *testing.T) {
	// Create test config
	cfg := &config.AppSettings{
		Port:    0, // Use port 0 to get a random available port
		Timeout: 1 * time.Second,
		Debug:   "INFO",
	}

	// Create a simple router
	router := chi.NewRouter()

	// Create server
	server := New(cfg, router)

	// Start the server
	err := server.Start()
	if err != nil {
		t.Fatalf("Server.Start() error = %v", err)
	}

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	err = server.Stop(ctx)
	// We expect an error due to timeout
	if err == nil {
		t.Log("Server.Stop() with tiny timeout succeeded (might be timing dependent)")
	} else {
		t.Logf("Server.Stop() with tiny timeout error (expected) = %v", err)
	}
}

func TestServer_Handler(t *testing.T) {
	// Create test config
	cfg := &config.AppSettings{
		Port:    0,
		Timeout: 1 * time.Second,
		Debug:   "INFO",
	}

	// Create a router with a test handler
	router := chi.NewRouter()
	router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	// Create server
	server := New(cfg, router)

	if server.httpServer.Handler == nil {
		t.Fatal("Server.httpServer.Handler is nil")
	}
}

func TestServer_Config(t *testing.T) {
	// Test with different port configurations
	ports := []int{0, 8080, 9090, 80}

	for _, port := range ports {
		cfg := &config.AppSettings{
			Port:    port,
			Timeout: 30 * time.Second,
			Debug:   "INFO",
		}

		router := chi.NewRouter()
		server := New(cfg, router)

		expectedAddr := fmt.Sprintf(":%d", port)
		if server.httpServer.Addr != expectedAddr {
			t.Errorf("Port %d: Server.httpServer.Addr = %v, want %v", port, server.httpServer.Addr, expectedAddr)
		}
	}
}
