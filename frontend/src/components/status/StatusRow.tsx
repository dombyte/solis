import React, { useState } from 'react';
import { format, isSameDay } from 'date-fns';
import { History } from 'lucide-react';
import { Skeleton } from '../ui/skeleton';
import { cn } from '../../lib/utils';
import { formatAgo, formatDuration, formatNumber } from '../../lib/utils/format';
import type { StatusLevel } from '../../lib/utils/status';
import { faultFreeShare, type StatusDay } from '../../lib/utils/statusDays';
import { LEVEL_BG, LEVEL_TEXT } from './levelStyles';

const PERCENT = 100;
const SHARE_DIGITS = 1;
// The highest one-decimal value below 100 %: a few minutes of faults in a month round
// to "100.0 %", which would contradict the red bars.
const ALMOST_ALL = 99.9;

/** "100 % fault-free" only without any fault; otherwise at most "> 99.9 %". */
function shareLabel(share: number | null): string {
  if (share === null) return 'no data';
  if (share === 1) return '100 % fault-free';
  const pct = share * PERCENT;
  return pct > ALMOST_ALL
    ? `> ${formatNumber(ALMOST_ALL, SHARE_DIGITS)} % fault-free`
    : `${formatNumber(pct, SHARE_DIGITS)} % fault-free`;
}

const LEVEL_WORD: Record<StatusLevel, string> = {
  ok: 'No issues',
  notice: 'Not producing / limited',
  fault: 'Fault',
  unknown: 'No data',
};

export interface StatusRowData {
  id: string;
  name: string;
  description?: string;
  /** Current state as text and level. */
  text: string;
  level: StatusLevel;
  /** When the current value was received. */
  received: Date | null;
  /** One entry per day, oldest first; null while the history loads. */
  days: StatusDay[] | null;
}

interface StatusRowProps {
  row: StatusRowData;
  now: Date;
  today: Date;
  /** Opens the status log (on a day when one was clicked). */
  onOpen: (day?: Date) => void;
  /** Bars react to hover and click (mouse); on touch they are display only. */
  interactive: boolean;
  /** Labels below the bars: first and last day of the window. */
  fromLabel: string;
  toLabel: string;
}

function DayTooltip({ day, index, count, today }: { day: StatusDay; index: number; count: number; today: Date }): React.ReactElement {
  const center = ((index + 0.5) / count) * PERCENT;
  // Kept inside the row at both ends.
  const shift = center < 15 ? '0%' : center > 85 ? '-100%' : '-50%';
  const parts = (['fault', 'notice', 'ok'] as const).filter(l => day.time[l] > 0);

  return (
    <div
      role="tooltip"
      className="glass-panel pointer-events-none absolute bottom-full z-20 mb-2 w-56 rounded-lg p-3 text-xs shadow-lg"
      style={{ left: `${center}%`, transform: `translateX(${shift})` }}
    >
      <p className="font-semibold text-foreground">
        {isSameDay(day.day, today) ? `Today, ${format(day.day, 'd MMM')}` : format(day.day, 'EEE, d MMM yyyy')}
      </p>
      <p className={cn('mt-0.5 font-medium', LEVEL_TEXT[day.level])}>{LEVEL_WORD[day.level]}</p>
      {parts.length > 0 && (
        <dl className="mt-1.5 grid grid-cols-[1fr_auto] gap-x-3 gap-y-0.5">
          {parts.map(l => (
            <React.Fragment key={l}>
              <dt className="flex items-center gap-1.5 text-muted-foreground">
                <span className={cn('size-2 rounded-sm', LEVEL_BG[l])} aria-hidden="true" />
                {LEVEL_WORD[l]}
              </dt>
              <dd className="text-right tabular-nums text-foreground">{formatDuration(day.time[l])}</dd>
            </React.Fragment>
          ))}
        </dl>
      )}
      {day.issues.length > 0 && (
        <p className="mt-1.5 break-words text-muted-foreground">{day.issues.join(', ')}</p>
      )}
      <p className="mt-1.5 text-muted-foreground">Click for the log of this day</p>
    </div>
  );
}

/**
 * One status register in the overview list: name, current state and one bar per day
 * (worst level of the day: green no issues, orange not producing or limited, red fault,
 * gray no data). The header opens the status log, a bar opens it on that day.
 */
export function StatusRow({ row, now, today, onOpen, interactive, fromLabel, toLabel }: StatusRowProps): React.ReactElement {
  const [hovered, setHovered] = useState<number | null>(null);
  const share = row.days ? faultFreeShare(row.days) : null;
  const count = row.days?.length ?? 0;
  const ago = row.received ? formatAgo(row.received, now) ?? format(row.received, 'd MMM HH:mm') : null;

  return (
    <li className={cn('flex flex-col gap-2 px-4 py-3 sm:px-5', row.level === 'fault' && 'bg-destructive/5')}>
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => onOpen()}
          className="group flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-0.5 rounded text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={`${row.name}: ${row.text}. Show status log`}
        >
          <span className="flex min-w-0 items-center gap-2">
            <span className={cn('size-2.5 shrink-0 rounded-full', LEVEL_BG[row.level])} aria-hidden="true" />
            <span className="truncate font-medium group-hover:underline">{row.name}</span>
          </span>
          <span className={cn('min-w-0 break-words text-sm', row.level === 'fault' ? 'font-semibold text-destructive' : 'text-muted-foreground')}>
            {row.text}
          </span>
        </button>
        {ago && (
          <span className="hidden shrink-0 text-xs text-muted-foreground tabular-nums sm:inline" title={row.received ? `Received at ${format(row.received, 'd MMM yyyy, HH:mm:ss')}` : undefined}>
            {ago}
          </span>
        )}
        {/* Explicit "open the log" control (a bare chevron did not say what it does). */}
        <button
          type="button"
          onClick={() => onOpen()}
          className="inline-flex shrink-0 items-center gap-1 rounded-md bg-control px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-control-hover hover:text-foreground"
          aria-label={`Show status log of ${row.name}`}
        >
          <History className="size-3.5" aria-hidden="true" />
          Log
        </button>
      </div>

      {row.days === null ? (
        <Skeleton className="h-8 w-full" />
      ) : (
        <div className="relative" onPointerLeave={() => setHovered(null)}>
          <div className={cn('flex h-8 items-stretch', count > 45 ? 'gap-px' : 'gap-0.5 sm:gap-1')}>
            {row.days.map((d, i) => interactive ? (
              <button
                key={d.day.getTime()}
                type="button"
                // One tab stop per row (the header); the log has its own day navigation.
                tabIndex={-1}
                aria-label={`${format(d.day, 'd MMM yyyy')}: ${LEVEL_WORD[d.level]}`}
                onPointerEnter={e => { if (e.pointerType === 'mouse') setHovered(i); }}
                onClick={() => onOpen(d.day)}
                className={cn('min-w-0 flex-1 rounded-[2px] transition-opacity hover:opacity-80', LEVEL_BG[d.level])}
              />
            ) : (
              // Touch: too narrow to hit one day reliably; the row opens the log.
              <span
                key={d.day.getTime()}
                aria-hidden="true"
                className={cn('min-w-0 flex-1 rounded-[2px]', LEVEL_BG[d.level])}
              />
            ))}
          </div>
          {interactive && hovered !== null && row.days[hovered] && (
            <DayTooltip day={row.days[hovered]} index={hovered} count={count} today={today} />
          )}
        </div>
      )}

      <div aria-hidden="true" className="flex items-center gap-3 text-[11px] text-muted-foreground">
        <span>{fromLabel}</span>
        <span className="h-px flex-1 bg-border" />
        <span className="tabular-nums">
          {shareLabel(share)}
        </span>
        <span className="h-px flex-1 bg-border" />
        <span>{toLabel}</span>
      </div>
    </li>
  );
}
