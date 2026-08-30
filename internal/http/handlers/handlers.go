// Package handlers provides HTTP request handlers for the Solis monitor API.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/go-chi/chi/v5"
)

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
	Timestamp     string      `json:"timestamp"`
	StatusDecoded interface{} `json:"status_decoded"`
}

// StatusResponse represents the response for status register requests.
type StatusResponse struct {
	Key     string               `json:"key"`
	Name    string               `json:"name"`
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

// PanicRecoveryMiddleware recovers from panics in HTTP handlers.
// This ensures the server never crashes due to a panic in a handler.
func PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error().Msgf("PANIC in HTTP handler: %v", r)
				WriteJSON(w, ErrorResponse{
					Error:   "Internal Server Error",
					Message: fmt.Sprintf("Panic recovered: %v", r),
					Code:    http.StatusInternalServerError,
				}, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
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
					description += " - Use with start/end query parameters for historical data"
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
// This implements the v2 API design:
// - NO query params: Returns latest current value (from cache)
// - ?start=2026-08-01&end=2026-08-03: For daily/monthly/yearly keys, returns historical data
// - For total keys: Always returns lifetime value regardless of query params
func GetDataHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, reg, err := validateAndGetRegister(r, deps.Service)
		if err != nil {
			WriteError(w, err.Error(), err.statusCode)
			return
		}

		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")
		hasQueryParams := startStr != "" || endStr != ""

		keyType := GetKeyType(key)

		if hasQueryParams && keyType == "current" {
			WriteError(w, fmt.Sprintf("historical queries are not supported for register %s - only daily, monthly, yearly, and total registers support historical data", key), http.StatusBadRequest)
			return
		}

		switch keyType {
		case "daily":
			handleDailyRegister(w, r, key, deps.Service, hasQueryParams)
		case "monthly":
			handleMonthlyRegister(w, r, key, deps.Service, hasQueryParams)
		case "yearly":
			handleYearlyRegister(w, r, key, deps.Service, hasQueryParams)
		case "total":
			handleTotalRegister(w, key, reg, deps.Service)
		default:
			handleDefaultRegister(w, r, key, reg, deps.Service, hasQueryParams)
		}
	}
}

// registerValidationError is a custom error type for validation failures
type registerValidationError struct {
	message    string
	statusCode int
}

func (e *registerValidationError) Error() string {
	return e.message
}

// validateAndGetRegister validates the request and returns the register key and metadata
func validateAndGetRegister(r *http.Request, service ReadServiceInterface) (string, *solis.Register, *registerValidationError) {
	key := chi.URLParam(r, "key")
	if key == "" {
		return "", nil, &registerValidationError{message: "register key is required", statusCode: http.StatusBadRequest}
	}

	if !service.IsRegisterEnabled(key) {
		return "", nil, &registerValidationError{message: fmt.Sprintf("unknown register key: %s", key), statusCode: http.StatusNotFound}
	}

	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return "", nil, &registerValidationError{message: fmt.Sprintf("unknown register key: %s", key), statusCode: http.StatusNotFound}
	}

	return key, reg, nil
}

// handleDailyRegister handles daily register requests
func handleDailyRegister(w http.ResponseWriter, r *http.Request, key string, service ReadServiceInterface, hasQueryParams bool) {
	if hasQueryParams {
		handleDailyWithParams(w, r, key, service)
		return
	}
	handleCurrentValue(w, key, service)
}

// handleDailyWithParams handles daily register requests with time range parameters
func handleDailyWithParams(w http.ResponseWriter, r *http.Request, key string, service ReadServiceInterface) {
	timeRange, err := ParseTimeRange(r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		WriteError(w, fmt.Sprintf("invalid time range: %v", err), http.StatusBadRequest)
		return
	}

	history, err := service.GetDailyHistory(key, timeRange.Start, timeRange.End)
	if err != nil {
		WriteError(w, err.Error(), http.StatusNotFound)
		return
	}

	WriteJSON(w, history, http.StatusOK)
}

// handleMonthlyRegister handles monthly register requests
func handleMonthlyRegister(w http.ResponseWriter, r *http.Request, key string, service ReadServiceInterface, hasQueryParams bool) {
	if hasQueryParams {
		handleMonthlyWithParams(w, r, key, service)
		return
	}
	handleCurrentValue(w, key, service)
}

// handleMonthlyWithParams handles monthly register requests with time range parameters
func handleMonthlyWithParams(w http.ResponseWriter, r *http.Request, key string, service ReadServiceInterface) {
	timeRange, err := ParseTimeRange(r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		WriteError(w, fmt.Sprintf("invalid time range: %v", err), http.StatusBadRequest)
		return
	}

	history, err := service.GetMonthlyHistory(key, timeRange.Start, timeRange.End)
	if err != nil {
		WriteError(w, err.Error(), http.StatusNotFound)
		return
	}

	WriteJSON(w, history, http.StatusOK)
}

// handleYearlyRegister handles yearly register requests
func handleYearlyRegister(w http.ResponseWriter, r *http.Request, key string, service ReadServiceInterface, hasQueryParams bool) {
	if hasQueryParams {
		handleYearlyWithParams(w, r, key, service)
		return
	}
	handleCurrentValue(w, key, service)
}

// handleYearlyWithParams handles yearly register requests with time range parameters
func handleYearlyWithParams(w http.ResponseWriter, r *http.Request, key string, service ReadServiceInterface) {
	timeRange, err := ParseTimeRange(r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		WriteError(w, fmt.Sprintf("invalid time range: %v", err), http.StatusBadRequest)
		return
	}

	history, err := service.GetYearlyHistory(key, timeRange.Start, timeRange.End)
	if err != nil {
		WriteError(w, err.Error(), http.StatusNotFound)
		return
	}

	WriteJSON(w, history, http.StatusOK)
}

// handleTotalRegister handles total register requests
func handleTotalRegister(w http.ResponseWriter, key string, reg *solis.Register, service ReadServiceInterface) {
	history, err := service.GetTotalHistory(key)
	if err != nil {
		WriteError(w, err.Error(), http.StatusNotFound)
		return
	}
	if history == nil {
		WriteError(w, fmt.Sprintf("no total data found for register %s", key), http.StatusNotFound)
		return
	}

	response := DataResponse{
		Key:       key,
		Name:      reg.Name,
		Unit:      reg.Unit,
		Value:     history.Value,
		RawValue:  history.RawValue,
		Timestamp: history.Timestamp,
	}

	WriteJSON(w, response, http.StatusOK)
}

// handleCurrentValue handles requests for current value of a register
func handleCurrentValue(w http.ResponseWriter, key string, service ReadServiceInterface) {
	value, err := service.GetRegister(key)
	if err != nil {
		WriteError(w, err.Error(), http.StatusNotFound)
		return
	}

	WriteJSON(w, buildDataResponse(key, value), http.StatusOK)
}

// handleDefaultRegister handles default case (status and current registers)
func handleDefaultRegister(w http.ResponseWriter, r *http.Request, key string, reg *solis.Register, service ReadServiceInterface, hasQueryParams bool) {
	if reg.Status {
		handleStatusRegister(w, r, key, reg, service)
		return
	}

	value, err := service.GetRegister(key)
	if err != nil {
		WriteError(w, err.Error(), http.StatusNotFound)
		return
	}

	WriteJSON(w, buildDataResponse(key, value), http.StatusOK)
}

// handleStatusRegister handles status register requests with error history
func handleStatusRegister(w http.ResponseWriter, r *http.Request, key string, reg *solis.Register, service ReadServiceInterface) {
	startTime := time.Unix(0, 0)
	endTime := time.Unix(1<<63-1, 0)

	errorHistory, err := service.GetErrorHistory(key, startTime, endTime)
	entries := make([]StatusHistoryEntry, 0, len(errorHistory)+1)

	if err != nil {
		logger.Warn().Msgf("Failed to get error history for %s: %v", key, err)
	} else {
		addErrorHistoryEntries(&entries, errorHistory, reg)
	}

	value, err := service.GetRegister(key)
	if err != nil {
		WriteError(w, err.Error(), http.StatusNotFound)
		return
	}

	if value.StatusDecoded != nil {
		entries = append(entries, StatusHistoryEntry{
			Timestamp:     value.Timestamp.Format(time.RFC3339),
			StatusDecoded: value.StatusDecoded,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	WriteJSON(w, StatusResponse{
		Key:     key,
		Name:    value.Name,
		History: entries,
	}, http.StatusOK)
}

// addErrorHistoryEntries adds error history entries to the StatusHistoryEntry slice
func addErrorHistoryEntries(entries *[]StatusHistoryEntry, errorHistory []*storage.ErrorDataPoint, reg *solis.Register) {
	for _, dp := range errorHistory {
		rawUint16 := uint16(dp.RawValue)
		decodedValue := solis.DecodeRegister(reg, []uint16{rawUint16})

		if decodedValue.StatusDecoded != nil {
			*entries = append(*entries, StatusHistoryEntry{
				Timestamp:     dp.Timestamp,
				StatusDecoded: decodedValue.StatusDecoded,
			})
		}
	}
}

// GetDailyHandler returns daily aggregated values for energy registers.
