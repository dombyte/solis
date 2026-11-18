import type { ReportResponse } from '../../lib/api/report';
import { figureDelta, figureLabel, figureUnit, figureValue, type Tone } from '../../lib/utils/reportFormat';
import { cn } from '../../lib/utils';
import { SURFACE } from '../ui/surface';

const TONE_CLASS: Record<Tone, string> = {
  better: 'text-success',
  worse: 'text-destructive',
  neutral: 'text-muted-foreground',
};

/**
 * The eight key figures of a report, with their change when there is a comparison. A
 * matched comparison (days are missing) shows both compared values next to the change:
 * they cover only the matched days, so they differ from the headline total.
 */
export function KeyFigures({ report }: { report: ReportResponse }) {
  const cmp = report.comparison;
  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-2.5 sm:gap-3.5">
      {report.figures.map(f => {
        // Without a single day both periods have data for there is nothing to compare;
        // the notes below say so instead of "no data" on every tile.
        const delta = cmp && !(cmp.matched && cmp.matched_days === 0) ? figureDelta(f) : null;
        const unit = figureUnit(f.kind);
        return (
          <div key={f.id} className={cn(SURFACE, "p-3 sm:p-4 flex flex-col gap-1 min-w-0")}>
            <span className="text-xs sm:text-sm font-medium text-muted-foreground">{figureLabel(f.id)}</span>
            <span className="text-xl sm:text-2xl font-bold tracking-tight truncate">
              {figureValue(f.value)}
              {f.value !== null && (
                <span className="ml-1 text-xs sm:text-sm font-medium text-muted-foreground">{figureUnit(f.kind)}</span>
              )}
            </span>
            {delta && (
              <span className="text-xs sm:text-sm">
                <span className={cn('font-semibold', TONE_CLASS[delta.tone])} aria-label={delta.label}>
                  {delta.text}
                </span>
                {f.base !== null && f.compared !== null && (
                  <span className="text-muted-foreground">
                    {cmp?.matched
                      ? ` · ${figureValue(f.compared)} vs ${figureValue(f.base)} ${unit}`
                      : ` vs ${figureValue(f.base)} ${unit}`}
                  </span>
                )}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}
