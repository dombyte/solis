import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronDown, FileText } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import {
  barPercent, dayReportPath, formatKwh, formatShare, useEnergyTrend, type TrendDay,
} from '../../lib/hooks/useEnergyTrend';
import { formatNumber } from '../../lib/utils/format';
import { cn } from '../../lib/utils';

const DAYS = 7;
const PERCENT = 100;

function Bar({ value, top, color }: { value: number | null; top: number; color: string }): React.ReactElement {
  return (
    <div className="flex items-center gap-2">
      <div className="h-2 flex-1 min-w-0">
        {value !== null && (
          <div className={cn('h-full rounded-r-[2px]', color)} style={{ width: `${barPercent(value, top)}%` }} />
        )}
      </div>
      <span className="w-[4.75rem] shrink-0 text-right tabular-nums text-foreground">{formatKwh(value)}</span>
    </div>
  );
}

/** Export, import, self-sufficiency and (finished days only) the link to the day report. */
function Details({ day }: { day: TrendDay }): React.ReactElement {
  const rows: [string, string][] = [
    ['Grid export', formatKwh(day.exported)],
    ['Grid import', formatKwh(day.imported)],
    ['Self-sufficiency', formatShare(day.selfSufficiency)],
  ];
  return (
    <div className="pb-3 pl-[5.25rem] text-xs">
      <dl className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1">
        {rows.map(([label, value]) => (
          <React.Fragment key={label}>
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="text-right tabular-nums text-foreground">{value}</dd>
          </React.Fragment>
        ))}
      </dl>
      {day.today ? (
        <p className="mt-2 text-muted-foreground">The day report is available from tomorrow.</p>
      ) : (
        <Link
          to={dayReportPath(day)}
          className="mt-3 flex h-11 items-center justify-center gap-2 rounded-lg border border-border bg-control text-sm font-medium hover:bg-control-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <FileText className="size-4" aria-hidden="true" />
          Open day report
        </Link>
      )}
    </div>
  );
}

interface RowProps {
  day: TrendDay;
  top: number;
  open: boolean;
  onToggle: () => void;
}

function Row({ day, top, open, onToggle }: RowProps): React.ReactElement {
  const detailsID = `trend-day-${day.key}`;
  return (
    // The divider is the li's straight border; the highlight is a flat band on an inner block
    // so it never bends the line.
    <li>
      <div className={cn('px-2', open && 'bg-control-hover/50')}>
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={open}
          aria-controls={detailsID}
          aria-label={`${day.long}${day.today ? ' (so far)' : ''}: PV ${formatKwh(day.pv)}, consumption ${formatKwh(day.consumption)}`}
          className="flex min-h-12 w-full items-center gap-3 py-2 text-left text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-md"
        >
          <div className={cn('w-[4.5rem] shrink-0 leading-tight', day.today && 'opacity-60')}>
            <p className="font-medium text-foreground">{day.today ? 'Today' : day.weekday}</p>
            <p className="text-muted-foreground">{day.today ? 'so far' : day.label}</p>
          </div>
          <div className={cn('flex-1 min-w-0 flex flex-col gap-1', day.today && 'opacity-60')}>
            <Bar value={day.pv} top={top} color="bg-flow-pv" />
            <Bar value={day.consumption} top={top} color="bg-flow-hh" />
          </div>
          <span className="w-9 shrink-0 text-right tabular-nums text-muted-foreground">
            {day.selfSufficiency === null ? '' : `${formatNumber(day.selfSufficiency * PERCENT, 0)}%`}
          </span>
          <ChevronDown
            className={cn('size-4 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none', open && 'rotate-180')}
            aria-hidden="true"
          />
        </button>
        {open && <div id={detailsID}><Details day={day} /></div>}
      </div>
    </li>
  );
}

/**
 * Phone variant of EnergyTrend: the last 7 days as one row per day, newest first, PV and
 * consumption as horizontal bars on one common scale with their values and the
 * self-sufficiency next to them. A tap only expands a row (export, import and a button to
 * the day report), so a stray tap while scrolling never navigates away.
 */
export function EnergyTrendList({ className = '' }: { className?: string }): React.ReactElement {
  const { days, top, error } = useEnergyTrend(DAYS);
  const [open, setOpen] = useState<string | null>(null);

  return (
    <Card className={cn('w-full min-w-0', className)}>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>Last {DAYS} days</CardTitle>
        <div className="flex gap-3 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5"><span className="size-2.5 rounded-sm bg-flow-pv" />PV</span>
          <span className="flex items-center gap-1.5"><span className="size-2.5 rounded-sm bg-flow-hh" />Consumption</span>
          <span>% self-suff.</span>
        </div>
      </CardHeader>
      <CardContent>
        {error && <p className="mb-2 text-xs text-destructive">{error}</p>}
        <ul aria-label={`PV yield and consumption per day, last ${DAYS} days`} className="-mx-2 divide-y divide-border">
          {[...days].reverse().map(d => (
            <Row
              key={d.key}
              day={d}
              top={top}
              open={open === d.key}
              onToggle={() => setOpen(o => (o === d.key ? null : d.key))}
            />
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
