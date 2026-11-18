// Energy report API (GET /api/report): JSON for the preview, CSV/PDF as downloads.
// Energy in kWh, shares in percent, changes of ratios in percentage points; null = no data.

export type CompareMode = 'last_year' | 'previous' | 'none';
export type ReportFormat = 'json' | 'csv' | 'pdf';
export type Granularity = 'day' | 'month' | 'year';

interface ReportEnergy {
  pv: number;
  consumption: number;
  household: number;
  backup: number;
  grid_import: number;
  grid_export: number;
  battery_charge: number;
  battery_discharge: number;
}

interface ReportSummary {
  days: number;
  complete_days: number;
  total: ReportEnergy | null;
  per_day: ReportEnergy | null;
  self_sufficiency_pct: number | null;
  self_consumption_pct: number | null;
  /** Battery charge that must have come from the grid (lower bound), kWh. */
  battery_charge_from_grid_min: number | null;
  grid_charging_days: string[];
}

export interface ReportFigure {
  id: string;
  kind: 'energy' | 'ratio';
  better: 'higher' | 'lower' | 'neutral';
  /** The period's value over all its complete days. */
  value: number | null;
  /**
   * The two values the change is computed from. With a matched comparison both cover
   * only the matched days; otherwise `compared` equals `value`.
   */
  compared: number | null;
  base: number | null;
  change: number | null;
  change_pct: number | null;
}

export interface ReportBucket {
  key: string;
  from: string;
  to: string;
  partial: boolean;
  summary: ReportSummary;
}

export interface ReportResponse {
  from: string;
  to: string;
  granularity: Granularity;
  summary: ReportSummary;
  figures: ReportFigure[];
  comparison: {
    mode: Exclude<CompareMode, 'none'>;
    from: string;
    to: string;
    summary: ReportSummary;
    /**
     * Days are missing or the periods differ in length: the figures compare only the
     * days both periods have data for.
     */
    matched: boolean;
    matched_days: number;
  } | null;
  buckets: ReportBucket[];
  /** The requested range ended today: today is in progress, the report ends yesterday (`to`). */
  today_excluded: boolean;
}

export interface ReportQuery {
  from: string; // YYYY-MM-DD
  to: string;   // YYYY-MM-DD, inclusive
  compare: CompareMode;
  granularity?: Granularity;
}

/** Builds the /api/report URL of q in the given format. */
function reportUrl(q: ReportQuery, format: ReportFormat): string {
  const params = new URLSearchParams({ from: q.from, to: q.to, compare: q.compare, format });
  if (q.granularity) params.set('granularity', q.granularity);
  return `/api/report?${params.toString()}`;
}

/** Throws the server's error message (the documented error shape) for a failed response. */
async function raiseFor(response: Response): Promise<never> {
  let message = `Request failed (${response.status})`;
  try {
    const body = (await response.json()) as { message?: string };
    if (body.message) message = body.message;
  } catch {
    // not JSON: keep the status message
  }
  throw new Error(message);
}

export async function fetchReport(q: ReportQuery, signal?: AbortSignal): Promise<ReportResponse> {
  const response = await fetch(reportUrl(q, 'json'), { signal });
  if (!response.ok) await raiseFor(response);
  return (await response.json()) as ReportResponse;
}

/**
 * Downloads the report as a file. Fetched first (not a plain link) so a server error is
 * shown in the page instead of being saved as a file.
 */
export async function downloadReport(q: ReportQuery, format: Exclude<ReportFormat, 'json'>): Promise<void> {
  const response = await fetch(reportUrl(q, format));
  if (!response.ok) await raiseFor(response);
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const name = /filename="([^"]+)"/.exec(disposition)?.[1] ?? `solis-report-${q.from}_${q.to}.${format}`;
  const url = URL.createObjectURL(await response.blob());
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  // Revoke later: some browsers start the download asynchronously.
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}
