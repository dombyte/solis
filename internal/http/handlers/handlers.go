// Package handlers provides HTTP request handlers for the Solis monitor API.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/metrics"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/go-chi/chi/v5"
)

// @title Solis Monitor API
// @version 1.0
// @description API for monitoring Solis inverters via Modbus
// @termsOfService http://swagger.io/terms/
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
//
// logger is the package-level logger for handler operations.
var logger = logging.NewComponentLogger("http.handlers")

// ReadServiceInterface defines the methods from service.ReadService that handlers need.
// This allows for easier testing with mocks.
type ReadServiceInterface interface {
	HealthCheck() (map[string]string, error)
	IsRegisterEnabled(key string) bool
	GetKeys() []string
	GetValues(keys []string) (map[string]*solis.Value, error)
	GetRegister(key string) (*solis.Value, error)
	GetErrorHistory(key string, start, end time.Time) ([]*storage.ErrorDataPoint, error)
	GetHistoricalData(key string, start, end time.Time, interval storage.Interval) (*storage.HistoryResult, error)
	GetDailyHistory(key string, start, end time.Time) ([]*storage.DailyDataPoint, error)
	GetMonthlyHistory(key string, start, end time.Time) ([]*storage.MonthlyDataPoint, error)
	GetYearlyHistory(key string, start, end time.Time) ([]*storage.YearlyDataPoint, error)
	GetTotalHistory(key string) (*storage.TotalDataPoint, error)
}

// HandlerDeps contains dependencies for HTTP handlers.
type HandlerDeps struct {
	// Service is the service layer for business logic.
	Service ReadServiceInterface
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	// Error is the HTTP status text
	Error string `json:"error"`
	// Message is the detailed error message
	Message string `json:"message"`
	// Code is the HTTP status code
	Code int `json:"code"`
}

// StatusHistoryEntry represents a single decoded status entry in the history.
type StatusHistoryEntry struct {
	Timestamp    string      `json:"timestamp"`
	StatusDecoded interface{} `json:"status_decoded"`
}

// StatusResponse represents the response for status register requests.
type StatusResponse struct {
	Key     string              `json:"key"`
	Name    string              `json:"name"`
	History []StatusHistoryEntry `json:"history"`
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, data any, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error().Msgf("Failed to encode JSON: %v", err)
	}
}

// WriteError writes an error response as JSON.
func WriteError(w http.ResponseWriter, message string, statusCode int) {
	response := map[string]any{
		"error":   message,
		"status":  statusCode,
		"message": message,
	}
	WriteJSON(w, response, statusCode)
}

// GetHealthHandler returns a handler for the health check endpoint.
// @Summary Get health status
// @Description Returns the health status of the API service
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
func GetHealthHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := deps.Service.HealthCheck()
		if err != nil {
			WriteError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		status["timestamp"] = time.Now().UTC().Format(time.RFC3339)
		WriteJSON(w, status, http.StatusOK)
	}
}

// RegisterInfo represents metadata about a register for the /api/v1/keys endpoint.
type RegisterInfo struct {
	// Key is the unique identifier for this register
	Key string `json:"key"`
	// Name is the human-readable name of the register
	Name string `json:"name"`
	// Address is the Modbus register address
	Address uint16 `json:"address"`
	// DataType is the type of value stored in this register
	DataType string `json:"data_type"`
	// Unit is the unit of measurement
	Unit string `json:"unit"`
	// Stability indicates how often this value changes
	Stability string `json:"stability"`
	// Description combines name and unit for display
	Description string `json:"description"`
}

// GetKeysHandler returns a handler for getting all register keys with metadata.
// @Summary Get all register keys
// @Description Returns a list of all available register keys with their descriptions, units, and metadata. Includes daily, monthly, yearly, and total energy registers. Does not include actual values.
// @Tags keys
// @Accept json
// @Produce json
// @Success 200 {array} RegisterInfo
// @Router /api/keys [get]
func GetKeysHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get enabled register keys from the service
		enabledKeys := deps.Service.GetKeys()

		// Convert to RegisterInfo slice
		infos := make([]RegisterInfo, 0, len(enabledKeys))
		for _, key := range enabledKeys {
			if reg, ok := solis.RegisterMapByKey[key]; ok {
				// Build description with unit
				description := fmt.Sprintf("%s (%s)", reg.Name, reg.Unit)

				// Append usage note for history-only registers
				if solis.IsDailyRegister(key) || solis.IsMonthlyRegister(key) || solis.IsYearlyRegister(key) || solis.IsTotalRegister(key) {
					description += " - Must be used via /api/history/daily, /api/history/monthly, /api/history/yearly, or /api/history/total endpoints respectively"
				}

				infos = append(infos, RegisterInfo{
					Key:         reg.Key,
					Name:        reg.Name,
					Address:     reg.Address,
					DataType:    reg.DataType.String(),
					Unit:        reg.Unit,
					Stability:   reg.Stability.String(),
					Description: description,
				})
			}
		}

		WriteJSON(w, infos, http.StatusOK)
	}
}

// GetDataHandler returns a handler for getting data for a specific register key.
// Supports all register types including status and stable registers.
// @Summary Get register data
// @Description Returns the current value for a specific register key. Historical queries with start/end parameters are no longer supported and will return 501 Not Implemented.
// @Tags data
// @Accept json
// @Produce json
// @Param key path string true "Register key"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/data/{key} [get]
func GetDataHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the register key from the URL path
		key := chi.URLParam(r, "key")
		if key == "" {
			WriteError(w, "register key is required", http.StatusBadRequest)
			return
		}

		// Check if the register is enabled (this will also return false for unknown keys)
		if !deps.Service.IsRegisterEnabled(key) {
			WriteError(w, fmt.Sprintf("unknown register key: %s", key), http.StatusNotFound)
			return
		}

		// Get the register metadata to check if it's a status register
		reg, ok := solis.RegisterMapByKey[key]
		if !ok {
			// This should not happen if IsRegisterEnabled is working correctly
			WriteError(w, fmt.Sprintf("unknown register key: %s", key), http.StatusNotFound)
			return
		}

		// Parse query parameters
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		// Check if any query parameter was provided
		hasQueryParams := startStr != "" || endStr != ""

		if hasQueryParams {
			// Historical raw data is no longer available - only latest values in cache
			WriteError(w, "Historical raw data is not available - only current values are supported", http.StatusNotImplemented)
			return
		}

		// No query parameters - get current value
		value, err := deps.Service.GetRegister(key)
		if err != nil {
			WriteError(w, err.Error(), http.StatusNotFound)
			return
		}

		// For status registers, return history response
		if reg.Status {
			// Get all error history for this status register (no time filter = all data)
			// Use min and max time to get all records
			startTime := time.Unix(0, 0) // Unix epoch start
			endTime := time.Unix(1<<63-1, 0) // Far future
			errorHistory, err := deps.Service.GetErrorHistory(key, startTime, endTime)
			
			// Pre-allocate entries with capacity for database entries + current value
			entries := make([]StatusHistoryEntry, 0, len(errorHistory)+1)
			
			// Add stored history from database
			if err != nil {
				logger.Warn().Msgf("Failed to get error history for %s: %v", key, err)
			} else {
				for _, dp := range errorHistory {
					// Convert raw_value (float64) to uint16 for status decoding
					rawUint16 := uint16(dp.RawValue)
					// Use reg directly (already have it from line 200) instead of looking it up again
					rawBytes := []byte{byte(rawUint16 >> 8), byte(rawUint16 & 0xFF)}
					decodedValue := solis.DecodeRegister(reg, rawBytes)
					
					if decodedValue.StatusDecoded != nil {
						entries = append(entries, StatusHistoryEntry{
							Timestamp:     dp.Timestamp,
							StatusDecoded: decodedValue.StatusDecoded,
						})
					}
				}
			}
			
			// Include current cached value if we have status_decoded
			if value.StatusDecoded != nil {
				entries = append(entries, StatusHistoryEntry{
					Timestamp:     value.Timestamp.Format(time.RFC3339),
					StatusDecoded: value.StatusDecoded,
				})
			}
			
			// Sort entries by timestamp (latest first)
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Timestamp > entries[j].Timestamp
			})
			
			WriteJSON(w, StatusResponse{
				Key:     key,
				Name:    value.Name,
				History: entries,
			}, http.StatusOK)
			return
		}

		// For non-status registers, return the standard response
		result := map[string]any{
			"key":       key,
			"name":      value.Name,
			"value":     value.DecodedValue,
			"raw_value": value.RawValue,
			"unit":      value.Unit,
			"timestamp": value.Timestamp.Format(time.RFC3339),
		}
		if value.StringValue != "" {
			result["string_value"] = value.StringValue
		}
		if value.StatusDecoded != nil {
			result["status_decoded"] = value.StatusDecoded
		}

		WriteJSON(w, result, http.StatusOK)
	}
}

// GetMetricsHandler returns a handler for the Prometheus metrics endpoint.
// @Summary Get Prometheus metrics
// @Description Returns Prometheus metrics for all register values. Only available when metrics are enabled in configuration.
// @Tags metrics
// @Produce plain
// @Success 200 {string} string
// @Failure 503 {string} string "Metrics not enabled"
// @Router /metrics [get]
func GetMetricsHandler(deps HandlerDeps) http.HandlerFunc {
	// Wrap the http.Handler as http.HandlerFunc
	return func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	}
}

// GetDailyHandler returns daily aggregated values for energy registers.
// @Summary Get daily values
// @Description Returns daily energy values for specific registers.
// @Tags daily
// @Param key path string true "Register key"
// @Param start query string false "Start date (YYYY-MM-DD format)"
// @Param end query string false "End date (YYYY-MM-DD format)"
// @Success 200 {array} []storage.DailyDataPoint
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/history/daily/{key} [get]
func GetDailyHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		// Check if key is a daily register
		if !solis.IsDailyRegister(key) {
			WriteError(w, fmt.Sprintf("register %s is not a daily energy register", key), http.StatusBadRequest)
			return
		}

		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		var start, end time.Time
		if startStr != "" {
			var err error
			start, err = time.Parse("2006-01-02", startStr)
			if err != nil {
				WriteError(w, fmt.Sprintf("invalid start date: %v (expected YYYY-MM-DD)", err), http.StatusBadRequest)
				return
			}
		} else {
			start = time.Now().Add(-30 * 24 * time.Hour) // Default: last 30 days
		}

		if endStr != "" {
			var err error
			end, err = time.Parse("2006-01-02", endStr)
			if err != nil {
				WriteError(w, fmt.Sprintf("invalid end date: %v (expected YYYY-MM-DD)", err), http.StatusBadRequest)
				return
			}
		} else {
			end = time.Now()
		}

		history, err := deps.Service.GetDailyHistory(key, start, end)
		if err != nil {
			WriteError(w, err.Error(), http.StatusNotFound)
			return
		}
		WriteJSON(w, history, http.StatusOK)
	}
}

// GetMonthlyHandler returns monthly aggregated values for energy registers.
// @Summary Get monthly values
// @Description Returns monthly energy values for specific registers.
// @Tags monthly
// @Param key path string true "Register key"
// @Param start query string false "Start month (YYYY-MM format)"
// @Param end query string false "End month (YYYY-MM format)"
// @Success 200 {array} []storage.MonthlyDataPoint
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/history/monthly/{key} [get]
func GetMonthlyHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		// Check if key is a monthly register
		if !solis.IsMonthlyRegister(key) {
			WriteError(w, fmt.Sprintf("register %s is not a monthly energy register", key), http.StatusBadRequest)
			return
		}

		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		var start, end time.Time
		if startStr != "" {
			var err error
			start, err = time.Parse("2006-01", startStr)
			if err != nil {
				WriteError(w, fmt.Sprintf("invalid start month: %v (expected YYYY-MM)", err), http.StatusBadRequest)
				return
			}
		} else {
			start = time.Now().Add(-12 * 30 * 24 * time.Hour) // Default: last 12 months
		}

		if endStr != "" {
			var err error
			end, err = time.Parse("2006-01", endStr)
			if err != nil {
				WriteError(w, fmt.Sprintf("invalid end month: %v (expected YYYY-MM)", err), http.StatusBadRequest)
				return
			}
		} else {
			end = time.Now()
		}

		history, err := deps.Service.GetMonthlyHistory(key, start, end)
		if err != nil {
			WriteError(w, err.Error(), http.StatusNotFound)
			return
		}
		WriteJSON(w, history, http.StatusOK)
	}
}

// GetYearlyHandler returns yearly aggregated values for energy registers.
// @Summary Get yearly values
// @Description Returns yearly energy values for specific registers.
// @Tags yearly
// @Param key path string true "Register key"
// @Param start query string false "Start year (YYYY format)"
// @Param end query string false "End year (YYYY format)"
// @Success 200 {array} []storage.YearlyDataPoint
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/history/yearly/{key} [get]
func GetYearlyHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		// Check if key is a yearly register
		if !solis.IsYearlyRegister(key) {
			WriteError(w, fmt.Sprintf("register %s is not a yearly energy register", key), http.StatusBadRequest)
			return
		}

		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		var start, end time.Time
		if startStr != "" {
			var err error
			start, err = time.Parse("2006", startStr)
			if err != nil {
				WriteError(w, fmt.Sprintf("invalid start year: %v (expected YYYY)", err), http.StatusBadRequest)
				return
			}
		} else {
			start = time.Now().Add(-10 * 365 * 24 * time.Hour) // Default: last 10 years
		}

		if endStr != "" {
			var err error
			end, err = time.Parse("2006", endStr)
			if err != nil {
				WriteError(w, fmt.Sprintf("invalid end year: %v (expected YYYY)", err), http.StatusBadRequest)
				return
			}
		} else {
			end = time.Now()
		}

		history, err := deps.Service.GetYearlyHistory(key, start, end)
		if err != nil {
			WriteError(w, err.Error(), http.StatusNotFound)
			return
		}
		WriteJSON(w, history, http.StatusOK)
	}
}

// GetTotalHandler returns total (lifetime) values for energy registers.
// @Summary Get total values
// @Description Returns total (lifetime) energy values for specific registers.
// @Tags total
// @Param key path string true "Register key"
// @Success 200 {object} storage.TotalDataPoint
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/history/total/{key} [get]
func GetTotalHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		// Check if key is a total register
		if !solis.IsTotalRegister(key) {
			WriteError(w, fmt.Sprintf("register %s is not a total energy register", key), http.StatusBadRequest)
			return
		}

		history, err := deps.Service.GetTotalHistory(key)
		if err != nil {
			WriteError(w, err.Error(), http.StatusNotFound)
			return
		}
		if history == nil {
			WriteError(w, fmt.Sprintf("no total data found for register %s", key), http.StatusNotFound)
			return
		}
		WriteJSON(w, history, http.StatusOK)
	}
}
