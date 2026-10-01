package httphandler

import (
	"bytes"
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

	"github.com/dombyte/solis/internal/buildinfo"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/http/httphandler/mocks"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util/clocktest"
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
		svc.EXPECT().Health().Return(health.Snapshot{
			Status: tt.status, Component: "poller",
			Reason: "restart budget exhausted", Components: map[string]health.ComponentStatus{},
		}).Once()
		code, body := do(t, r, "/health")
		assert.Equal(t, tt.code, code, tt.status)
		assert.Equal(t, tt.status, body["status"])
		if tt.status == health.StatusFailed {
			assert.Equal(t, "poller", body["component"])
			assert.Equal(t, "restart budget exhausted", body["reason"])
		}
	}
}

func TestVersion(t *testing.T) {
	build := buildinfo.Info{
		Version: "3.1.0", Commit: "abc1234", BuildDate: "2026-10-01T00:00:00Z", GoVersion: "go1.26",
	}
	rec := httptest.NewRecorder()
	GetVersionHandler(HandlerDeps{Build: build}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"version":"3.1.0","commit":"abc1234",`+
		`"build_date":"2026-10-01T00:00:00Z","go_version":"go1.26"}`, rec.Body.String())
}

func TestKeys(t *testing.T) {
	svc, r := setup(t)
	svc.EXPECT().Keys().Return([]solis.Register{
		{Key: "grid_power", Name: "Grid Power", Address: 33130, DataType: solis.Int32, Unit: "W"},
		{
			Key: "pv_energy_monthly", Name: "PV Energy Monthly", DataType: solis.Uint32,
			Unit: "kWh", Store: solis.StoreMonthly,
		},
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

// The handler only renders what the service decided (review HTTP-L11 moved the rules
// into service.Data): each result kind has its JSON shape, values rounded to 2 decimals.
func TestData_RendersEachResultKind(t *testing.T) {
	svc, r := setup(t)
	svc.EXPECT().Data(mock.Anything, mock.MatchedBy(func(q service.DataQuery) bool {
		return q.Key == "grid_power"
	})).Return(service.DataResult{Current: &solis.Value{
		Key: "grid_power", Name: "Grid Power", Unit: "W", Timestamp: t0,
	}}, nil).Once()
	code, body := do(t, r, "/api/data/grid_power")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "value", "zero is present, not omitted")
	assert.Equal(t, t0.Format(time.RFC3339), body["timestamp"])

	svc.EXPECT().Data(mock.Anything, mock.MatchedBy(func(q service.DataQuery) bool {
		return q.Key == "pv_energy_total"
	})).Return(service.DataResult{
		Register: reg("pv_energy_total", solis.StoreTotal),
		Total:    &history.TotalDataPoint{Value: 5230.456, RawValue: 5230.456, Timestamp: "t"},
	}, nil).Once()
	code, body = do(t, r, "/api/data/pv_energy_total")
	assert.Equal(t, http.StatusOK, code)
	assert.InDelta(t, 5230.46, body["value"], 1e-9)
	assert.Equal(t, "pv_energy_total", body["key"])

	svc.EXPECT().Data(mock.Anything, mock.MatchedBy(func(q service.DataQuery) bool {
		return q.Key == "grid_fault_1"
	})).Return(service.DataResult{Status: &service.StatusHistory{
		Key: "grid_fault_1", History: []service.StatusEntry{},
	}}, nil).Once()
	code, body = do(t, r, "/api/data/grid_fault_1")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "grid_fault_1", body["key"])

	svc.EXPECT().Data(mock.Anything, mock.MatchedBy(func(q service.DataQuery) bool {
		return q.Key == "pv_energy_daily" && q.Start == "2026-08-01" && q.End == "2026-08-03" &&
			q.Now.Equal(t0)
	})).Return(service.DataResult{
		Rows: []*history.DailyDataPoint{{Date: "2026-08-01", Value: 1.234}},
	}, nil).Once()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/data/pv_energy_daily?start=2026-08-01&end=2026-08-03", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[{"date":"2026-08-01","value":1.23,"raw_value":0}]`, rec.Body.String())
}

func TestData_Errors(t *testing.T) {
	svc, r := setup(t)
	for key, tt := range map[string]struct {
		err  error
		code int
		msg  string
	}{
		"nope":       {&service.KeyError{Key: "nope", Err: service.ErrUnknownKey}, 404, "nope"},
		"grid_power": {&service.KeyError{Key: "grid_power", Err: service.ErrWrongKind, Detail: "x"}, 400, "unsupported"},
		"pv_daily":   {fmt.Errorf("%w: \"yesterday\"", service.ErrInvalidRange), 400, "invalid time range"},
		"no_value":   {&service.KeyError{Key: "no_value", Err: service.ErrNoData}, 404, "no data"},
		"broken":     {errors.New("sqlite: disk I/O at /data/x.db"), 500, "Internal Server Error"},
	} {
		svc.EXPECT().Data(mock.Anything, mock.MatchedBy(func(q service.DataQuery) bool {
			return q.Key == key
		})).Return(service.DataResult{}, tt.err).Once()
		code, body := do(t, r, "/api/data/"+key)
		assert.Equal(t, tt.code, code, key)
		assert.Contains(t, body["message"], tt.msg, key)
		assert.InDelta(t, float64(tt.code), body["code"], 0, key)
	}
}

func TestErrorMapper(t *testing.T) {
	m := NewErrorMapper(zerolog.Nop())
	assert.Equal(t, http.StatusNotFound, m.Status(fmt.Errorf("x: %w", service.ErrUnknownKey)))
	assert.Equal(t, http.StatusBadRequest, m.Status(service.ErrWrongKind))
	assert.Equal(t, http.StatusGatewayTimeout, m.Status(context.DeadlineExceeded))
	assert.Equal(t, StatusClientClosedRequest, m.Status(fmt.Errorf("q: %w", context.Canceled)))
	assert.Equal(t, http.StatusInternalServerError, m.Status(errors.New("x")))
}

// A client that disconnects mid-query is not a server error: 499 and a debug log, no
// error-level log line (review HTTP-L2).
func TestErrorMapper_ClientClosedIsNotLoggedAsError(t *testing.T) {
	var logs bytes.Buffer
	m := NewErrorMapper(zerolog.New(&logs).Level(zerolog.InfoLevel))
	rec := httptest.NewRecorder()
	m.Write(rec, fmt.Errorf("storage: %w", context.Canceled))
	assert.Equal(t, StatusClientClosedRequest, rec.Code)
	assert.Empty(t, logs.String())

	rec = httptest.NewRecorder()
	m.Write(rec, errors.New("disk I/O error"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, logs.String(), "request failed")
	assert.NotContains(t, rec.Body.String(), "disk I/O")
}

func TestData_StorageTimeout(t *testing.T) {
	svc := mocks.NewMockReadService(t)
	deps := HandlerDeps{
		Service: svc, Errors: NewErrorMapper(zerolog.Nop()),
		Clock: clocktest.New(t0), Timeout: time.Second,
	}
	svc.EXPECT().Data(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, _ service.DataQuery) (service.DataResult, error) {
			_, ok := ctx.Deadline()
			assert.True(t, ok, "storage reads carry app.timeout")
			return service.DataResult{Total: &history.TotalDataPoint{}}, nil
		}).Once()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data/pv_energy_total", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", "pv_energy_total")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	GetDataHandler(deps).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
