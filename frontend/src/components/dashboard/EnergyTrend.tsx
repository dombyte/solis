import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import {
  barPercent, dayReportPath, formatKwh as kwh, formatShare as pct, useEnergyTrend, type TrendDay as Day,
} from '../../lib/hooks/useEnergyTrend';
import { formatNumber } from '../../lib/utils/format';
import { cn } from '../../lib/utils';

const GRID_LINES = 4;
const PERCENT = 100;
// Plot height: what the window leaves below the diagram row, within bounds.
const PLOT_HEIGHT = 'h-[clamp(10rem,calc(100dvh-46rem),24rem)]';

function Tooltip({ day, index, count, pinned }: { day: Day; index: number; count: number; pinned: boolean }): React.ReactElement {
  // Centered over its day, but kept inside the plot at both ends.
  const center = ((index + 0.5) / count) * PERCENT;
  const shift = index < 2 ? '0%' : index > count - 3 ? '-100%' : '-50%';
  const rows: [string, string, string?][] = [
    ['PV yield', kwh(day.pv), 'bg-flow-pv'],
    ['Consumption', kwh(day.consumption), 'bg-flow-hh'],
    ['Grid export', kwh(day.exported)],
    ['Grid import', kwh(day.imported)],
    ['Self-sufficiency', pct(day.selfSufficiency)],
  ];

  return (
    <div
      role="tooltip"
      className={cn('glass-panel absolute top-1 z-10 w-52 rounded-lg p-3 text-xs shadow-lg', !pinned && 'pointer-events-none')}
      style={{ left: `${center}%`, transform: `translateX(${shift})` }}
    >
      <p className="mb-1.5 font-semibold text-foreground">{day.long}{day.today && ' (so far)'}</p>
      <dl className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-0.5">
        {rows.map(([label, value, swatch]) => (
          <React.Fragment key={label}>
            <dt className="flex items-center gap-1.5 text-muted-foreground">
              {swatch && <span className={cn('size-2 rounded-sm', swatch)} aria-hidden="true" />}
              {label}
            </dt>
            <dd className="text-right tabular-nums text-foreground">{value}</dd>
          </React.Fragment>
        ))}
      </dl>
      {pinned && !day.today && (
        <Link to={dayReportPath(day)} className="mt-2 inline-block font-medium text-primary hover:underline">
          Open day report
        </Link>
      )}
      {!pinned && !day.today && <p className="mt-2 text-muted-foreground">Click for the day report</p>}
    </div>
  );
}

interface EnergyTrendProps {
  /** Number of days shown, today included. */
  days?: number;
  className?: string;
}

/**
 * PV yield vs consumption of the last days (today so far included) as grouped bars, with
 * export, import and self-sufficiency per day in the tooltip and as a row below (data:
 * useEnergyTrend). A day without data is an empty slot, never a 0 kWh bar. A mouse click
 * on a finished day opens its report; on touch a tap shows the tooltip with that link.
 * Phones get EnergyTrendList instead.
 */
export function EnergyTrend({ days: count = 14, className = '' }: EnergyTrendProps): React.ReactElement {
  const { days, top, error } = useEnergyTrend(count);
  const navigate = useNavigate();
  const [hovered, setHovered] = useState<number | null>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const active = hovered ?? pinned;

  const onPointerUp = (e: React.PointerEvent, i: number) => {
    const day = days[i];
    if (e.pointerType === 'mouse') {
      if (!day.today) navigate(dayReportPath(day));
      return;
    }
    setPinned(p => (p === i ? null : i));
  };

  return (
    <Card className={cn('w-full min-w-0', className)}>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>Last {count} days</CardTitle>
        <div className="flex gap-4 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5"><span className="size-2.5 rounded-sm bg-flow-pv" />PV yield</span>
          <span className="flex items-center gap-1.5"><span className="size-2.5 rounded-sm bg-flow-hh" />Consumption</span>
          <span className="max-md:hidden">% = self-sufficiency</span>
        </div>
      </CardHeader>
      <CardContent>
        {error && <p className="mb-2 text-xs text-destructive">{error}</p>}
        <div className={cn('flex gap-2', PLOT_HEIGHT)}>
          <div aria-hidden="true" className="relative w-12 shrink-0 mb-5 md:mb-10 text-[11px] text-muted-foreground tabular-nums">
            {Array.from({ length: GRID_LINES + 1 }, (_, i) => {
              const v = top * (GRID_LINES - i) / GRID_LINES;
              return (
                <span key={i} className="absolute right-0 -translate-y-1/2 whitespace-nowrap" style={{ top: `${(i / GRID_LINES) * 100}%` }}>
                  {i === 0 ? `${v} kWh` : v}
                </span>
              );
            })}
          </div>
          <div className="flex-1 min-w-0 flex flex-col gap-1">
            <div
              role="group"
              aria-label={`PV yield and consumption per day, last ${count} days`}
              className="relative flex-1 flex items-end gap-1 sm:gap-2 border-b border-muted-foreground/60"
              style={{ backgroundImage: 'linear-gradient(var(--border) 1px, transparent 1px)', backgroundSize: `100% ${100 / GRID_LINES}%` }}
              onPointerLeave={e => { if (e.pointerType === 'mouse') setHovered(null); }}
            >
              {days.map((d, i) => (
                <button
                  key={d.key}
                  type="button"
                  aria-label={`${d.long}${d.today ? ' (so far)' : ''}: PV ${kwh(d.pv)}, consumption ${kwh(d.consumption)}, export ${kwh(d.exported)}, import ${kwh(d.imported)}, self-sufficiency ${pct(d.selfSufficiency)}`}
                  className={cn(
                    'flex-1 h-full flex items-end gap-px min-w-0 rounded-t-sm outline-none focus-visible:ring-2 focus-visible:ring-ring',
                    active === i && 'bg-control-hover/60',
                    d.today ? 'opacity-60' : 'cursor-pointer',
                  )}
                  // Mouse hover and keyboard focus only: a tap also fires emulated
                  // mouse and focus events, and must pin the tooltip instead.
                  onPointerEnter={e => { if (e.pointerType === 'mouse') setHovered(i); }}
                  onFocus={e => { if (e.currentTarget.matches(':focus-visible')) setHovered(i); }}
                  onBlur={() => setHovered(null)}
                  onPointerUp={e => onPointerUp(e, i)}
                  onKeyDown={e => {
                    if (e.key === 'Enter' && !d.today) navigate(dayReportPath(d));
                  }}
                >
                  {d.pv !== null && <span className="flex-1 rounded-t-[2px] bg-flow-pv" style={{ height: `${barPercent(d.pv, top)}%` }} />}
                  {d.consumption !== null && <span className="flex-1 rounded-t-[2px] bg-flow-hh" style={{ height: `${barPercent(d.consumption, top)}%` }} />}
                </button>
              ))}
              {active !== null && <Tooltip day={days[active]} index={active} count={count} pinned={hovered === null} />}
            </div>
            <div aria-hidden="true" className="flex gap-1 sm:gap-2 h-4 text-[11px] text-muted-foreground">
              {days.map((d, i) => (
                // Every second label on narrow screens so they never overlap; counted from
                // the end, so "Today" always stays.
                <span key={d.key} className={cn('flex-1 min-w-0 text-center whitespace-nowrap', count > 7 && (count - 1 - i) % 2 === 1 && 'max-md:invisible')}>
                  {d.today ? 'Today' : d.label}
                </span>
              ))}
            </div>
            <div aria-hidden="true" className="max-md:hidden flex gap-1 sm:gap-2 h-4 text-[11px] text-muted-foreground tabular-nums">
              {days.map(d => (
                <span key={d.key} className="flex-1 min-w-0 text-center whitespace-nowrap">
                  {d.selfSufficiency === null ? '' : `${formatNumber(d.selfSufficiency * PERCENT, 0)}%`}
                </span>
              ))}
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
