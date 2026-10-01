// Package router defines the HTTP route configuration for the Solis monitor API.
package router

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/http/httphandler"
	"github.com/dombyte/solis/internal/http/middleware"
)

// Deps are the router dependencies.
type Deps struct {
	// Handlers are the API handler dependencies.
	Handlers httphandler.HandlerDeps
	// WebSocket serves /ws.
	WebSocket http.Handler
	// Frontend and Docs are the built SPA and Swagger UI (embedded in the binary); each is
	// only mounted when it has an index.html.
	Frontend fs.FS
	Docs     fs.FS
	Log      zerolog.Logger
}

// backendPrefixes never fall through to the SPA.
func backendPrefixes() []string {
	return []string{"/api", "/health", "/ws", "/docs"}
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
	r := chi.NewRouter()
	r.Use(chimw.RequestID) // first, so the logger and recoverer see the id
	r.Use(middleware.Recover(d.Log))
	r.Use(middleware.Logger(d.Log))
	r.Use(middleware.SecurityHeaders)
	r.Use(cacheHeaders)
	// No CORS: the SPA and the dev server proxy are same-origin, so a permissive
	// Access-Control-Allow-Origin would only let other websites read the data.

	if d.WebSocket != nil {
		r.Handle("/ws", d.WebSocket)
		r.Handle("/ws/", d.WebSocket)
	}
	r.Method(http.MethodGet, "/health", httphandler.GetHealthHandler(d.Handlers))
	r.Route("/api", func(r chi.Router) {
		r.Method(http.MethodGet, "/keys", httphandler.GetKeysHandler(d.Handlers))
		r.Method(http.MethodGet, "/version", httphandler.GetVersionHandler(d.Handlers))
		r.Method(http.MethodGet, "/data/{key}", httphandler.GetDataHandler(d.Handlers))
	})
	mountDocs(r, d.Docs)
	mountFrontend(r, d.Frontend)
	return r
}

// HasIndex reports whether fsys holds an index.html (a built SPA or Swagger UI).
func HasIndex(fsys fs.FS) bool {
	if fsys == nil {
		return false
	}
	_, err := fs.Stat(fsys, "index.html")
	return err == nil
}

func mountDocs(r chi.Router, fsys fs.FS) {
	if !HasIndex(fsys) {
		return
	}
	r.Handle("/docs", http.RedirectHandler("/docs/", http.StatusMovedPermanently))
	r.Handle("/docs/*", http.StripPrefix("/docs/", staticDir(fsys, ".")))
}

func mountFrontend(r chi.Router, fsys fs.FS) {
	if !HasIndex(fsys) {
		return
	}
	r.Handle("/assets/*", http.StripPrefix("/assets/", staticDir(fsys, "assets")))
	r.Handle("/data/*", http.StripPrefix("/data/", staticDir(fsys, "data")))
	for name, ct := range staticFiles() {
		r.Handle("/"+name, serveFile(fsys, name, ct))
	}
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFileFS(w, req, fsys, "index.html")
	})
	r.NotFound(spaFallback(fsys))
}

// staticDir serves the files below dir of fsys without directory listings or dotfiles:
// a directory is only served when it has an index.html, anything else is a 404.
func staticDir(fsys fs.FS, dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := fsPath(r.URL.Path)
		if hiddenPath(rel) {
			http.NotFound(w, r)
			return
		}
		name := path.Join(dir, rel)
		st, err := fs.Stat(fsys, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if st.IsDir() {
			name = path.Join(name, "index.html")
			if _, err := fs.Stat(fsys, name); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		// #nosec G703 -- name is Join(dir, fsPath(URL.Path)): cleaned and rooted in fsys
		http.ServeFileFS(w, r, fsys, name)
	})
}

// fsPath turns a URL path into an fs.FS name: cleaned and rooted, so no ".." survives,
// without the leading slash ("." for the root).
func fsPath(urlPath string) string {
	clean := path.Clean("/" + urlPath)
	if clean == "/" {
		return "."
	}
	return clean[1:]
}

// hiddenPath reports whether any segment of the fs path is a dotfile ("." is the root).
func hiddenPath(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if seg != "." && strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func serveFile(fsys fs.FS, name, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		http.ServeFileFS(w, r, fsys, name)
	})
}

// spaFallback serves existing files of fsys and index.html for client-side routes.
func spaFallback(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || isBackendPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		name := fsPath(r.URL.Path)
		if st, err := fs.Stat(fsys, name); err == nil && !st.IsDir() && !hiddenPath(name) {
			// #nosec G703 -- name is fsPath(URL.Path): cleaned and rooted in fsys
			http.ServeFileFS(w, r, fsys, name)
			return
		}
		http.ServeFileFS(w, r, fsys, "index.html")
	}
}

// isBackendPath reports whether path is a backend root or below it. Whole segments only:
// a plain prefix match on "/health" would also swallow SPA routes like /healthz.
func isBackendPath(path string) bool {
	for _, root := range backendPrefixes() {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
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
		case !isBackendPath(p):
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
