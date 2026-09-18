// Package routes defines the HTTP route configuration for the Solis monitor API.
package routes

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dombyte/solis/internal/http/handlers"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// cacheMiddleware sets cache headers based on the request path
func cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Assets: immutable, long cache
		if strings.HasPrefix(path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			// Index and SPA routes: no-store
		} else if path == "/" || (strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "/api/") &&
			!strings.HasPrefix(path, "/health") && !strings.HasPrefix(path, "/ws") &&
			!strings.HasPrefix(path, "/docs")) {
			w.Header().Set("Cache-Control", "no-store")
			// Manifest and data: no-cache
		} else if strings.HasPrefix(path, "/manifest.webmanifest") || strings.HasPrefix(path, "/data/") {
			w.Header().Set("Cache-Control", "no-cache")
			// sw.js: no-cache with max-age=0
		} else if strings.HasPrefix(path, "/sw.js") {
			w.Header().Set("Cache-Control", "no-cache, max-age=0")
			// Icons and static images: short cache
		} else if strings.HasPrefix(path, "/favicon") || strings.HasPrefix(path, "/vite.svg") ||
			strings.HasPrefix(path, "/pwa-") || strings.HasPrefix(path, "/apple-touch") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}

		next.ServeHTTP(w, r)
	})
}

// HandlerDeps contains dependencies for HTTP handlers.
type HandlerDeps struct {
	// Service is the service layer for business logic.
	Service *service.ReadService
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
	// Add cache headers middleware
	r.Use(cacheMiddleware)

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

		// Serve data directory (licenses.json, version.json, etc.)
		r.Handle("/data/*", http.StripPrefix("/data/", http.FileServer(http.Dir(filepath.Join(frontendDist, "data")))))

		// Serve manifest and sw.js
		r.Handle("/manifest.webmanifest", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/sw.js", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/vite.svg", http.FileServer(http.Dir(frontendDist)))

		// Serve icon files
		r.Handle("/favicon.ico", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/favicon.svg", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/pwa-64x64.png", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/pwa-192x192.png", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/pwa-512x512.png", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/maskable-icon-512x512.png", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/apple-touch-icon-180x180.png", http.FileServer(http.Dir(frontendDist)))
		r.Handle("/apple-touch-icon.png", http.FileServer(http.Dir(frontendDist)))

		// For all other root-level requests, serve index.html
		// This allows the frontend router to handle client-side routing
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(frontendDist, "index.html"))
		})
	}

	handlerDeps := handlers.HandlerDeps{
		Service: deps.Service, // *service.ReadService implements handlers.ReadServiceInterface
	}

	// Serve static files from docs/dist directory
	docsDist := "./docs/dist"
	if _, err := os.Stat(docsDist); err == nil {
		// Serve Swagger UI and documentation files
		// Redirect /docs to /docs/ for consistency
		r.Handle("/docs", http.RedirectHandler("/docs/", http.StatusMovedPermanently))
		r.Handle("/docs/*", http.StripPrefix("/docs/", http.FileServer(http.Dir(docsDist))))
	}

	// WebSocket endpoint for real-time updates
	if deps.WebSocketHub != nil {
		r.Handle("/ws", websocket.Handler(deps.WebSocketHub))
		r.Handle("/ws/", websocket.Handler(deps.WebSocketHub))
	}

	// Health check endpoint
	r.Get("/health", handlers.GetHealthHandler(handlerDeps))

	// New API endpoints at /api/
	r.Route("/api", func(r chi.Router) {

		// All register  keys with metadata (excludes daily, monthly, yearly, total)
		r.Get("/keys", handlers.GetKeysHandler(handlerDeps))

		// Data for specific register key - supports historical queries with start/end
		r.Get("/data/{key}", handlers.GetDataHandler(handlerDeps))
	})

	// Frontend catch-all handler for client-side routing (SPA support)
	// Serve index.html only for frontend routes (not API, docs, health, etc.)
	if _, err := os.Stat(frontendDist); err == nil {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			// Only serve index.html for GET requests to frontend routes
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			// Check if this is a backend route that shouldn't serve index.html
			path := r.URL.Path
			backendPrefixes := []string{"/api/", "/health", "/ws", "/docs"}
			for _, prefix := range backendPrefixes {
				if strings.HasPrefix(path, prefix) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
			}

			// This is a frontend route, serve index.html for SPA routing
			http.ServeFile(w, r, filepath.Join(frontendDist, "index.html"))
		})
	}

	return r
}

// SetupRoutes is a convenience function to set up all routes.
func SetupRoutes(deps HandlerDeps) *chi.Mux {
	return NewRouter(deps)
}
