// Package router defines the HTTP route configuration for the Solis monitor API.
package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/http/httphandler"
	"github.com/dombyte/solis/internal/http/middleware"
)

// Default locations of the built frontend and docs.
const (
	FrontendDist = "./frontend/dist"
	DocsDist     = "./docs/dist"
)

// Deps are the router dependencies.
type Deps struct {
	// Handlers are the API handler dependencies.
	Handlers httphandler.HandlerDeps
	// WebSocket serves /ws.
	WebSocket http.Handler
	// FrontendDir and DocsDir override the dist folders (tests); empty = defaults.
	FrontendDir string
	DocsDir     string
	Log         zerolog.Logger
}

// backendPrefixes never fall through to the SPA.
func backendPrefixes() []string {
	return []string{"/api/", "/health", "/ws", "/docs"}
}

// staticFiles are the root-level frontend files with their content types.
func staticFiles() map[string]string {
	return map[string]string{
		"manifest.webmanifest": "application/manifest+json", "sw.js": "application/javascript",
		"vite.svg": "image/svg+xml", "favicon.ico": "image/x-icon", "favicon.svg": "image/svg+xml",
		"pwa-64x64.png": "image/png", "pwa-192x192.png": "image/png",
		"pwa-512x512.png": "image/png", "maskable-icon-512x512.png": "image/png",
		"apple-touch-icon-180x180.png": "image/png", "apple-touch-icon.png": "image/png",
	}
}

// SetupRoutes builds the router.
func SetupRoutes(d Deps) *chi.Mux {
	if d.FrontendDir == "" {
		d.FrontendDir = FrontendDist
	}
	if d.DocsDir == "" {
		d.DocsDir = DocsDist
	}
	r := chi.NewRouter()
	r.Use(middleware.Recover(d.Log))
	r.Use(middleware.Logger(d.Log))
	r.Use(chimw.RequestID)
	r.Use(cacheHeaders)
	r.Use(cors)

	if d.WebSocket != nil {
		r.Handle("/ws", d.WebSocket)
		r.Handle("/ws/", d.WebSocket)
	}
	r.Method(http.MethodGet, "/health", httphandler.GetHealthHandler(d.Handlers))
	r.Route("/api", func(r chi.Router) {
		r.Method(http.MethodGet, "/keys", httphandler.GetKeysHandler(d.Handlers))
		r.Method(http.MethodGet, "/data/{key}", httphandler.GetDataHandler(d.Handlers))
	})
	mountDocs(r, d.DocsDir)
	mountFrontend(r, d.FrontendDir)
	return r
}

func mountDocs(r chi.Router, dir string) {
	if _, err := os.Stat(dir); err != nil {
		return
	}
	r.Handle("/docs", http.RedirectHandler("/docs/", http.StatusMovedPermanently))
	r.Handle("/docs/*", http.StripPrefix("/docs/", http.FileServer(http.Dir(dir))))
}

func mountFrontend(r chi.Router, dir string) {
	if _, err := os.Stat(dir); err != nil {
		return
	}
	r.Handle("/assets/*", http.StripPrefix("/assets/",
		http.FileServer(http.Dir(filepath.Join(dir, "assets")))))
	r.Handle("/data/*", http.StripPrefix("/data/",
		http.FileServer(http.Dir(filepath.Join(dir, "data")))))
	for name, ct := range staticFiles() {
		r.Handle("/"+name, serveFile(filepath.Join(dir, name), ct))
	}
	index := filepath.Join(dir, "index.html")
	r.Get("/", func(w http.ResponseWriter, req *http.Request) { http.ServeFile(w, req, index) })
	r.NotFound(spaFallback(dir, index))
}

func serveFile(path, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, path)
	})
}

// spaFallback serves existing files under dir and index.html for client-side routes.
func spaFallback(dir, index string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || hasPrefix(r.URL.Path, backendPrefixes()) {
			http.NotFound(w, r)
			return
		}
		clean := filepath.Clean("/" + r.URL.Path) // rooted: no traversal above dir
		path := filepath.Join(dir, clean)
		// #nosec G703 -- path is Join(dir, Clean("/"+URL.Path)): rooted under dir, no traversal
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.ServeFile(w, r, index)
	}
}

func hasPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// cacheHeaders sets Cache-Control by path.
func cacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(p, "/manifest.webmanifest"), strings.HasPrefix(p, "/data/"):
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(p, "/sw.js"):
			w.Header().Set("Cache-Control", "no-cache, max-age=0")
		case hasPrefix(p, []string{"/favicon", "/vite.svg", "/pwa-", "/apple-touch", "/maskable"}):
			w.Header().Set("Cache-Control", "public, max-age=86400")
		case !hasPrefix(p, backendPrefixes()):
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// cors keeps the v2 permissive CORS headers for the REST API.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
