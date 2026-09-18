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

		// Set cache headers based on path patterns
		switch {
		case strings.HasPrefix(path, "/assets/"):
			// Assets: immutable, long cache
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(path, "/manifest.webmanifest"),
			strings.HasPrefix(path, "/data/"):
			// Manifest and data: no-cache
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(path, "/sw.js"):
			// sw.js: no-cache with max-age=0
			w.Header().Set("Cache-Control", "no-cache, max-age=0")
		case strings.HasPrefix(path, "/favicon"),
			strings.HasPrefix(path, "/vite.svg"),
			strings.HasPrefix(path, "/pwa-"),
			strings.HasPrefix(path, "/apple-touch"):
			// Icons and static images: short cache
			w.Header().Set("Cache-Control", "public, max-age=86400")
		case isFrontendRoute(path):
			// Index and SPA routes: no-store
			w.Header().Set("Cache-Control", "no-store")
		}

		next.ServeHTTP(w, r)
	})
}

// isFrontendRoute checks if the path is a frontend route that should have no-store cache
func isFrontendRoute(path string) bool {
	// Check if path is a frontend route (not API, health, ws, docs, etc.)
	backendPrefixes := []string{"/api/", "/health", "/ws", "/docs", "/assets/",
		"/manifest.webmanifest", "/data/", "/sw.js", "/favicon",
		"/vite.svg", "/pwa-", "/apple-touch"}
	for _, prefix := range backendPrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

// HandlerDeps contains dependencies for HTTP handlers.
type HandlerDeps struct {
	// Service is the service layer for business logic.
	Service *service.ReadService
	// WebSocketHub is the WebSocket hub for real-time updates.
	WebSocketHub *websocket.Hub
}

// serveFileHandler creates a handler that serves a file with the specified content type
func serveFileHandler(basePath, filePath, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		//nolint:gosec - filePath is a constant string, safe from path traversal
		http.ServeFile(w, r, filepath.Join(basePath, filePath))
	})
}

// NewRouter creates a new Chi router with all routes configured.
func NewRouter(deps HandlerDeps) *chi.Mux {
	r := chi.NewRouter()

	// Add common middleware
	// Use custom panic recovery middleware for better logging instead of chi's Recoverer
	r.Use(handlers.PanicRecoveryMiddleware)
	r.Use(middleware.Logger)
	r.Use(middleware.RequestID)
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
		r.Handle("/data/*", http.StripPrefix("/data/",
			http.FileServer(http.Dir(filepath.Join(frontendDist, "data")))))

		// Serve manifest, sw.js, and vite.svg
		r.Handle("/manifest.webmanifest", serveFileHandler(frontendDist, "manifest.webmanifest", "application/manifest+json"))
		r.Handle("/sw.js", serveFileHandler(frontendDist, "sw.js", "application/javascript"))
		r.Handle("/vite.svg", serveFileHandler(frontendDist, "vite.svg", "image/svg+xml"))

		// Serve icon files
		r.Handle("/favicon.ico", serveFileHandler(frontendDist, "favicon.ico", "image/x-icon"))
		r.Handle("/favicon.svg", serveFileHandler(frontendDist, "favicon.svg", "image/svg+xml"))
		r.Handle("/pwa-64x64.png", serveFileHandler(frontendDist, "pwa-64x64.png", "image/png"))
		r.Handle("/pwa-192x192.png", serveFileHandler(frontendDist, "pwa-192x192.png", "image/png"))
		r.Handle("/pwa-512x512.png", serveFileHandler(frontendDist, "pwa-512x512.png", "image/png"))
		r.Handle("/maskable-icon-512x512.png",
			serveFileHandler(frontendDist, "maskable-icon-512x512.png", "image/png"))
		r.Handle("/apple-touch-icon-180x180.png",
			serveFileHandler(frontendDist, "apple-touch-icon-180x180.png", "image/png"))
		r.Handle("/apple-touch-icon.png",
			serveFileHandler(frontendDist, "apple-touch-icon.png", "image/png"))

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

			// Try to serve the requested file from frontend/dist first
			// Sanitize path to prevent path traversal
			cleanPath := filepath.Clean(path)
			if strings.HasPrefix(cleanPath, "..") {
				// Path traversal detected, reject
				w.WriteHeader(http.StatusNotFound)
				return
			}
			filePath := filepath.Join(frontendDist, cleanPath)
			if _, err := os.Stat(filePath); err == nil {
				// File exists, serve it with appropriate content type
				ext := filepath.Ext(filePath)
				switch ext {
				case ".svg":
					w.Header().Set("Content-Type", "image/svg+xml")
				case ".png":
					w.Header().Set("Content-Type", "image/png")
				case ".ico":
					w.Header().Set("Content-Type", "image/x-icon")
				case ".js":
					w.Header().Set("Content-Type", "application/javascript")
				case ".css":
					w.Header().Set("Content-Type", "text/css")
				case ".json":
					w.Header().Set("Content-Type", "application/json")
				case ".html":
					w.Header().Set("Content-Type", "text/html")
				default:
					// Try to detect content type from file
					contentType := http.DetectContentType([]byte{})
					w.Header().Set("Content-Type", contentType)
				}
				http.ServeFile(w, r, filePath)
				return
			}

			// This is a frontend route, serve index.html for SPA routing
			// index.html is a constant string, so this is safe from path traversal
			http.ServeFile(w, r, filepath.Join(frontendDist, "index.html"))
		})
	}

	return r
}

// SetupRoutes is a convenience function to set up all routes.
func SetupRoutes(deps HandlerDeps) *chi.Mux {
	return NewRouter(deps)
}
