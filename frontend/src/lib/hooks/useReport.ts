import { useEffect, useState } from 'react';
import { fetchReport, type ReportQuery, type ReportResponse } from '../api/report';

export interface UseReportResult {
  /** The latest loaded report; while a newer query loads it is the previous one. */
  report: ReportResponse | null;
  isLoading: boolean;
  error: string | null;
}

interface Loaded {
  key: string;
  report: ReportResponse | null;
  error: string | null;
}

/**
 * Loads the report preview for q (null = nothing to load, e.g. an invalid range). A newer
 * query aborts the request in flight, so a slow response never replaces a newer one.
 */
export function useReport(q: ReportQuery | null): UseReportResult {
  const [loaded, setLoaded] = useState<Loaded>({ key: '', report: null, error: null });
  const key = q ? `${q.from}|${q.to}|${q.compare}` : '';

  useEffect(() => {
    if (!q) return;
    const controller = new AbortController();
    fetchReport(q, controller.signal)
      .then(report => setLoaded({ key, report, error: null }))
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        const error = err instanceof Error ? err.message : 'Could not load the report.';
        setLoaded({ key, report: null, error });
      });
    return () => controller.abort();
    // key covers every field of q; q itself may be a new object on each render
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  if (!q) return { report: null, isLoading: false, error: null };
  const isLoading = loaded.key !== key;
  return { report: loaded.report, isLoading, error: isLoading ? null : loaded.error };
}
