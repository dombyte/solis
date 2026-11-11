package httphandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/http/httphandler/mocks"
	"github.com/dombyte/solis/internal/report"
	"github.com/dombyte/solis/internal/service"
	"github.com/dombyte/solis/internal/util/clocktest"
)

func aug(day int) time.Time { return time.Date(2026, time.August, day, 0, 0, 0, 0, time.UTC) }

// sampleReport has two complete days of three and a last-year comparison.
func sampleReport() report.Report {
	e := report.Energy{
		PV: 20, Consumption: 10, Household: 9, Backup: 1, Import: 2, Export: 12,
		Charge: 4, Discharge: 3,
	}
	days := []report.Day{{Date: aug(1), Energy: e}, {Date: aug(3), Energy: e}}
	last := []report.Day{{Date: aug(1).AddDate(-1, 0, 0), Energy: e}}
	return report.Build(report.Input{
		Range: report.Range{From: aug(1), To: aug(3)}, Granularity: report.ByDay, Days: days,
		Compare: &report.CompareInput{
			Mode:  report.CompareLastYear,
			Range: report.Range{From: aug(1).AddDate(-1, 0, 0), To: aug(3).AddDate(-1, 0, 0)},
			Days:  last,
		},
	})
}

func reportRouter(t *testing.T) (*mocks.MockReadService, *chi.Mux) {
	t.Helper()
	svc := mocks.NewMockReadService(t)
	deps := HandlerDeps{
		Service: svc, Errors: NewErrorMapper(zerolog.Nop()), Clock: clocktest.New(t0),
		Timeout: time.Second,
	}
	r := chi.NewRouter()
	r.Method(http.MethodGet, "/api/report", GetReportHandler(deps))
	return svc, r
}

func get(h http.Handler, url string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestReport_JSON(t *testing.T) {
	svc, r := reportRouter(t)
	svc.EXPECT().Report(mock.Anything, service.ReportQuery{
		From: "2026-08-01", To: "2026-08-03", Compare: "last_year", Today: "2026-08-05",
	}).Return(sampleReport(), nil).Once()
	rec := get(r, "/api/report?from=2026-08-01&to=2026-08-03&compare=last_year")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		From, To, Granularity string
		Summary               map[string]any
		Figures               []map[string]any
		Comparison            map[string]any
		Patterns              map[string]any
		Buckets               []map[string]any
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "2026-08-01", body.From)
	assert.Equal(t, "day", body.Granularity)
	assert.InDelta(t, 2.0, body.Summary["complete_days"], 0)
	assert.InDelta(t, 80.0, body.Summary["self_sufficiency_pct"], 1e-9)
	assert.Equal(t, map[string]any{
		"id": "self_sufficiency", "kind": "ratio", "better": "higher",
		"value": 80.0, "compared": 80.0, "base": 80.0, "change": 0.0, "change_pct": nil,
	}, body.Figures[2])
	assert.Equal(t, "pv", body.Figures[0]["id"])
	assert.Equal(t, true, body.Comparison["matched"], "2 of 3 days")
	assert.InDelta(t, 1.0, body.Comparison["matched_days"], 0, "1 Aug in both years")
	assert.InDelta(t, 40.0, body.Figures[0]["value"], 1e-9, "both days of the period")
	assert.InDelta(t, 20.0, body.Figures[0]["compared"], 1e-9, "the matched day")
	assert.InDelta(t, 0.0, body.Figures[0]["change_pct"], 1e-9, "20 kWh on 1 Aug both")
	assert.Contains(t, body.Patterns, "low_import_days")
	assert.Equal(t, "2026-08-01", body.Patterns["best_pv"].(map[string]any)["date"])
	require.Len(t, body.Buckets, 3)
	assert.Nil(t, body.Buckets[1]["summary"].(map[string]any)["total"], "missing day")
	assert.Equal(t, []any{}, body.Summary["grid_charging_days"])
}

func TestReport_CSV(t *testing.T) {
	svc, r := reportRouter(t)
	svc.EXPECT().Report(mock.Anything, mock.MatchedBy(func(q service.ReportQuery) bool {
		return q.Granularity == "month"
	})).Return(sampleReport(), nil).Once()
	rec := get(r, "/api/report?from=2026-08-01&to=2026-08-03&format=csv&granularity=month")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/csv; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="solis-report-2026-08-01_2026-08-03.csv"`,
		rec.Header().Get("Content-Disposition"))
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	assert.Len(t, lines, 4)
	assert.True(t, strings.HasPrefix(lines[0], "period,from,to,"))
}

func TestReport_PDF(t *testing.T) {
	svc, r := reportRouter(t)
	svc.EXPECT().Report(mock.Anything, mock.Anything).Return(sampleReport(), nil).Once()
	rec := get(r, "/api/report?from=2026-08-01&to=2026-08-03&format=pdf")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="solis-report-2026-08-01_2026-08-03.pdf"`,
		rec.Header().Get("Content-Disposition"))
	assert.True(t, strings.HasPrefix(rec.Body.String(), "%PDF-"))
}

func TestReport_Errors(t *testing.T) {
	svc, r := reportRouter(t)
	rec := get(r, "/api/report?from=2026-08-01&to=2026-08-03&format=xlsx")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), `format \"xlsx\"`)

	svc.EXPECT().Report(mock.Anything, mock.Anything).
		Return(report.Report{}, fmt.Errorf("%w: to is after today", report.ErrInvalidRange)).
		Once()
	rec = get(r, "/api/report?from=2026-08-01&to=2026-09-03")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	svc.EXPECT().Report(mock.Anything, mock.Anything).
		Return(report.Report{}, fmt.Errorf("storage: query: %w", assert.AnError)).Once()
	rec = get(r, "/api/report?from=2026-08-01&to=2026-08-03&format=csv")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
