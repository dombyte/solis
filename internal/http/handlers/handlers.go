package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/utils"
)

// defaultHistoryWindow is the default history range when start is omitted.
const defaultHistoryWindow = 30 * 24 * time.Hour

// ReadService is what the handlers need from the service layer.
type ReadService interface {
	Health() health.Snapshot
	Keys() []solis.Register
	Register(key string) (solis.Register, error)
	Current(key string) (*solis.Value, error)
	DailyHistory(ctx context.Context, key string, start, end time.Time) (
		[]*history.DailyDataPoint, error)
	MonthlyHistory(ctx context.Context, key string, start, end time.Time) (
		[]*history.MonthlyDataPoint, error)
	YearlyHistory(ctx context.Context, key string, start, end time.Time) (
		[]*history.YearlyDataPoint, error)
	Total(ctx context.Context, key string) (*history.TotalDataPoint, error)
	StatusHistory(ctx context.Context, key string) (service.StatusHistory, error)
}

// HandlerDeps are the handler dependencies.
type HandlerDeps struct {
	Service ReadService
	Errors  *ErrorMapper
	Clock   utils.Clock
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
			if _, periodic := r.Store.Level(); periodic {
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

// GetDataHandler serves /api/data/{key}: no params = latest cached value; start/end on
// daily/monthly/yearly keys = history; total keys = lifetime value; status keys = decoded
// change history.
func GetDataHandler(deps HandlerDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		reg, err := deps.Service.Register(key)
		if err != nil {
			deps.Errors.Write(w, err)
			return
		}
		q := r.URL.Query()
		hasRange := q.Get("start") != "" || q.Get("end") != ""
		if hasRange && !historyCapable(reg.Store) {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf(
				"historical queries not supported for %s - only periodic registers", key))
			return
		}
		ctx, cancel := deps.readContext(r.Context())
		defer cancel()
		body, err := dispatch(ctx, deps, dataRequest{
			reg: reg, hasRange: hasRange,
			start: q.Get("start"), end: q.Get("end"),
		})
		if err != nil {
			deps.Errors.Write(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, body)
	})
}

// readContext derives the storage deadline of one request.
func (d HandlerDeps) readContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if d.Timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, d.Timeout)
}

// historyCapable reports whether start/end apply; totals have a single lifetime value.
func historyCapable(s solis.Store) bool {
	return s == solis.StoreDaily || s == solis.StoreMonthly || s == solis.StoreYearly
}

// dataRequest is one parsed /api/data request.
type dataRequest struct {
	reg        solis.Register
	hasRange   bool
	start, end string
}

// dispatch selects the read by register store (enum switch).
func dispatch(ctx context.Context, deps HandlerDeps, req dataRequest) (any, error) {
	switch {
	case req.reg.Store == solis.StoreTotal:
		return total(ctx, deps, req.reg)
	case req.reg.Store == solis.StoreStatus:
		return deps.Service.StatusHistory(ctx, req.reg.Key)
	case req.hasRange:
		return periodHistory(ctx, deps, req.reg, req.start, req.end)
	default:
		v, err := deps.Service.Current(req.reg.Key)
		if err != nil {
			return nil, err
		}
		return NewDataResponse(v), nil
	}
}

func periodHistory(ctx context.Context, deps HandlerDeps, reg solis.Register, start, end string) (
	any, error,
) {
	tr, err := ParseTimeRange(start, end, deps.Clock.Now())
	if err != nil {
		return nil, err
	}
	switch reg.Store {
	case solis.StoreMonthly:
		return deps.Service.MonthlyHistory(ctx, reg.Key, tr.Start, tr.End)
	case solis.StoreYearly:
		return deps.Service.YearlyHistory(ctx, reg.Key, tr.Start, tr.End)
	default:
		return deps.Service.DailyHistory(ctx, reg.Key, tr.Start, tr.End)
	}
}

func total(ctx context.Context, deps HandlerDeps, reg solis.Register) (any, error) {
	dp, err := deps.Service.Total(ctx, reg.Key)
	if err != nil {
		return nil, err
	}
	return DataResponse{
		Key: reg.Key, Name: reg.Name, Unit: reg.Unit,
		Value: round(dp.Value), RawValue: round(dp.RawValue), Timestamp: dp.Timestamp,
	}, nil
}

// DataResponse is a single current or total value (values rounded to two decimals).
type DataResponse struct {
	Key           string                     `json:"key"`
	Name          string                     `json:"name,omitempty"`
	Unit          string                     `json:"unit,omitempty"`
	Value         utils.Float64With2Decimals `json:"value"`
	RawValue      utils.Float64With2Decimals `json:"raw_value"`
	Timestamp     string                     `json:"timestamp,omitempty"`
	StatusDecoded any                        `json:"status_decoded,omitempty"`
}

// NewDataResponse renders a cached value.
func NewDataResponse(v *solis.Value) DataResponse {
	return DataResponse{
		Key: v.Key, Name: v.Name, Unit: v.Unit, Value: round(v.DecodedValue),
		RawValue: round(v.RawValue), Timestamp: v.Timestamp.Format(time.RFC3339),
		StatusDecoded: v.StatusDecoded,
	}
}

func round(f float64) utils.Float64With2Decimals {
	return utils.Float64With2Decimals(utils.RoundTo2DecimalPlaces(f))
}

// TimeRange is a parsed history range.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// ParseTimeRange parses start/end as YYYY-MM-DD, YYYY-MM or YYYY; start defaults to 30
// days before now and end to now. A month or year end is inclusive: it expands to the
// last day of that period (end=2026 covers all of 2026).
func ParseTimeRange(start, end string, now time.Time) (TimeRange, error) {
	s, _, err := parseTime(start, now.Add(-defaultHistoryWindow))
	if err != nil {
		return TimeRange{}, err
	}
	e, layout, err := parseTime(end, now)
	if err != nil {
		return TimeRange{}, err
	}
	switch layout {
	case period.MonthLayout:
		e = e.AddDate(0, 1, -1)
	case period.YearLayout:
		e = e.AddDate(1, 0, -1)
	}
	return TimeRange{Start: s, End: e}, nil
}

// parseTime returns the parsed time and the matching layout ("" for the default).
func parseTime(s string, def time.Time) (time.Time, string, error) {
	if s == "" {
		return def, "", nil
	}
	for _, layout := range []string{period.DayLayout, period.MonthLayout, period.YearLayout} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, layout, nil
		}
	}
	return time.Time{}, "", fmt.Errorf("%w: %q (want YYYY-MM-DD, YYYY-MM or YYYY)",
		service.ErrInvalidRange, s)
}
