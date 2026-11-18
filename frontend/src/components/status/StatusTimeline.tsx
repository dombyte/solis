import React, { useEffect, useMemo, useRef, useState } from 'react';
import { addDays, format, isSameDay, startOfDay, subDays } from 'date-fns';
import { ChevronLeft, ChevronRight, History } from 'lucide-react';
import { STATUS_ROW_CAP, useStatusHistory } from '../../lib/hooks/useStatusHistory';
import { useNow } from '../../lib/hooks/useNow';
import { useToday } from '../../lib/hooks/useToday';
import { cn } from '../../lib/utils';
import { formatDuration } from '../../lib/utils/format';
import { Button } from '../ui/button';
import { Skeleton } from '../ui/skeleton';
import { LEVEL_BG } from './levelStyles';
import { daySegments, toChanges, type StatusChange } from '../../lib/utils/statusDays';

/** Days loaded when the dialog opens, and per "Load older" click. */
const FIRST_DAYS = 7;
const OLDER_DAYS = 30;
const DAY_KEY = 'yyyy-MM-dd';
const NOW_REFRESH_MS = 30_000;
const STRIP_TICKS = [0, 6, 12, 18, 24];
const ISSUE_MIN_PX = 3;

function DayStrip({ changes, day, now }: { changes: StatusChange[]; day: Date; now: Date }): React.ReactElement {
  const dayStart = startOfDay(day);
  // Real length of the day (23 or 25 hours on DST days).
  const length = addDays(dayStart, 1).getTime() - dayStart.getTime();
  const segments = daySegments(changes, day, now);
  const pct = (d: Date) => ((d.getTime() - dayStart.getTime()) / length) * 100;

  return (
    <div>
      <div className="relative h-6 overflow-hidden rounded-md bg-muted">
        {segments.map(s => (
          <div
            key={s.from.getTime()}
            title={`${format(s.from, 'HH:mm')}–${format(s.to, 'HH:mm')}: ${s.text}`}
            className={cn('absolute inset-y-0 border-r border-background/60', LEVEL_BG[s.level], s.level !== 'ok' && 'z-10')}
            // A one-minute fault is 0.07 % of a day: keep issues at least visible.
            style={{ left: `${pct(s.from)}%`, width: `${pct(s.to) - pct(s.from)}%`, minWidth: s.level === 'ok' ? undefined : ISSUE_MIN_PX }}
          />
        ))}
      </div>
      <div aria-hidden="true" className="relative mt-1 h-4 text-[11px] text-muted-foreground tabular-nums">
        {STRIP_TICKS.map(h => (
          <span
            key={h}
            className={cn('absolute', h === 0 ? 'left-0' : h === 24 ? 'right-0' : '-translate-x-1/2')}
            style={h > 0 && h < 24 ? { left: `${(h / 24) * 100}%` } : undefined}
          >
            {String(h).padStart(2, '0')}:00
          </span>
        ))}
      </div>
      <ul className="sr-only">
        {segments.map(s => (
          <li key={s.from.getTime()}>{`${format(s.from, 'HH:mm')} to ${format(s.to, 'HH:mm')}: ${s.text}`}</li>
        ))}
      </ul>
    </div>
  );
}

function ChangeRow({ change, now }: { change: StatusChange; now: Date }): React.ReactElement {
  const duration = (change.until ?? now).getTime() - change.at.getTime();
  return (
    <li className="flex items-baseline gap-3 rounded px-2 py-1.5 hover:bg-control-hover">
      <span className="w-16 shrink-0 text-xs text-muted-foreground tabular-nums">{format(change.at, 'HH:mm:ss')}</span>
      <span className={cn('mt-1.5 size-2 shrink-0 self-start rounded-full', LEVEL_BG[change.level])} aria-hidden="true" />
      <span className={cn('min-w-0 flex-1 break-words text-sm', change.level === 'fault' && 'font-medium text-destructive')}>
        {change.text}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
        {change.until ? formatDuration(duration) : `${formatDuration(duration)}, ongoing`}
      </span>
    </li>
  );
}

interface StatusTimelineProps {
  /** Register id (decides how its values are rated). */
  id: string;
  /** Its /api/data source. */
  source: string;
  /** Day shown first (default today); the first load reaches back to it. */
  initialDay?: Date;
}

/**
 * The change history of a status register: a 24-hour strip of the selected day and the
 * changes grouped by day with how long each state lasted. Loads the last week first and
 * older months on demand (start/end on the data endpoint).
 */
export function StatusTimeline({ id, source, initialDay }: StatusTimelineProps): React.ReactElement {
  const today = useToday();
  const now = useNow(NOW_REFRESH_MS);
  const [selected, setSelected] = useState(() => startOfDay(initialDay ?? today));
  const { entries, before, from: loadedFrom, loading, error, capped, load, abort } = useStatusHistory(id, source);
  // The last week, or back to the day picked in the overview if that is older.
  const firstFrom = useMemo(() => {
    const week = subDays(today, FIRST_DAYS - 1);
    return initialDay && startOfDay(initialDay) < week ? startOfDay(initialDay) : week;
  }, [today, initialDay]);
  const from = loadedFrom ?? firstFrom;
  const pending = loadedFrom === null && error === null;

  // The first week on open; aborted on close. The parent keys this component by
  // register, so another register starts with a fresh state.
  const firstRange = useRef<[Date, Date]>([firstFrom, today]);
  useEffect(() => {
    void load(...firstRange.current);
    return abort;
  }, [load, abort]);

  const loadOlder = () => void load(subDays(from, OLDER_DAYS), subDays(from, 1));

  // The strip also knows the state at the start of the loaded range (a status can stay
  // the same for weeks); the list shows only the loaded changes.
  const stripChanges = useMemo(() => toChanges(before ? [before, ...entries] : entries), [before, entries]);
  const changes = useMemo(() => toChanges(entries), [entries]);
  const days = useMemo(() => {
    const byDay = new Map<string, StatusChange[]>();
    for (const c of [...changes].reverse()) {
      const key = format(c.at, DAY_KEY);
      byDay.set(key, [...(byDay.get(key) ?? []), c]);
    }
    return [...byDay.entries()];
  }, [changes]);

  // Opened on a day from the overview: bring that day's changes into view once loaded.
  const listRef = useRef<HTMLDivElement>(null);
  const scrolled = useRef(false);
  useEffect(() => {
    if (!initialDay || pending || scrolled.current) return;
    scrolled.current = true;
    listRef.current
      ?.querySelector(`[data-day="${format(initialDay, DAY_KEY)}"]`)
      ?.scrollIntoView({ block: 'start' });
  }, [initialDay, pending]);

  const canPrev = startOfDay(selected) > startOfDay(from);
  const canNext = !isSameDay(selected, today);

  // Two regions: the day strip stays put, only the change list scrolls (with solid,
  // full-width day headers that stick flush to its top edge).
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <section aria-label="Day overview" className="flex shrink-0 flex-col gap-2 border-b border-border p-4">
        <div className="flex items-center justify-between gap-2">
          <Button variant="ghost" size="icon" className="size-8" disabled={!canPrev} onClick={() => setSelected(d => subDays(d, 1))} aria-label="Previous day">
            <ChevronLeft className="size-4" aria-hidden="true" />
          </Button>
          <span className="text-sm font-medium">
            {isSameDay(selected, today) ? `Today, ${format(selected, 'd MMM yyyy')}` : format(selected, 'EEE, d MMM yyyy')}
          </span>
          <Button variant="ghost" size="icon" className="size-8" disabled={!canNext} onClick={() => setSelected(d => addDays(d, 1))} aria-label="Next day">
            <ChevronRight className="size-4" aria-hidden="true" />
          </Button>
        </div>
        {pending ? <Skeleton className="h-6 w-full" /> : <DayStrip changes={stripChanges} day={selected} now={now} />}
        {error && <p className="rounded bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
      </section>

      {pending ? (
        <div role="status" aria-label="Loading history" className="flex flex-col gap-2 p-4">
          {Array.from({ length: 5 }, (_, i) => <Skeleton key={i} className="h-7 w-full" />)}
        </div>
      ) : (
        <div ref={listRef} className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pb-4">
          {days.length === 0 && !error && (
            <p className="py-6 text-center text-sm text-muted-foreground">
              No changes since {format(from, 'd MMM yyyy')}.
            </p>
          )}
          {days.map(([key, dayChanges]) => {
            const day = dayChanges[0].at;
            return (
              <section key={key} data-day={key} aria-label={format(day, 'd MMM yyyy')}>
                <button
                  type="button"
                  onClick={() => setSelected(startOfDay(day))}
                  aria-pressed={isSameDay(day, selected)}
                  className={cn(
                    'sticky top-0 z-10 flex w-full items-baseline gap-2 border-b border-border bg-background px-4 py-1.5 text-left text-xs font-semibold text-muted-foreground hover:text-foreground',
                    isSameDay(day, selected) && 'text-foreground',
                  )}
                >
                  {isSameDay(day, today) ? 'Today' : format(day, 'EEE, d MMM yyyy')}
                  <span className="font-normal">
                    {dayChanges.length} {dayChanges.length === 1 ? 'change' : 'changes'}
                  </span>
                </button>
                <ul className="px-2 pt-1">
                  {dayChanges.map(c => <ChangeRow key={c.at.getTime()} change={c} now={now} />)}
                </ul>
              </section>
            );
          })}
          {before && (
            <p className="px-4 text-xs text-muted-foreground">
              Before {format(from, 'd MMM yyyy')}: <span className="text-foreground">{before.text}</span>, since {format(before.at, 'd MMM yyyy, HH:mm')}
            </p>
          )}
          {capped && (
            <p className="px-4 text-xs text-muted-foreground">
              Some ranges hit the limit of {STATUS_ROW_CAP} changes per request; older changes in them are not shown.
            </p>
          )}
          <Button variant="outline" size="sm" className="self-center gap-2" onClick={loadOlder} disabled={loading}>
            <History className="size-4" aria-hidden="true" />
            {loading ? 'Loading…' : `Load ${OLDER_DAYS} more days (before ${format(from, 'd MMM')})`}
          </Button>
        </div>
      )}
    </div>
  );
}
