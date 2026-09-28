package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/http/httphandler"
	"github.com/dombyte/solis/internal/http/httphandler/mocks"
	"github.com/dombyte/solis/internal/util/clocktest"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func router(t *testing.T) (*mocks.MockReadService, http.Handler) {
	t.Helper()
	fe, docs := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(fe, "index.html"), "INDEX")
	writeFile(t, filepath.Join(fe, "assets", "app.js"), "JS")
	writeFile(t, filepath.Join(fe, "robots.txt"), "ROBOTS")
	writeFile(t, filepath.Join(fe, "favicon.svg"), "<svg/>")
	writeFile(t, filepath.Join(docs, "index.html"), "DOCS")
	svc := mocks.NewMockReadService(t)
	ws := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	return svc, SetupRoutes(Deps{
		Handlers: httphandler.HandlerDeps{
			Service: svc,
			Errors:  httphandler.NewErrorMapper(zerolog.Nop()), Clock: clocktest.New(time.Now()),
		},
		WebSocket: ws, FrontendDir: fe, DocsDir: docs, Log: zerolog.Nop(),
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
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))

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

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/keys", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/nowhere", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRoutes_WithoutDistFolders(t *testing.T) {
	svc := mocks.NewMockReadService(t)
	r := SetupRoutes(Deps{
		Handlers:    httphandler.HandlerDeps{Service: svc},
		FrontendDir: filepath.Join(t.TempDir(), "absent"), DocsDir: "/nonexistent",
		Log: zerolog.Nop(),
	})
	assert.Equal(t, http.StatusNotFound, get(r, "/").Code)
	assert.Equal(t, http.StatusNotFound, get(r, "/ws").Code)
}
