package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/dombyte/solis/internal/buildinfo"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/http/httphandler"
	"github.com/dombyte/solis/internal/http/httphandler/mocks"
	"github.com/dombyte/solis/internal/util/clocktest"
)

func file(content string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(content)} }

func router(t *testing.T) (*mocks.MockReadService, http.Handler) {
	t.Helper()
	fe := fstest.MapFS{
		"index.html":    file("INDEX"),
		"assets/app.js": file("JS"), "assets/.env": file("SECRET"),
		"robots.txt": file("ROBOTS"), "favicon.svg": file("<svg/>"), ".hidden": file("HIDDEN"),
	}
	docs := fstest.MapFS{"index.html": file("DOCS")}
	svc := mocks.NewMockReadService(t)
	ws := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	return svc, SetupRoutes(Deps{
		Handlers: httphandler.HandlerDeps{
			Service: svc,
			Errors:  httphandler.NewErrorMapper(zerolog.Nop()), Clock: clocktest.New(time.Now()),
			Build: buildinfo.Info{Version: "3.1.0", Commit: "<abc>"},
		},
		WebSocket: ws, Frontend: fe, Docs: docs, Log: zerolog.Nop(),
	})
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRoutes(t *testing.T) {
	svc, r := router(t)
	svc.EXPECT().Health().Return(health.Snapshot{Status: health.StatusOK}).Once()
	assert.Equal(t, http.StatusOK, get(r, "/health").Code)
	assert.Equal(t, http.StatusTeapot, get(r, "/ws").Code)

	rec := get(r, "/")
	assert.Equal(t, "INDEX", rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	// No CORS (review HTTP-M2); security headers on every response (HTTP-L10).
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))

	rec = get(r, "/assets/app.js")
	assert.Equal(t, "JS", rec.Body.String())
	assert.Contains(t, rec.Header().Get("Cache-Control"), "immutable")

	assert.Equal(t, "ROBOTS", get(r, "/robots.txt").Body.String())
	assert.Equal(t, "image/svg+xml", get(r, "/favicon.svg").Header().Get("Content-Type"))
	assert.Equal(t, "INDEX", get(r, "/history").Body.String(), "SPA route")
	trav := get(r, "/../../etc/passwd")
	assert.Equal(t, http.StatusBadRequest, trav.Code, "traversal rejected")
	assert.NotContains(t, trav.Body.String(), "root:")
	assert.Equal(t, http.StatusNotFound, get(r, "/api/unknown").Code)
	assert.Equal(t, http.StatusMovedPermanently, get(r, "/docs").Code)
	assert.Equal(t, "DOCS", get(r, "/docs/").Body.String())
	assert.JSONEq(t, `{"version":"3.1.0","commit":"<abc>","build_date":"","go_version":""}`,
		get(r, "/api/version").Body.String())

	// A cross-origin preflight is no longer answered with an allow.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/keys", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/nowhere", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// Static folders never list directories or serve dotfiles (review HTTP-L5); a directory
// with an index.html (/docs/) is still served.
func TestRoutes_StaticDirsDoNotList(t *testing.T) {
	_, r := router(t)
	assert.Equal(t, "INDEX", get(r, "/assets").Body.String(), "SPA route, not a listing")
	for _, p := range []string{"/assets/", "/data/", "/assets/.env", "/docs/.git/config"} {
		rec := get(r, p)
		assert.Equal(t, http.StatusNotFound, rec.Code, p)
		assert.NotContains(t, rec.Body.String(), "app.js", p)
	}
	assert.Equal(t, "JS", get(r, "/assets/app.js").Body.String())
	assert.Equal(t, "DOCS", get(r, "/docs/").Body.String())
	assert.Equal(t, "INDEX", get(r, "/.hidden").Body.String(), "dotfile: SPA, not the file")
}

func TestRoutes_WithoutDistFolders(t *testing.T) {
	svc := mocks.NewMockReadService(t)
	r := SetupRoutes(Deps{
		Handlers: httphandler.HandlerDeps{Service: svc},
		Frontend: fstest.MapFS{}, Log: zerolog.Nop(), // Docs nil: not built either
	})
	assert.Equal(t, http.StatusNotFound, get(r, "/").Code)
	assert.Equal(t, http.StatusNotFound, get(r, "/docs/").Code)
	assert.Equal(t, http.StatusNotFound, get(r, "/ws").Code)
}

// Backend roots match whole path segments only: /healthz is an SPA route, /api/x and
// /docs/ are backend paths (review nit).
func TestIsBackendPath(t *testing.T) {
	for p, want := range map[string]bool{
		"/health": true, "/healthz": false, "/api/keys": true, "/api": true, "/apix": false,
		"/ws": true, "/docs/": true, "/documents": false, "/history": false,
	} {
		assert.Equal(t, want, isBackendPath(p), p)
	}
}
