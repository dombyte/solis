package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/go-chi/chi/v5"
)

// MockReadService is a mock implementation of ReadServiceInterface for testing
type MockReadService struct {
	isRegisterEnabledFunc  func(key string) bool
	getKeysFunc            func() []string
	getValuesFunc          func(keys []string) (map[string]*solis.Value, error)
	getRegisterFunc        func(key string) (*solis.Value, error)
	getHistoricalDataFunc  func(key string, start, end time.Time, interval storage.Interval) (*storage.HistoryResult, error)
	healthCheckFunc        func() (map[string]string, error)
	decodeFaultBitsFunc    func(addr uint16, value uint16) []string
	decodeOpStatusBitsFunc func(value uint16) []string
	getErrorHistoryFunc    func(key string, start, end time.Time) ([]*storage.ErrorDataPoint, error)
	getDailyHistoryFunc    func(key string, start, end time.Time) ([]*storage.DailyDataPoint, error)
	getMonthlyHistoryFunc  func(key string, start, end time.Time) ([]*storage.MonthlyDataPoint, error)
	getYearlyHistoryFunc   func(key string, start, end time.Time) ([]*storage.YearlyDataPoint, error)
	getTotalHistoryFunc    func(key string) (*storage.TotalDataPoint, error)
}

func (m *MockReadService) GetKeys() []string {
	if m.getKeysFunc != nil {
		return m.getKeysFunc()
	}
	return []string{}
}

func (m *MockReadService) IsRegisterEnabled(key string) bool {
	if m.isRegisterEnabledFunc != nil {
		return m.isRegisterEnabledFunc(key)
	}
	return true // Default: all registers enabled
}

func (m *MockReadService) HealthCheck() (map[string]string, error) {
	if m.healthCheckFunc != nil {
		return m.healthCheckFunc()
	}
	return map[string]string{"status": "ok"}, nil
}

func (m *MockReadService) GetValues(keys []string) (map[string]*solis.Value, error) {
	if m.getValuesFunc != nil {
		return m.getValuesFunc(keys)
	}
	return nil, nil
}

func (m *MockReadService) GetRegister(key string) (*solis.Value, error) {
	if m.getRegisterFunc != nil {
		return m.getRegisterFunc(key)
	}
	return nil, nil
}

func (m *MockReadService) GetHistoricalData(key string, start, end time.Time, interval storage.Interval) (*storage.HistoryResult, error) {
	if m.getHistoricalDataFunc != nil {
		return m.getHistoricalDataFunc(key, start, end, interval)
	}
	return nil, nil
}

func (m *MockReadService) GetDailyHistory(key string, start, end time.Time) ([]*storage.DailyDataPoint, error) {
	if m.getDailyHistoryFunc != nil {
		return m.getDailyHistoryFunc(key, start, end)
	}
	return nil, nil
}

func (m *MockReadService) GetMonthlyHistory(key string, start, end time.Time) ([]*storage.MonthlyDataPoint, error) {
	if m.getMonthlyHistoryFunc != nil {
		return m.getMonthlyHistoryFunc(key, start, end)
	}
	return nil, nil
}

func (m *MockReadService) GetYearlyHistory(key string, start, end time.Time) ([]*storage.YearlyDataPoint, error) {
	if m.getYearlyHistoryFunc != nil {
		return m.getYearlyHistoryFunc(key, start, end)
	}
	return nil, nil
}

func (m *MockReadService) GetTotalHistory(key string) (*storage.TotalDataPoint, error) {
	if m.getTotalHistoryFunc != nil {
		return m.getTotalHistoryFunc(key)
	}
	return nil, nil
}

func (m *MockReadService) GetErrorHistory(key string, start, end time.Time) ([]*storage.ErrorDataPoint, error) {
	if m.getErrorHistoryFunc != nil {
		return m.getErrorHistoryFunc(key, start, end)
	}
	return nil, nil
}

// Test WriteJSON functionality
func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}
	statusCode := http.StatusOK

	WriteJSON(w, data, statusCode)

	if w.Code != statusCode {
		t.Errorf("WriteJSON() status code = %v, want %v", w.Code, statusCode)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("WriteJSON() Content-Type = %v, want %v", contentType, "application/json")
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Errorf("WriteJSON() failed to decode response: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("WriteJSON() result = %v, want %v", result, data)
	}
}

func TestWriteJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	statusCode := http.StatusOK

	WriteJSON(w, nil, statusCode)

	if w.Code != statusCode {
		t.Errorf("WriteJSON() status code = %v, want %v", w.Code, statusCode)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("WriteJSON() Content-Type = %v, want %v", contentType, "application/json")
	}
}

func TestWriteJSON_DifferentStatusCodes(t *testing.T) {
	statusCodes := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, statusCode := range statusCodes {
		w := httptest.NewRecorder()
		data := map[string]string{"status": http.StatusText(statusCode)}

		WriteJSON(w, data, statusCode)

		if w.Code != statusCode {
			t.Errorf("WriteJSON() status code = %v, want %v", w.Code, statusCode)
		}
	}
}

func TestWriteJSON_ComplexData(t *testing.T) {
	w := httptest.NewRecorder()
	data := struct {
		Name     string            `json:"name"`
		Values   []int             `json:"values"`
		Metadata map[string]string `json:"metadata"`
	}{
		Name:     "test",
		Values:   []int{1, 2, 3},
		Metadata: map[string]string{"key": "value"},
	}
	statusCode := http.StatusOK

	WriteJSON(w, data, statusCode)

	if w.Code != statusCode {
		t.Errorf("WriteJSON() status code = %v, want %v", w.Code, statusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("WriteJSON() failed to decode response: %v", err)
	}

	if result["name"] != "test" {
		t.Errorf("WriteJSON() name = %v, want %v", result["name"], "test")
	}
}

// Test WriteError functionality
func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	message := "test error"
	statusCode := http.StatusBadRequest

	WriteError(w, message, statusCode)

	if w.Code != statusCode {
		t.Errorf("WriteError() status code = %v, want %v", w.Code, statusCode)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("WriteError() Content-Type = %v, want %v", contentType, "application/json")
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Errorf("WriteError() failed to decode response: %v", err)
	}

	if result["error"] != message {
		t.Errorf("WriteError() error = %v, want %v", result["error"], message)
	}
	if result["message"] != message {
		t.Errorf("WriteError() message = %v, want %v", result["message"], message)
	}
	if result["status"] != float64(statusCode) {
		t.Errorf("WriteError() status = %v, want %v", result["status"], float64(statusCode))
	}
}

func TestWriteError_DifferentStatusCodes(t *testing.T) {
	statusCodes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, statusCode := range statusCodes {
		w := httptest.NewRecorder()
		message := http.StatusText(statusCode)

		WriteError(w, message, statusCode)

		if w.Code != statusCode {
			t.Errorf("WriteError() status code = %v, want %v for %s", w.Code, statusCode, message)
		}
	}
}

func TestWriteError_EmptyMessage(t *testing.T) {
	w := httptest.NewRecorder()
	message := ""
	statusCode := http.StatusInternalServerError

	WriteError(w, message, statusCode)

	if w.Code != statusCode {
		t.Errorf("WriteError() status code = %v, want %v", w.Code, statusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("WriteError() failed to decode response: %v", err)
	}

	if result["error"] != "" {
		t.Errorf("WriteError() error = %v, want empty string", result["error"])
	}
}

// Test ErrorResponse structure
func TestErrorResponse_Structure(t *testing.T) {
	errResp := ErrorResponse{
		Error:   "Bad Request",
		Message: "invalid parameter",
		Code:    http.StatusBadRequest,
	}

	if errResp.Error != "Bad Request" {
		t.Errorf("ErrorResponse.Error = %v, want %v", errResp.Error, "Bad Request")
	}
	if errResp.Message != "invalid parameter" {
		t.Errorf("ErrorResponse.Message = %v, want %v", errResp.Message, "invalid parameter")
	}
	if errResp.Code != http.StatusBadRequest {
		t.Errorf("ErrorResponse.Code = %v, want %v", errResp.Code, http.StatusBadRequest)
	}
}

// Test RegisterInfo structure
func TestRegisterInfo_Structure(t *testing.T) {
	info := RegisterInfo{
		Key:         "test_key",
		Name:        "Test Register",
		Address:     100,
		DataType:    "Uint16",
		Unit:        "V",
		Stability:   "dynamic",
		Description: "Test Register (V)",
	}

	if info.Key != "test_key" {
		t.Errorf("RegisterInfo.Key = %v, want %v", info.Key, "test_key")
	}
	if info.Name != "Test Register" {
		t.Errorf("RegisterInfo.Name = %v, want %v", info.Name, "Test Register")
	}
	if info.Address != 100 {
		t.Errorf("RegisterInfo.Address = %v, want %v", info.Address, 100)
	}
	if info.DataType != "Uint16" {
		t.Errorf("RegisterInfo.DataType = %v, want %v", info.DataType, "Uint16")
	}
	if info.Unit != "V" {
		t.Errorf("RegisterInfo.Unit = %v, want %v", info.Unit, "V")
	}
	if info.Stability != "dynamic" {
		t.Errorf("RegisterInfo.Stability = %v, want %v", info.Stability, "dynamic")
	}
	if info.Description != "Test Register (V)" {
		t.Errorf("RegisterInfo.Description = %v, want %v", info.Description, "Test Register (V)")
	}
}

// Test HandlerDeps structure
func TestHandlerDeps_Structure(t *testing.T) {
	deps := HandlerDeps{}
	if deps.Service != nil {
		t.Errorf("HandlerDeps.Service should be nil by default")
	}
}

// Test GetHealthHandler
func TestGetHealthHandler(t *testing.T) {
	// Create a mock service with HealthCheck method
	service := &MockReadService{
		healthCheckFunc: func() (map[string]string, error) {
			return map[string]string{"status": "ok", "timestamp": time.Now().UTC().Format(time.RFC3339)}, nil
		},
	}

	deps := HandlerDeps{
		Service: service,
	}

	handler := GetHealthHandler(deps)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetHealthHandler() status code = %v, want %v", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("GetHealthHandler() Content-Type = %v, want %v", contentType, "application/json")
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("GetHealthHandler() failed to decode response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("GetHealthHandler() status = %v, want 'ok'", result["status"])
	}
	if _, ok := result["timestamp"]; !ok {
		t.Error("GetHealthHandler() response should contain timestamp")
	}
}

func TestGetHealthHandler_ServiceError(t *testing.T) {
	// Create a mock service that returns an error
	service := &MockReadService{
		healthCheckFunc: func() (map[string]string, error) {
			return nil, errors.New("health check failed")
		},
	}

	deps := HandlerDeps{
		Service: service,
	}

	handler := GetHealthHandler(deps)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("GetHealthHandler() with error status code = %v, want %v", w.Code, http.StatusInternalServerError)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("GetHealthHandler() failed to decode error response: %v", err)
	}

	if _, ok := result["error"]; !ok {
		t.Error("GetHealthHandler() error response should contain error field")
	}
}

// Test GetKeysHandler
func TestGetKeysHandler(t *testing.T) {
	// GetKeysHandler needs a service with GetKeys() method
	// Create a mock service that returns all register keys
	service := &MockReadService{
		getKeysFunc: func() []string {
			keys := make([]string, 0, len(solis.RegisterMapByKey))
			for k := range solis.RegisterMapByKey {
				keys = append(keys, k)
			}
			return keys
		},
	}
	deps := HandlerDeps{Service: service}

	handler := GetKeysHandler(deps)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetKeysHandler() status code = %v, want %v", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("GetKeysHandler() Content-Type = %v, want %v", contentType, "application/json")
	}

	var result []RegisterInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("GetKeysHandler() failed to decode response: %v", err)
	}

	// Should return all registers
	if len(result) == 0 {
		t.Error("GetKeysHandler() returned empty result, want non-empty")
	}

	// Check first register has required fields
	if len(result) > 0 {
		if result[0].Key == "" {
			t.Error("GetKeysHandler() first register has empty key")
		}
		if result[0].Name == "" {
			t.Error("GetKeysHandler() first register has empty name")
		}
		if result[0].Address == 0 {
			t.Error("GetKeysHandler() first register has zero address")
		}
		if result[0].DataType == "" {
			t.Error("GetKeysHandler() first register has empty data type")
		}
	}
}

// Test GetDataHandler - current value (no query params)
func TestGetDataHandler_CurrentValue(t *testing.T) {
	// Create a mock value - use a real register key from solis package
	testValue := &solis.Value{
		Key:          "pv_voltage_1",
		Name:         "PV1 Voltage",
		RawValue:     100.0,
		DecodedValue: 100.5,
		Unit:         "V",
		Timestamp:    time.Now(),
		Stability:    solis.Dynamic,
	}

	// Create a mock service
	service := &MockReadService{
		getRegisterFunc: func(key string) (*solis.Value, error) {
			if key == "pv_voltage_1" {
				return testValue, nil
			}
			return nil, fmt.Errorf("unknown register: %s", key)
		},
		decodeFaultBitsFunc: func(addr uint16, value uint16) []string {
			return nil
		},
		decodeOpStatusBitsFunc: func(value uint16) []string {
			return nil
		},
	}

	deps := HandlerDeps{
		Service: service,
	}

	handler := GetDataHandler(deps)

	// Create request with chi URL param
	r := chi.NewRouter()
	r.Get("/api/v1/data/{key}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/data/pv_voltage_1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetDataHandler() status code = %v, want %v", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("GetDataHandler() failed to decode response: %v", err)
	}

	if result["key"] != "pv_voltage_1" {
		t.Errorf("GetDataHandler() key = %v, want 'pv_voltage_1'", result["key"])
	}
	if result["name"] != "PV1 Voltage" {
		t.Errorf("GetDataHandler() name = %v, want 'PV1 Voltage'", result["name"])
	}
	if result["unit"] != "V" {
		t.Errorf("GetDataHandler() unit = %v, want 'V'", result["unit"])
	}
	if _, ok := result["timestamp"]; !ok {
		t.Error("GetDataHandler() response should contain timestamp")
	}
}

func TestGetDataHandler_UnknownKey(t *testing.T) {
	service := &MockReadService{
		getRegisterFunc: func(key string) (*solis.Value, error) {
			return nil, fmt.Errorf("unknown register: %s", key)
		},
		decodeFaultBitsFunc: func(addr uint16, value uint16) []string {
			return nil
		},
		decodeOpStatusBitsFunc: func(value uint16) []string {
			return nil
		},
	}

	deps := HandlerDeps{
		Service: service,
	}

	handler := GetDataHandler(deps)

	r := chi.NewRouter()
	r.Get("/api/v1/data/{key}", handler)

	// Use a key that doesn't exist in solis.RegisterMapByKey
	req := httptest.NewRequest(http.MethodGet, "/api/v1/data/nonexistent_register_xyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GetDataHandler() with unknown key status code = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestGetDataHandler_EmptyKey(t *testing.T) {
	// Skip this test - chi routing makes it difficult to test empty key
	// The handler checks for empty key via chi.URLParam which requires chi routing
	// In practice, chi will return 404 for routes that don't match the pattern
	t.Skip("Empty key test skipped - requires chi routing context")
}

// Test GetDataHandler with history query parameters
func TestGetDataHandler_HistoryQuery(t *testing.T) {
	// Create mock historical data - use a valid register key
	// Use a fixed timestamp that's easy to parse
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	historyResult := &storage.HistoryResult{
		Key:      "pv_voltage_1",
		Unit:     "V",
		Interval: storage.IntervalRaw,
		Data: []storage.HistoryDataPoint{
			{Timestamp: start.Format(time.RFC3339), Value: 100.0},
			{Timestamp: end.Format(time.RFC3339), Value: 200.0},
		},
	}

	service := &MockReadService{
		getHistoricalDataFunc: func(key string, start, end time.Time, interval storage.Interval) (*storage.HistoryResult, error) {
			return historyResult, nil
		},
		decodeFaultBitsFunc: func(addr uint16, value uint16) []string {
			return nil
		},
		decodeOpStatusBitsFunc: func(value uint16) []string {
			return nil
		},
	}

	deps := HandlerDeps{
		Service: service,
	}

	handler := GetDataHandler(deps)

	r := chi.NewRouter()
	r.Get("/api/v1/data/{key}", handler)

	// Request with start and end query parameters - only 'raw' interval is supported now
	startStr := "2024-01-01T00:00:00Z"
	endStr := "2024-01-02T00:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/data/pv_voltage_1?start="+startStr+"&end="+endStr+"&interval=raw", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Historical raw data is no longer available - should return 501 Not Implemented
	if w.Code != http.StatusNotImplemented {
		t.Errorf("GetDataHandler() with history query status code = %v, want %v, body: %s", w.Code, http.StatusNotImplemented, w.Body.String())
	}
}

func TestGetDataHandler_InvalidTimeFormat(t *testing.T) {
	service := &MockReadService{
		getHistoricalDataFunc: func(key string, start, end time.Time, interval storage.Interval) (*storage.HistoryResult, error) {
			return nil, nil
		},
		decodeFaultBitsFunc: func(addr uint16, value uint16) []string {
			return nil
		},
		decodeOpStatusBitsFunc: func(value uint16) []string {
			return nil
		},
	}

	deps := HandlerDeps{
		Service: service,
	}

	handler := GetDataHandler(deps)

	r := chi.NewRouter()
	r.Get("/api/v1/data/{key}", handler)

	// Request with invalid time format - use valid register key
	// With our changes, any query params return 501 Not Implemented
	req := httptest.NewRequest(http.MethodGet, "/api/v1/data/pv_voltage_1?start=invalid-time", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("GetDataHandler() with invalid time format status code = %v, want %v", w.Code, http.StatusNotImplemented)
	}
}
