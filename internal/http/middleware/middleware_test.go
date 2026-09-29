package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestRecover_DoesNotEchoPanicValue(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(zerolog.New(&logs))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret database path /data/solis.db")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "secret")
	assert.Contains(t, rec.Body.String(), "internal server error")
	assert.Contains(t, logs.String(), "secret", "logged server-side")
}

func TestRecover_ReraisesAbortHandler(t *testing.T) {
	h := Recover(zerolog.Nop())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

func TestRecover_AbortsAfterPartialWrite(t *testing.T) {
	h := Recover(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"partial":`))
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	})
	assert.Equal(t, `{"partial":`, rec.Body.String(), "no second response appended")
}

func TestLogger(t *testing.T) {
	var logs bytes.Buffer
	log := zerolog.New(&logs).Level(zerolog.DebugLevel)
	ok := Logger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ok.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a", nil))
	assert.Contains(t, logs.String(), `"status":204`)

	fail := Logger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	fail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/b", nil))
	assert.Contains(t, logs.String(), `"level":"warn"`)
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	assert.Empty(t, rec.Header().Get("X-Frame-Options"), "framing stays allowed (dashboards)")
}

func TestLogger_IncludesRequestID(t *testing.T) {
	var logs bytes.Buffer
	h := middleware.RequestID(Logger(zerolog.New(&logs))(http.NotFoundHandler()))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Regexp(t, `"request_id":"[^"]+"`, logs.String())
}
