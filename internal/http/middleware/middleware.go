// Package middleware provides HTTP middleware (func(http.Handler) http.Handler) with an
// injected logger.
package middleware

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// Recover turns handler panics into a 500 JSON response; the panic value is logged,
// never sent to the client. When the handler already wrote the response head, a second
// write would corrupt the body, so the connection is aborted instead.
func Recover(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				if p := recover(); p != nil {
					if p == http.ErrAbortHandler { //nolint:errorlint // sentinel panic value
						panic(p) //nolint:forbidigo // re-raise net/http's abort sentinel
					}
					log.Error().Interface("panic", p).Str("path", r.URL.Path).
						Msg("panic in HTTP handler")
					if ww.Status() != 0 {
						panic(http.ErrAbortHandler) //nolint:forbidigo // net/http abort contract
					}
					writeInternalError(ww)
				}
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

func writeInternalError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // best effort
		"error":   http.StatusText(http.StatusInternalServerError),
		"message": "internal server error",
		"code":    http.StatusInternalServerError,
	})
}

// SecurityHeaders sets the headers every response gets: no MIME sniffing and no
// referrer leaking the monitor's LAN URL. Framing is deliberately allowed: the dashboard
// is commonly embedded as an iframe (e.g. a Home Assistant panel).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// Logger logs one line per request at debug level (errors at warn).
func Logger(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			ev := log.Debug()
			if ww.Status() >= http.StatusInternalServerError {
				ev = log.Warn()
			}
			ev.Str("request_id", middleware.GetReqID(r.Context())).
				Str("method", r.Method).Str("path", r.URL.Path).Int("status", ww.Status()).
				Dur("duration", time.Since(start)).Msg("http request")
		})
	}
}
