package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util"
)

// ReadService is what the handlers need from the service layer.
type ReadService interface {
	Health() health.Snapshot
	Keys() []solis.Register
	Data(ctx context.Context, q service.DataQuery) (service.DataResult, error)
}

// HandlerDeps are the handler dependencies.
type HandlerDeps struct {
	Service ReadService
	Errors  *ErrorMapper
	Clock   util.Clock
	// Timeout bounds each storage read (app.timeout) so a slow history query cannot hold
	// the single SQLite connection the poller writes through; 0 = request context only.
	Timeout time.Duration
}

// GetHealthHandler serves the supervisor snapshot: 200 for ok/degraded, 503 when any
// component failed (fail-closed). It never blocks on a component.
func GetHealthHandler(deps HandlerDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snap := deps.Service.Health()
		status := http.StatusOK
		if snap.Status == health.StatusFailed {
			status = http.StatusServiceUnavailable
		}
		WriteJSON(w, status, snap)
	})
}

// RegisterInfo is one /api/keys entry (v3: "stability" removed, "store" added).
type RegisterInfo struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Address     uint16 `json:"address,omitempty"`
	DataType    string `json:"data_type"`
	Unit        string `json:"unit"`
	Store       string `json:"store"`
	Description string `json:"description"`
}

// GetKeysHandler lists every register with metadata.
func GetKeysHandler(deps HandlerDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		regs := deps.Service.Keys()
		infos := make([]RegisterInfo, 0, len(regs))
		for _, r := range regs {
			desc := fmt.Sprintf("%s (%s)", r.Name, r.Unit)
			if service.HistoryCapable(r.Store) { // totals are periodic but take no range (400)
				desc += " - Use with start/end query parameters for historical data"
			}
			infos = append(infos, RegisterInfo{
				Key: r.Key, Name: r.Name, Address: r.Address,
				DataType: r.DataType.String(), Unit: r.Unit, Store: r.Store.String(),
				Description: desc,
			})
		}
		WriteJSON(w, http.StatusOK, infos)
	})
}

// GetDataHandler serves /api/data/{key}. The service decides what a key returns
// (current value, history rows, lifetime total or status history); the handler only
// bounds the read and renders JSON.
func GetDataHandler(deps HandlerDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ctx, cancel := deps.readContext(r.Context())
		defer cancel()
		res, err := deps.Service.Data(ctx, service.DataQuery{
			Key: chi.URLParam(r, "key"), Start: q.Get("start"), End: q.Get("end"),
			Now: deps.Clock.Now(),
		})
		if err != nil {
			deps.Errors.Write(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, render(res))
	})
}

// render turns a data result into its JSON body.
func render(res service.DataResult) any {
	switch {
	case res.Current != nil:
		return NewDataResponse(res.Current)
	case res.Total != nil:
		return DataResponse{
			Key: res.Register.Key, Name: res.Register.Name, Unit: res.Register.Unit,
			Value: round(res.Total.Value), RawValue: round(res.Total.RawValue),
			Timestamp: res.Total.Timestamp,
		}
	case res.Status != nil:
		return res.Status
	default:
		return res.Rows
	}
}

// readContext derives the storage deadline of one request.
func (d HandlerDeps) readContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if d.Timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, d.Timeout)
}

// DataResponse is a single current or total value (values rounded to two decimals).
type DataResponse struct {
	Key           string                    `json:"key"`
	Name          string                    `json:"name,omitempty"`
	Unit          string                    `json:"unit,omitempty"`
	Value         util.Float64With2Decimals `json:"value"`
	RawValue      util.Float64With2Decimals `json:"raw_value"`
	Timestamp     string                    `json:"timestamp,omitempty"`
	StatusDecoded any                       `json:"status_decoded,omitempty"`
}

// NewDataResponse renders a cached value.
func NewDataResponse(v *solis.Value) DataResponse {
	return DataResponse{
		Key: v.Key, Name: v.Name, Unit: v.Unit, Value: round(v.DecodedValue),
		RawValue: round(v.RawValue), Timestamp: v.Timestamp.Format(time.RFC3339),
		StatusDecoded: v.StatusDecoded,
	}
}

func round(f float64) util.Float64With2Decimals {
	return util.Float64With2Decimals(util.RoundTo2DecimalPlaces(f))
}
