package httphandler

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/report"
	reportcsv "github.com/dombyte/solis/internal/report/csv"
	reportpdf "github.com/dombyte/solis/internal/report/pdf"
	"github.com/dombyte/solis/internal/service"
)

// reportWriter renders a report as one response format; now is the request time.
type reportWriter func(w http.ResponseWriter, rep report.Report, deps HandlerDeps,
	now time.Time)

// reportWriters maps the format parameter to its writer.
func reportWriters() map[string]reportWriter {
	return map[string]reportWriter{
		"json": writeReportJSON, "csv": writeReportCSV, "pdf": writeReportPDF,
	}
}

// GetReportHandler serves /api/report?from&to[&compare][&granularity][&format]: the
// energy report of a day range as JSON (default, the Reports page preview) or as a CSV
// or PDF download. "today" for the range check is the server's local day.
func GetReportHandler(deps HandlerDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		format := q.Get("format")
		if format == "" {
			format = "json"
		}
		write, ok := reportWriters()[format]
		if !ok {
			deps.Errors.Write(w, fmt.Errorf("%w: format %q (want json, csv or pdf)",
				report.ErrInvalidRange, format))
			return
		}
		ctx, cancel := deps.readContext(r.Context())
		defer cancel()
		now := deps.Clock.Now()
		rep, err := deps.Service.Report(ctx, service.ReportQuery{
			From: q.Get("from"), To: q.Get("to"), Compare: q.Get("compare"),
			Granularity: q.Get("granularity"), Today: now.Format(period.DayLayout),
		})
		if err != nil {
			deps.Errors.Write(w, err)
			return
		}
		write(w, rep, deps, now)
	})
}

func writeReportJSON(w http.ResponseWriter, rep report.Report, _ HandlerDeps, _ time.Time) {
	WriteJSON(w, http.StatusOK, NewReportResponse(rep))
}

// writeReportCSV renders into a buffer first so a failure still gets an error status.
func writeReportCSV(w http.ResponseWriter, rep report.Report, deps HandlerDeps, _ time.Time) {
	var buf bytes.Buffer
	if err := reportcsv.Write(&buf, rep); err != nil {
		deps.Errors.Write(w, err)
		return
	}
	writeDownload(w, "text/csv; charset=utf-8", reportFilename(rep, "csv"), buf.Bytes())
}

// writeReportPDF renders the A4 report into a buffer, then sends it.
func writeReportPDF(w http.ResponseWriter, rep report.Report, deps HandlerDeps, now time.Time) {
	var buf bytes.Buffer
	meta := reportpdf.Meta{Generated: now, Version: deps.Build.Version}
	if err := reportpdf.Write(&buf, rep, meta); err != nil {
		deps.Errors.Write(w, err)
		return
	}
	writeDownload(w, "application/pdf", reportFilename(rep, "pdf"), buf.Bytes())
}

// writeDownload sends body as an attachment named filename.
func writeDownload(w http.ResponseWriter, contentType, filename string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body) //nolint:errcheck // the client went away; nothing left to report
}

// reportFilename is solis-report-<from>_<to>.<ext>.
func reportFilename(rep report.Report, ext string) string {
	return fmt.Sprintf("solis-report-%s_%s.%s", rep.Range.From.Format(period.DayLayout),
		rep.Range.To.Format(period.DayLayout), ext)
}
