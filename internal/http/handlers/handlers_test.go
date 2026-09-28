package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/http/handlers/mocks"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/utils/clocktest"
)

var t0 = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*mocks.MockReadService, *chi.Mux) {
	t.Helper()
	svc := mocks.NewMockReadService(t)
	deps := HandlerDeps{Service: svc, Errors: NewErrorMapper(zerolog.Nop()), Clock: clocktest.New(t0)}
	r := chi.NewRouter()
	r.Method(http.MethodGet, "/health", GetHealthHandler(deps))
	r.Method(http.MethodGet, "/api/keys", GetKeysHandler(deps))
	r.Method(http.MethodGet, "/api/data/{key}", GetDataHandler(deps))
	return svc, r
}

func do(t *testing.T, h http.Handler, url string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	if rec.Body.Len() > 0 && rec.Body.Bytes()[0] == '{' {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	}
	return rec.Code, body
}

func reg(key string, store solis.Store) solis.Register {
	return solis.Register{Key: key, Name: key, Unit: "kWh", Store: store, Scale: 1}
}

func TestHealth_FailClosed(t *testing.T) {
	tests := []struct {
		status string
		code   int
	}{
		{health.StatusOK, http.StatusOK},
		{health.StatusDegraded, http.StatusOK},
		{health.StatusFailed, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		svc, r := setup(t)
		svc.EXPECT().Health().Return(health.Snapshot{Status: tt.status, Component: "poller",
			Reason: "restart budget exhausted", Components: map[string]health.ComponentStatus{}}).Once()
		code, body := do(t, r, "/health")
		assert.Equal(t, tt.code, code, tt.status)
		assert.Equal(t, tt.status, body["status"])
		if tt.status == health.StatusFailed {
			assert.Equal(t, "poller", body["component"])
			assert.Equal(t, "restart budget exhausted", body["reason"])
		}
	}
}

func TestKeys(t *testing.T) {
	svc, r := setup(t)
	svc.EXPECT().Keys().Return([]solis.Register{
		{Key: "grid_power", Name: "Grid Power", Address: 33130, DataType: solis.Int32, Unit: "W"},
		{Key: "pv_energy_monthly", Name: "PV Energy Monthly", DataType: solis.Uint32,
			Unit: "kWh", Store: solis.StoreMonthly},
	}).Once()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/keys", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var infos []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &infos))
	require.Len(t, infos, 2)
	assert.InDelta(t, 33130.0, infos[0]["address"], 0)
	assert.Equal(t, "none", infos[0]["store"])
	assert.NotContains(t, infos[0], "stability")
	assert.NotContains(t, infos[1], "address", "omitted for computed registers")
	assert.Equal(t, "monthly", infos[1]["store"])
	assert.Contains(t, infos[1]["description"], "start/end")
}

func TestData_CurrentValue(t *testing.T) {
	svc, r := setup(t)
	svc.EXPECT().Register("grid_power").Return(reg("grid_power", solis.StoreNone), nil).Once()
	svc.EXPECT().Current("grid_power").Return(&solis.Value{Key: "grid_power", Name: "Grid Power",
		Unit: "W", DecodedValue: 0, RawValue: 0, Timestamp: t0}, nil).Once()
	code, body := do(t, r, "/api/data/grid_power")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "value", "zero is present, not omitted")
	assert.Equal(t, t0.Format(time.RFC3339), body["timestamp"])
}

func TestData_HistoryByStore(t *testing.T) {
	svc, r := setup(t)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	svc.EXPECT().Register("pv_energy_daily").Return(reg("pv_energy_daily", solis.StoreDaily), nil)
	svc.EXPECT().DailyHistory(mock.Anything, "pv_energy_daily", start, end).
		Return([]*storage.DailyDataPoint{{Date: "2026-08-01", Value: 1.234}}, nil).Once()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/data/pv_energy_daily?start=2026-08-01&end=2026-08-03", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[{"date":"2026-08-01","value":1.23,"raw_value":0}]`, rec.Body.String())

	svc.EXPECT().Register("pv_energy_monthly").Return(reg("pv_energy_monthly",
		solis.StoreMonthly), nil)
	svc.EXPECT().MonthlyHistory(mock.Anything, "pv_energy_monthly", mock.Anything, t0).
		Return(nil, nil).Once()
	code, _ := do(t, r, "/api/data/pv_energy_monthly?start=2026-01")
	assert.Equal(t, http.StatusOK, code)

	svc.EXPECT().Register("pv_energy_yearly").Return(reg("pv_energy_yearly", solis.StoreYearly),
		nil)
	svc.EXPECT().YearlyHistory(mock.Anything, "pv_energy_yearly", t0.Add(-defaultHistoryWindow),
		time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)).Return(nil, nil).Once()
	code, _ = do(t, r, "/api/data/pv_energy_yearly?end=2027")
	assert.Equal(t, http.StatusOK, code)

	// Without params a periodic key returns its current value.
	svc.EXPECT().Current("pv_energy_daily").Return(&solis.Value{Key: "pv_energy_daily"}, nil).Once()
	code, _ = do(t, r, "/api/data/pv_energy_daily")
	assert.Equal(t, http.StatusOK, code)
}

func TestData_TotalAndStatus(t *testing.T) {
	svc, r := setup(t)
	svc.EXPECT().Register("pv_energy_total").Return(reg("pv_energy_total", solis.StoreTotal), nil)
	svc.EXPECT().Total(mock.Anything, "pv_energy_total").Return(&storage.TotalDataPoint{
		Value: 5230.456, RawValue: 5230.456, Timestamp: "t"}, nil).Once()
	code, body := do(t, r, "/api/data/pv_energy_total")
	assert.Equal(t, http.StatusOK, code)
	assert.InDelta(t, 5230.46, body["value"], 1e-9)

	// A total has one lifetime value: range parameters are rejected, not ignored.
	code, body = do(t, r, "/api/data/pv_energy_total?start=garbage")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body["message"], "historical queries not supported")

	svc.EXPECT().Register("grid_fault_1").Return(reg("grid_fault_1", solis.StoreStatus), nil)
	svc.EXPECT().StatusHistory(mock.Anything, "grid_fault_1").Return(service.StatusHistory{
		Key: "grid_fault_1", History: []service.StatusEntry{}}, nil).Once()
	code, body = do(t, r, "/api/data/grid_fault_1")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "grid_fault_1", body["key"])
}

func TestData_Errors(t *testing.T) {
	svc, r := setup(t)
	svc.EXPECT().Register("nope").Return(solis.Register{},
		&service.KeyError{Key: "nope", Err: service.ErrUnknownKey}).Once()
	code, body := do(t, r, "/api/data/nope")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "Not Found", body["error"])
	assert.InDelta(t, 404.0, body["code"], 0)

	svc.EXPECT().Register("grid_power").Return(reg("grid_power", solis.StoreNone), nil)
	code, body = do(t, r, "/api/data/grid_power?start=2026-08-01")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body["message"], "historical queries not supported")

	svc.EXPECT().Register("pv_energy_daily").Return(reg("pv_energy_daily", solis.StoreDaily), nil)
	code, body = do(t, r, "/api/data/pv_energy_daily?start=yesterday")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body["message"], "invalid time range")

	svc.EXPECT().Current("grid_power").Return(nil, errors.New("sqlite: disk I/O at /data/x.db")).Once()
	code, body = do(t, r, "/api/data/grid_power")
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "Internal Server Error", body["message"], "internals never exposed")

	svc.EXPECT().Current("grid_power").Return(nil, &service.KeyError{Key: "grid_power",
		Err: service.ErrNoData}).Once()
	code, _ = do(t, r, "/api/data/grid_power")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestErrorMapper(t *testing.T) {
	m := NewErrorMapper(zerolog.Nop())
	assert.Equal(t, http.StatusNotFound, m.Status(fmt.Errorf("x: %w", service.ErrUnknownKey)))
	assert.Equal(t, http.StatusBadRequest, m.Status(service.ErrWrongKind))
	assert.Equal(t, http.StatusGatewayTimeout, m.Status(context.DeadlineExceeded))
	assert.Equal(t, http.StatusInternalServerError, m.Status(errors.New("x")))

	// Per-handler override wins without touching the default mapper.
	teapot := errors.New("teapot")
	o := m.With(Rule{Target: service.ErrNoData, Status: http.StatusNoContent},
		Rule{Target: teapot, Status: http.StatusTeapot})
	assert.Equal(t, http.StatusNoContent, o.Status(service.ErrNoData))
	assert.Equal(t, http.StatusTeapot, o.Status(teapot))
	assert.Equal(t, http.StatusNotFound, m.Status(service.ErrNoData))
}

func TestParseTimeRange(t *testing.T) {
	tr, err := ParseTimeRange("", "", t0)
	require.NoError(t, err)
	assert.Equal(t, t0.Add(-defaultHistoryWindow), tr.Start)
	assert.Equal(t, t0, tr.End)
	_, err = ParseTimeRange("2026-08-01", "bad", t0)
	assert.ErrorIs(t, err, service.ErrInvalidRange)

	// Month/year ends are inclusive of the whole period.
	tests := []struct{ start, end, wantStart, wantEnd string }{
		{"2026", "2026", "2026-01-01", "2026-12-31"},
		{"2026-02", "2026-02", "2026-02-01", "2026-02-28"},
		{"2024-02", "2024-02", "2024-02-01", "2024-02-29"},
		{"2026-08-01", "2026-08-03", "2026-08-01", "2026-08-03"},
	}
	for _, tt := range tests {
		tr, err := ParseTimeRange(tt.start, tt.end, t0)
		require.NoError(t, err)
		assert.Equal(t, tt.wantStart, tr.Start.Format(time.DateOnly), tt.start)
		assert.Equal(t, tt.wantEnd, tr.End.Format(time.DateOnly), tt.end)
	}
}

func TestData_StorageTimeout(t *testing.T) {
	svc := mocks.NewMockReadService(t)
	deps := HandlerDeps{Service: svc, Errors: NewErrorMapper(zerolog.Nop()),
		Clock: clocktest.New(t0), Timeout: time.Second}
	svc.EXPECT().Register("pv_energy_total").Return(reg("pv_energy_total", solis.StoreTotal), nil)
	svc.EXPECT().Total(mock.Anything, "pv_energy_total").RunAndReturn(
		func(ctx context.Context, _ string) (*storage.TotalDataPoint, error) {
			_, ok := ctx.Deadline()
			assert.True(t, ok, "storage reads carry app.timeout")
			return &storage.TotalDataPoint{}, nil
		}).Once()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data/pv_energy_total", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", "pv_energy_total")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	GetDataHandler(deps).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
