// Package routes defines the HTTP route configuration for the Solis monitor API.
package routes

import (
	"net/http"
	"os"
	"path/filepath"

	_ "github.com/dombyte/solis/docs"
	"github.com/dombyte/solis/internal/http/handlers"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// HandlerDeps contains dependencies for HTTP handlers.
type HandlerDeps struct {
	// Service is the service layer for business logic.
	Service *service.ReadService
	// MetricsEnabled indicates whether Prometheus metrics are enabled.
	MetricsEnabled bool
	// WebSocketHub is the WebSocket hub for real-time updates.
	WebSocketHub *websocket.Hub
}

// NewRouter creates a new Chi router with all routes configured.
func NewRouter(deps HandlerDeps) *chi.Mux {
	r := chi.NewRouter()

	// Add common middleware
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	// Add panic recovery middleware (redundant with Recoverer but more specific logging)
	r.Use(handlers.PanicRecoveryMiddleware)

	// Add CORS middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Serve static files from frontend/dist directory
	frontendDist := "./frontend/dist"
	if _, err := os.Stat(frontendDist); err == nil {
		// Serve static assets (js, css) from /assets/*
		assetsFS := http.FileServer(http.Dir(filepath.Join(frontendDist, "assets")))
		r.Handle("/assets/*", http.StripPrefix("/assets/", assetsFS))

		// Serve favicon from root
		r.Handle("/favicon.svg", http.FileServer(http.Dir(frontendDist)))

		// For all other root-level requests, serve index.html
		// This allows the frontend router to handle client-side routing
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(frontendDist, "index.html"))
		})
	}

	handlerDeps := handlers.HandlerDeps{
		Service: deps.Service, // *service.ReadService implements handlers.ReadServiceInterface
	}

	// WebSocket endpoint for real-time updates
	if deps.WebSocketHub != nil {
		r.Handle("/ws", websocket.Handler(deps.WebSocketHub))
		r.Handle("/ws/", websocket.Handler(deps.WebSocketHub))
	}

	// Health check endpoint
	r.Get("/health", handlers.GetHealthHandler(handlerDeps))

	// Prometheus metrics endpoint (only if enabled in config)
	if deps.MetricsEnabled {
		r.Get("/metrics", handlers.GetMetricsHandler(handlerDeps))
	}

	// Swagger UI at /docs
	// The http-swagger handler serves the UI and JSON automatically
	// It reads the registered docs from the swag library
	// Redirect /docs to /docs/ for proper handler matching
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
	})
	// Mount the Swagger handler at /docs/ to ensure the prefix is set correctly
	r.HandleFunc("/docs/*", httpSwagger.Handler(
		httpSwagger.URL("doc.json"),
	))

	// New API endpoints at /api/
	r.Route("/api", func(r chi.Router) {

		// All register  keys with metadata (excludes daily, monthly, yearly, total)
		r.Get("/keys", handlers.GetKeysHandler(handlerDeps))

		// Data for specific register key - supports historical queries with start/end
		r.Get("/data/{key}", handlers.GetDataHandler(handlerDeps))

		// History endpoints for aggregated data
		r.Route("/history", func(r chi.Router) {
			// Daily history
			r.Get("/daily/{key}", handlers.GetDailyHandler(handlerDeps))
			// Monthly history
			r.Get("/monthly/{key}", handlers.GetMonthlyHandler(handlerDeps))
			// Yearly history
			r.Get("/yearly/{key}", handlers.GetYearlyHandler(handlerDeps))
			// Total (lifetime) history
			r.Get("/total/{key}", handlers.GetTotalHandler(handlerDeps))
		})
	})

	// Frontend catch-all handler for client-side routing (SPA support)
	// Serve index.html for all unmatched GET requests
	if _, err := os.Stat(frontendDist); err == nil {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			// Only serve index.html for GET requests
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			http.ServeFile(w, r, filepath.Join(frontendDist, "index.html"))
		})
	}

	return r
}

// SetupRoutes is a convenience function to set up all routes.
func SetupRoutes(deps HandlerDeps) *chi.Mux {
	return NewRouter(deps)
}
