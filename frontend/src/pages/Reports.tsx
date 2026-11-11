import React, { useMemo, useState } from 'react';
import { parseISO, format } from 'date-fns';
import { AlertTriangle, ArrowDown, ArrowRight, CheckCircle2, Clock, Download, FileText, Loader2 } from 'lucide-react';
import { Button } from '../components/ui/button';
import { CompareSelect } from '../components/reports/CompareSelect';
import { KeyFigures } from '../components/reports/KeyFigures';
import { PeriodPicker } from '../components/reports/PeriodPicker';
import { ReportChart } from '../components/reports/ReportChart';
import { useMobile } from '../hooks/useMobile';
import { useReport } from '../lib/hooks/useReport';
import { useToday } from '../lib/hooks/useToday';
import { downloadReport, type CompareMode, type Granularity, type ReportQuery } from '../lib/api/report';
import {
  autoGranularity, finishedRange, periodName, presetRange, rangeDays, rangeProblem, type DayRange,
} from '../lib/utils/reportRange';
import { formatNumber } from '../lib/utils/reportFormat';
import { cn } from '../lib/utils';

const GRANULARITY_LABEL: Record<Granularity, string> = { day: 'Per day', month: 'Per month', year: 'Per year' };

export function Reports(): React.ReactElement {
  const isMobile = useMobile();
  const today = useToday();
  const [range, setRange] = useState<DayRange>(() => presetRange('last_month', today));
  const [compare, setCompare] = useState<CompareMode>('last_year');
  const [csvGranularity, setCsvGranularity] = useState<Granularity>(() => autoGranularity(range));
  const [download, setDownload] = useState<{ busy: 'pdf' | 'csv' | null; error: string | null }>({ busy: null, error: null });

  const problem = rangeProblem(range, today);
  const query = useMemo<ReportQuery | null>(
    () => (problem ? null : { from: range.from, to: range.to, compare }),
    [problem, range.from, range.to, compare],
  );
  const { report, isLoading, error } = useReport(query);

  const applyRange = (r: DayRange) => {
    setRange(r);
    setCsvGranularity(autoGranularity(r));
    setDownload(d => ({ ...d, error: null }));
  };
  const runDownload = async (fmt: 'pdf' | 'csv') => {
    if (!query) return;
    setDownload({ busy: fmt, error: null });
    try {
      await downloadReport(fmt === 'csv' ? { ...query, granularity: csvGranularity } : query, fmt);
      setDownload({ busy: null, error: null });
    } catch (err) {
      setDownload({ busy: null, error: err instanceof Error ? err.message : 'Download failed.' });
    }
  };

  const days = rangeDays(range);

  return (
    <div className="p-3 sm:p-4 md:p-6 lg:p-8 w-full max-w-6xl flex flex-col gap-5 sm:gap-6">
      <header className="flex flex-col gap-1.5">
        {!isMobile && <h1 className="text-2xl font-bold">Reports</h1>}
        <p className="text-sm text-muted-foreground">
          Energy report for a period: PDF with charts and key figures, CSV with the raw numbers.
        </p>
      </header>

      <div className="flex flex-col lg:flex-row gap-4 sm:gap-5">
        <section aria-labelledby="period-title" className="flex-[2] min-w-0 rounded-xl border border-border bg-card p-4 sm:p-5 flex flex-col gap-3">
          <h2 id="period-title" className="text-base font-semibold">Period and comparison</h2>
          {/* Like comparing two branches: the report period on the left, what it is compared with on the right. */}
          <div className="flex flex-col md:flex-row md:items-center gap-2">
            <div className="flex-1 min-w-0">
              <PeriodPicker range={range} today={today} onChange={applyRange} />
            </div>
            <ArrowRight aria-hidden="true" className="hidden md:block size-5 shrink-0 text-muted-foreground" />
            <ArrowDown aria-hidden="true" className="md:hidden size-4 self-center text-muted-foreground" />
            <div className="flex-1 min-w-0">
              {/* The server compares the finished days, so the options show those dates. */}
              <CompareSelect mode={compare} range={finishedRange(range, today)} onChange={setCompare} />
            </div>
          </div>
          <span className="text-sm text-muted-foreground">
            {days} {days === 1 ? 'day' : 'days'} · charts grouped by{' '}
            {autoGranularity(finishedRange(range, today))}
          </span>
          {problem && <p className="text-sm text-destructive" role="alert">{problem}</p>}
        </section>

        <section aria-labelledby="download-title" className="flex-1 rounded-xl border border-border bg-card p-4 sm:p-5 flex flex-col gap-4">
          <h2 id="download-title" className="text-base font-semibold">Download</h2>
          <div className="flex flex-col gap-1.5">
            <Button type="button" size="lg" className="h-12 text-base gap-2" disabled={!query || download.busy !== null} onClick={() => runDownload('pdf')}>
              {download.busy === 'pdf' ? <Loader2 className="animate-spin" /> : <FileText />}
              Download PDF report
            </Button>
            <span className="text-xs text-muted-foreground">Charts, key figures and a compact table</span>
          </div>
          <div className="h-px bg-border" />
          <div className="flex flex-col gap-2">
            <label htmlFor="csv-granularity" className="text-sm font-semibold">CSV rows</label>
            <div className="flex flex-wrap gap-2">
              <select
                id="csv-granularity"
                value={csvGranularity}
                onChange={e => setCsvGranularity(e.target.value as Granularity)}
                className="flex-1 min-w-32 h-11 rounded-lg border border-border bg-background px-3 text-sm"
              >
                {(['day', 'month', 'year'] as Granularity[]).map(g => (
                  <option key={g} value={g}>{GRANULARITY_LABEL[g]}</option>
                ))}
              </select>
              <Button type="button" variant="outline" className="flex-1 min-w-32 h-11 gap-2" disabled={!query || download.busy !== null} onClick={() => runDownload('csv')}>
                {download.busy === 'csv' ? <Loader2 className="animate-spin" /> : <Download />}
                Download CSV
              </Button>
            </div>
          </div>
          {download.error && <p className="text-sm text-destructive" role="alert">{download.error}</p>}
        </section>
      </div>

      <section aria-labelledby="preview-title" aria-busy={isLoading} className="flex flex-col gap-3.5">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 id="preview-title" className="text-lg font-semibold">Preview · {periodName(range)}</h2>
          {report?.comparison && (
            <span className="text-sm text-muted-foreground">
              Compared with {periodName({ from: report.comparison.from, to: report.comparison.to })}
            </span>
          )}
        </div>
        {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
        {!report && isLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground py-10 justify-center">
            <Loader2 className="size-4 animate-spin" /> Loading report…
          </div>
        )}
        {report && (
          <div className={cn('flex flex-col gap-4 transition-opacity', isLoading && 'opacity-60')}>
            <KeyFigures report={report} />
            <ReportChart report={report} />
            <ReportNotes report={report} />
          </div>
        )}
      </section>
    </div>
  );
}

/** Data completeness and grid-charging notes below the preview. */
function ReportNotes({ report }: { report: NonNullable<ReturnType<typeof useReport>['report']> }) {
  const s = report.summary;
  const charged = s.grid_charging_days;
  const cmp = report.comparison;
  return (
    <div className="flex flex-col gap-2 text-sm">
      <Note icon={s.complete_days === s.days ? 'ok' : 'warn'}>
        {s.complete_days === s.days
          ? `Data complete: ${s.days} of ${s.days} days.`
          : `Data for ${s.complete_days} of ${s.days} days; missing days are left out of every figure.`}
        {cmp?.matched && (cmp.matched_days > 0
          ? ` ${s.complete_days === s.days && cmp.summary.complete_days === cmp.summary.days ? 'The two periods differ in length' : 'Days are missing in one of the periods'}, so the changes compare only the ${cmp.matched_days} ${cmp.matched_days === 1 ? 'day' : 'days'} both periods have data for (the two values next to each change).`
          : ' The two periods have no days in common to compare.')}
      </Note>
      {report.today_excluded && (
        <Note icon="today">
          Today is still in progress and not part of the report: it ends on {format(parseISO(report.to), 'd MMM yyyy')}.
        </Note>
      )}
      {charged.length > 0 && (
        <Note icon="warn">
          On {charged.length === 1 ? format(parseISO(charged[0]), 'd MMM') : `${charged.length} days`} the battery took
          in more energy than the solar panels could supply: at least{' '}
          {formatNumber(s.battery_charge_from_grid_min ?? 0)} kWh came from the grid. Daily counters cannot show grid
          charging on sunny days, so the split of the solar energy is an estimate.
        </Note>
      )}
    </div>
  );
}

const NOTE_ICONS = {
  ok: { Icon: CheckCircle2, className: 'text-success' },
  warn: { Icon: AlertTriangle, className: 'text-warning' },
  today: { Icon: Clock, className: 'text-muted-foreground' },
};

function Note({ icon, children }: { icon: keyof typeof NOTE_ICONS; children: React.ReactNode }) {
  const { Icon, className } = NOTE_ICONS[icon];
  return (
    <div className="flex items-start gap-2.5 rounded-xl border border-border bg-card px-3.5 py-3">
      <Icon aria-hidden="true" className={cn('size-4 mt-0.5 shrink-0', className)} />
      <span>{children}</span>
    </div>
  );
}
