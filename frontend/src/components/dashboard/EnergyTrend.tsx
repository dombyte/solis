import React, { useEffect, useMemo } from 'react';
import { format, subDays } from 'date-fns';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import { useHistory } from '../../lib/hooks/useHistory';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { formatNumber } from '../../lib/utils/format';
import { cn } from '../../lib/utils';

const DAYS = 14;
const GRID_LINES = 4;
// Live register (today) and the key of its daily history rows.
const PV = { id: 'pv_energy_today', key: 'pv_energy_daily' };
const CONSUMPTION = { id: 'energy_consumption_today', key: 'energy_consumption_daily' };
const SERIES = [PV.id, CONSUMPTION.id];
// Plot height: what the window leaves below the diagram row, within bounds.
const PLOT_HEIGHT = 'h-[clamp(10rem,calc(100dvh-46rem),24rem)]';

/** Rounds v up to 1, 2, 2.5 or 5 times a power of ten (same axis rule as the reports). */
function niceMax(v: number): number {
  if (v <= 1) return 1;
  const exp = 10 ** Math.floor(Math.log10(v));
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * exp >= v) return m * exp;
  }
  return 10 * exp;
}

interface Day {
  key: string;
  label: string;
  pv: number | null;
  consumption: number | null;
  today: boolean;
}

/** The store's live value of a register, or null. */
function useLive(id: string): number | null {
  const v = useRegisterStore(state => state.registerValues.get(id)?.value);
  return typeof v === 'number' ? v : null;
}

/**
 * PV yield vs consumption of the last 14 days (today so far included) as grouped bars.
 * Past days come from the daily history (REST, loaded once per day); today's bars follow
 * the live counters. A day without data is an empty slot, never a 0 kWh bar.
 */
export function EnergyTrend({ className = '' }: { className?: string }): React.ReactElement {
  const { data, error, loadHistory } = useHistory();
  const livePV = useLive(PV.id);
  const liveConsumption = useLive(CONSUMPTION.id);

  // Recomputed on every render (live frames re-render often), so the window moves on at
  // midnight and the effect below reloads the history for the new range.
  const todayKey = format(new Date(), 'yyyy-MM-dd');
  const startKey = format(subDays(new Date(), DAYS - 1), 'yyyy-MM-dd');

  useEffect(() => {
    loadHistory(SERIES, startKey, todayKey);
  }, [loadHistory, startKey, todayKey]);

  const days: Day[] = useMemo(() => {
    // useHistory labels rows "d MMM yyyy"; match the slots by that label.
    const index = new Map((data?.labels ?? []).map((l, i) => [l, i]));
    const valueOf = (key: string, i: number | undefined): number | null => {
      if (i === undefined) return null;
      const ds = data?.datasets.find(d => d.key === key);
      return ds?.data[i] ?? null;
    };
    return Array.from({ length: DAYS }, (_, n) => {
      const date = subDays(new Date(`${todayKey}T00:00:00`), DAYS - 1 - n);
      const today = n === DAYS - 1;
      const i = index.get(format(date, 'd MMM yyyy'));
      return {
        key: format(date, 'yyyy-MM-dd'),
        label: format(date, 'd MMM'),
        pv: today ? livePV ?? valueOf(PV.key, i) : valueOf(PV.key, i),
        consumption: today ? liveConsumption ?? valueOf(CONSUMPTION.key, i) : valueOf(CONSUMPTION.key, i),
        today,
      };
    });
  }, [data, livePV, liveConsumption, todayKey]);

  const top = niceMax(Math.max(0, ...days.flatMap(d => [d.pv ?? 0, d.consumption ?? 0])));

  return (
    <Card className={cn('w-full min-w-0', className)}>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>Last {DAYS} days</CardTitle>
        <div className="flex gap-4 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5"><span className="size-2.5 rounded-sm bg-flow-pv" />PV yield</span>
          <span className="flex items-center gap-1.5"><span className="size-2.5 rounded-sm bg-flow-hh" />Consumption</span>
        </div>
      </CardHeader>
      <CardContent>
        {error && <p className="mb-2 text-xs text-destructive">{error}</p>}
        <div role="img" aria-label={`PV yield and consumption per day, last ${DAYS} days`} className={cn('flex gap-2', PLOT_HEIGHT)}>
          <div aria-hidden="true" className="relative w-12 shrink-0 mb-5 text-[11px] text-muted-foreground tabular-nums">
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
              className="relative flex-1 flex items-end gap-1 sm:gap-2 border-b border-muted-foreground/60"
              style={{ backgroundImage: 'linear-gradient(var(--border) 1px, transparent 1px)', backgroundSize: `100% ${100 / GRID_LINES}%` }}
            >
              {days.map(d => {
                const title = d.pv === null && d.consumption === null
                  ? `${d.label}: no data`
                  : `${d.label}${d.today ? ' (so far)' : ''}: PV ${d.pv === null ? '–' : formatNumber(d.pv, 1)} kWh, consumption ${d.consumption === null ? '–' : formatNumber(d.consumption, 1)} kWh`;
                return (
                  <div key={d.key} title={title} className={cn('flex-1 h-full flex items-end gap-px min-w-0', d.today && 'opacity-60')}>
                    {d.pv !== null && <div className="flex-1 rounded-t-[2px] bg-flow-pv" style={{ height: `${Math.min(100, d.pv / top * 100)}%` }} />}
                    {d.consumption !== null && <div className="flex-1 rounded-t-[2px] bg-flow-hh" style={{ height: `${Math.min(100, d.consumption / top * 100)}%` }} />}
                  </div>
                );
              })}
            </div>
            <div aria-hidden="true" className="flex gap-1 sm:gap-2 h-4 text-[11px] text-muted-foreground">
              {days.map((d, i) => (
                // Every second label on narrow screens so they never overlap; counted from
                // the end, so "Today" always stays.
                <span key={d.key} className={cn('flex-1 min-w-0 text-center whitespace-nowrap', (DAYS - 1 - i) % 2 === 1 && 'max-md:invisible')}>
                  {d.today ? 'Today' : d.label}
                </span>
              ))}
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
