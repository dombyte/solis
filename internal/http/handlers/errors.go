// Package handlers provides the HTTP handlers of the Solis monitor API. Handlers are thin:
// they parse the request, call the read service and render JSON; errors go through the
// central ErrorMapper.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/service"
)

// ErrorResponse is the documented error body (docs/src/openapi.yaml).
type ErrorResponse struct {
	// Error is the HTTP status text.
	Error string `json:"error"`
	// Message is a client-safe message.
	Message string `json:"message"`
	// Code is the HTTP status code.
	Code int `json:"code"`
}

// Rule maps errors matching Target (errors.Is) to Status.
type Rule struct {
	Target error
	Status int
}

// ErrorMapper maps errors to HTTP status codes via errors.Is over sentinel errors.
type ErrorMapper struct {
	rules []Rule
	log   zerolog.Logger
}

// DefaultRules are the application-wide mappings.
func DefaultRules() []Rule {
	return []Rule{
		{service.ErrUnknownKey, http.StatusNotFound},
		{service.ErrNoData, http.StatusNotFound},
		{service.ErrWrongKind, http.StatusBadRequest},
		{service.ErrInvalidRange, http.StatusBadRequest},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
	}
}

// NewErrorMapper creates a mapper with the default rules.
func NewErrorMapper(log zerolog.Logger) *ErrorMapper {
	return &ErrorMapper{rules: DefaultRules(), log: log}
}

// Status returns the status code for err (500 when nothing matches).
func (m *ErrorMapper) Status(err error) int {
	for _, r := range m.rules {
		if errors.Is(err, r.Target) {
			return r.Status
		}
	}
	return http.StatusInternalServerError
}

// Write renders err. Client errors expose their message; server errors are logged and
// answered with a generic message (never internal details).
func (m *ErrorMapper) Write(w http.ResponseWriter, err error) {
	status := m.Status(err)
	msg := err.Error()
	if status >= http.StatusInternalServerError {
		m.log.Error().Err(err).Int("status", status).Msg("request failed")
		msg = http.StatusText(status)
	}
	WriteError(w, status, msg)
}

// WriteError writes the documented error body.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: http.StatusText(status), Message: msg, Code: status})
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Encoding errors after WriteHeader can only be a broken connection.
	_ = json.NewEncoder(w).Encode(v) //nolint:errcheck // response already committed
}
