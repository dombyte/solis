import { format, parseISO } from 'date-fns';
import type { ReportBucket, ReportResponse } from '../../lib/api/report';
import { formatNumber } from '../../lib/utils/reportFormat';
import { cn } from '../../lib/utils';

const MAX_TICKS = 12;
const GRID_LINES = 5;

/** Rounds v up to 1, 2, 2.5 or 5 times a power of ten (same as the PDF axis). */
function niceMax(v: number): number {
  if (v <= 1) return 1;
  const exp = 10 ** Math.floor(Math.log10(v));
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * exp >= v) return m * exp;
  }
  return 10 * exp;
}

function bucketLabel(b: ReportBucket, g: ReportResponse['granularity'], oneMonth: boolean, oneYear: boolean): string {
  if (g === 'year') return b.key;
  if (g === 'month') return format(parseISO(b.from), oneYear ? 'MMM' : 'MMM yy');
  return oneMonth ? String(parseISO(b.from).getDate()) : format(parseISO(b.from), 'd MMM');
}

/** Tick labels that do not crowd the axis: 1, 5, 10 … within one month, else every n-th. */
function ticks(report: ReportResponse): string[] {
  const n = report.buckets.length;
  const oneMonth = report.from.slice(0, 7) === report.to.slice(0, 7);
  const oneYear = report.from.slice(0, 4) === report.to.slice(0, 4);
  const step = Math.ceil(n / MAX_TICKS);
  return report.buckets.map((b, i) => {
    const dom = parseISO(b.from).getDate();
    const show = report.granularity === 'day' && oneMonth && n > MAX_TICKS
      ? dom === 1 || dom % 5 === 0
      : i % step === 0;
    return show ? bucketLabel(b, report.granularity, oneMonth, oneYear) : '';
  });
}

/** PV yield vs consumption per bucket as a grouped bar chart. */
export function ReportChart({ report }: { report: ReportResponse }) {
  const top = niceMax(Math.max(0, ...report.buckets.flatMap(b =>
    b.summary.total ? [b.summary.total.pv, b.summary.total.consumption] : [])));
  const labels = ticks(report);
  const per = report.granularity;
  const incompleteCount = per === 'day' ? 0 : report.buckets.filter(b =>
    b.summary.complete_days > 0 && b.summary.complete_days < b.summary.days).length;
  const pvRange = report.summary.total
    ? `Total PV yield ${formatNumber(report.summary.total.pv)} kWh, consumption ${formatNumber(report.summary.total.consumption)} kWh.`
    : 'No data.';

  return (
    <section aria-labelledby="report-chart-title" className="rounded-xl border border-border bg-card p-4 sm:p-5 flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 id="report-chart-title" className="text-sm sm:text-base font-semibold">PV yield vs consumption per {per}</h3>
        <div className="flex gap-4 text-xs sm:text-sm text-muted-foreground">
          <span className="flex items-center gap-1.5"><span className="size-3 rounded-sm bg-flow-pv" />PV yield</span>
          <span className="flex items-center gap-1.5"><span className="size-3 rounded-sm bg-flow-hh" />Consumption</span>
        </div>
      </div>
      <div role="img" aria-label={`PV yield and consumption per ${per}. ${pvRange}`} className="flex gap-2">
        {/* Labels centred on their grid lines: top = i / GRID_LINES of the plot height. */}
        <div aria-hidden="true" className="relative h-48 sm:h-56 text-[11px] sm:text-xs text-muted-foreground w-12 shrink-0">
          {Array.from({ length: GRID_LINES + 1 }, (_, i) => {
            const v = top * (GRID_LINES - i) / GRID_LINES;
            return (
              <span key={i} className="absolute right-0 -translate-y-1/2 whitespace-nowrap" style={{ top: `${(i / GRID_LINES) * 100}%` }}>
                {i === 0 ? `${v} kWh` : v}
              </span>
            );
          })}
        </div>
        <div className="flex-1 min-w-0 flex flex-col gap-1.5">
          <div
            className="relative h-48 sm:h-56 flex items-end gap-[2px] border-b border-muted-foreground/60"
            style={{ backgroundImage: 'linear-gradient(var(--border) 1px, transparent 1px)', backgroundSize: `100% ${100 / GRID_LINES}%` }}
          >
            {report.buckets.map(b => {
              const t = b.summary.total;
              // A month or year with days without data: its bars sum fewer days.
              const incomplete = b.summary.complete_days < b.summary.days;
              const days = incomplete ? ` (data for ${b.summary.complete_days} of ${b.summary.days} days)` : '';
              const title = t
                ? `${b.key}: PV ${formatNumber(t.pv)} kWh, consumption ${formatNumber(t.consumption)} kWh${days}`
                : `${b.key}: no data`;
              return (
                <div key={b.key} title={title} className={cn('flex-1 h-full flex items-end gap-px min-w-0', incomplete && 'opacity-50')}>
                  {t && (
                    <>
                      <div className="flex-1 rounded-t-[2px] bg-flow-pv" style={{ height: `${Math.min(100, t.pv / top * 100)}%` }} />
                      <div className="flex-1 rounded-t-[2px] bg-flow-hh" style={{ height: `${Math.min(100, t.consumption / top * 100)}%` }} />
                    </>
                  )}
                </div>
              );
            })}
          </div>
          <div aria-hidden="true" className="flex gap-[2px] text-[11px] sm:text-xs text-muted-foreground">
            {/* tick labels */}
            {labels.map((l, i) => {
              // Small screens: every second shown label, so they never overlap.
              const shownIndex = labels.slice(0, i).filter(Boolean).length;
              const thin = labels.filter(Boolean).length > 6 && shownIndex % 2 === 1;
              return (
                <span key={report.buckets[i].key} className={cn('flex-1 text-center whitespace-nowrap min-w-0', thin && 'max-sm:invisible')}>
                  {l}
                </span>
              );
            })}
          </div>
        </div>
      </div>
      {incompleteCount > 0 && (
        <p className="text-xs sm:text-sm text-muted-foreground">
          Faded bars: {incompleteCount} {incompleteCount === 1 ? `${per} has` : `${per}s have`} data for only some of
          {incompleteCount === 1 ? ' its' : ' their'} days, so the bars sum fewer days.
        </p>
      )}
    </section>
  );
}
