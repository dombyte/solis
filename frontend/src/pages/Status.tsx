import React, { useEffect, useMemo, useState } from 'react';
import { format, isSameDay, isSameYear, subDays } from 'date-fns';
import { ChevronLeft, ChevronRight, X } from 'lucide-react';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { STATUS_IDS, useStatusFaults } from '../lib/hooks/useStatusFaults';
import { rangeKey, useStatusOverview, type OverviewSource } from '../lib/hooks/useStatusOverview';
import { useNow } from '../lib/hooks/useNow';
import { useToday } from '../lib/hooks/useToday';
import { useCompactLayout, useMobile } from '../hooks/useMobile';
import { Page } from '../components/layout/Page';
import { Button } from '../components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from '../components/ui/dialog';
import { SURFACE } from '../components/ui/surface';
import { NotifyToggle } from '../components/status/NotifyToggle';
import { StatusRow, type StatusRowData } from '../components/status/StatusRow';
import { StatusTimeline } from '../components/status/StatusTimeline';
import { LEVEL_BG, LEVEL_TEXT } from '../components/status/levelStyles';
import { apiDataObjects } from '../lib/config/data';
import { statusLevel, statusLevelOf, statusText } from '../lib/utils/status';
import { summarizeDays, toChanges, type StatusChangeInput } from '../lib/utils/statusDays';
import { cn } from '../lib/utils';
import type { RegisterValue } from '../types';

const AGO_REFRESH_MS = 5_000;
/** Window sizes: both on desktop; compact layouts (phones, narrow windows) only 30. */
const RANGES = [30, 90] as const;
const COMPACT_RANGE = 30;
type Range = (typeof RANGES)[number];
const RANGE_KEY = 'solis.status.range';

const SOURCES: OverviewSource[] = STATUS_IDS.flatMap(id => {
  const obj = apiDataObjects.find(o => o.id === id);
  return obj ? [{ id, source: obj.source }] : [];
});

// Per-browser convenience; storage can throw (private mode).
function loadRange(fallback: Range): Range {
  try {
    const v = Number(window.localStorage.getItem(RANGE_KEY));
    return RANGES.includes(v as Range) ? (v as Range) : fallback;
  } catch {
    return fallback;
  }
}

function saveRange(range: Range): void {
  try {
    window.localStorage.setItem(RANGE_KEY, String(range));
  } catch {
    // not persisted
  }
}

/** The live value as a change, so today's bar follows the current state without a reload. */
function liveChange(id: string, value: RegisterValue | undefined): StatusChangeInput | null {
  if (!value?.timestamp || value.statusDecoded === undefined) return null;
  return { at: new Date(value.timestamp), text: statusText(value.statusDecoded, value.value), level: statusLevelOf(id, value.statusDecoded) };
}

function RangeToggle({ range, onChange }: { range: Range; onChange: (r: Range) => void }): React.ReactElement {
  return (
    <div role="radiogroup" aria-label="Days shown" className="inline-flex rounded-md border border-border bg-control p-0.5">
      {RANGES.map(r => (
        <button
          key={r}
          type="button"
          role="radio"
          aria-checked={range === r}
          onClick={() => onChange(r)}
          className={cn(
            'rounded px-2.5 py-1 text-xs font-medium transition-colors',
            range === r ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-control-hover hover:text-foreground',
          )}
        >
          {r} days
        </button>
      ))}
    </div>
  );
}

/** "19 Oct – 17 Nov 2026"; the year once when both days share it. */
function windowLabel(start: Date, end: Date): string {
  return isSameYear(start, end)
    ? `${format(start, 'd MMM')} – ${format(end, 'd MMM yyyy')}`
    : `${format(start, 'd MMM yyyy')} – ${format(end, 'd MMM yyyy')}`;
}

interface PagerProps {
  label: string;
  className?: string;
  size: number;
  canNext: boolean;
  onPrev: () => void;
  onNext: () => void;
}

/** Steps the bars back and forth by one window. */
function Pager({ label, size, canNext, onPrev, onNext, className }: PagerProps): React.ReactElement {
  return (
    <div className={cn('flex items-center justify-between gap-1', className)}>
      <Button variant="ghost" size="icon" className="size-8" onClick={onPrev} aria-label={`Previous ${size} days`}>
        <ChevronLeft className="size-4" aria-hidden="true" />
      </Button>
      <span className="min-w-36 flex-1 text-center text-xs font-medium tabular-nums" aria-live="polite">{label}</span>
      <Button variant="ghost" size="icon" className="size-8" disabled={!canNext} onClick={onNext} aria-label={`Next ${size} days`}>
        <ChevronRight className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}

function Legend(): React.ReactElement {
  const items = [
    ['ok', 'No issues'],
    ['notice', 'Not producing / limited'],
    ['fault', 'Fault'],
    ['unknown', 'No data'],
  ] as const;
  return (
    <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
      {items.map(([level, label]) => (
        <span key={level} className="flex items-center gap-1.5">
          <span className={cn('size-2.5 rounded-sm', LEVEL_BG[level])} aria-hidden="true" />
          {label}
        </span>
      ))}
    </div>
  );
}

export function Status(): React.ReactElement {
  // Subscribes to every status key (shared with the nav badge and the notifier).
  useStatusFaults();
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const registerValues = useRegisterStore(state => state.registerValues);
  const now = useNow(AGO_REFRESH_MS);
  const today = useToday();
  const compact = useCompactLayout();
  // Touch: bars are display only (too narrow to hit one day); the row opens the log.
  const touch = useMobile();
  const [storedRange, setStoredRange] = useState<Range>(() => loadRange(90));
  const range: Range = compact ? COMPACT_RANGE : storedRange;
  // Windows back from today (0 = the window ending today).
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<{ id: string; day?: Date } | null>(null);
  const { data, loadedKey, loading, error, load, abort } = useStatusOverview(SOURCES);

  const end = useMemo(() => subDays(today, page * range), [today, page, range]);
  const start = useMemo(() => subDays(end, range - 1), [end, range]);
  const days = useMemo(
    () => Array.from({ length: range }, (_, i) => subDays(end, range - 1 - i)),
    [range, end],
  );
  const showsToday = isSameDay(end, today);

  // One request per register for the shown window (again after midnight).
  useEffect(() => {
    void load(start, end);
    return abort;
  }, [load, abort, start, end]);
  const current = loadedKey === rangeKey(start, end);

  const rows: StatusRowData[] = useMemo(() => STATUS_IDS.flatMap(id => {
    const reg = registerMetadata.get(id);
    if (!reg) return [];
    const value = registerValues.get(id);
    const history = current ? data.get(id) : undefined;
    let dayData = null;
    if (history) {
      const entries: StatusChangeInput[] = history.before ? [history.before, ...history.entries] : [...history.entries];
      // The live value extends today only (it is newer than any stored change).
      const live = showsToday ? liveChange(id, value) : null;
      const last = entries[entries.length - 1];
      if (live && (!last || (live.at > last.at && live.text !== last.text))) entries.push(live);
      dayData = summarizeDays(toChanges(entries), days, now);
    }
    return [{
      id,
      name: reg.name,
      description: reg.description,
      text: statusText(value?.statusDecoded, value?.value),
      level: statusLevel(reg, value),
      received: value?.timestamp ? new Date(value.timestamp) : null,
      days: !current || loading ? null : dayData ?? summarizeDays([], days, now),
    }];
  }), [registerMetadata, registerValues, data, days, now, loading, current, showsToday]);

  const changeRange = (r: Range) => {
    setStoredRange(r);
    saveRange(r);
    setPage(0);
  };
  const fromLabel = format(start, 'd MMM');
  const toLabel = showsToday ? 'Today' : format(end, 'd MMM');

  const selectedRow = rows.find(r => r.id === selected?.id);
  const selectedSource = SOURCES.find(s => s.id === selected?.id)?.source;

  return (
    <Page title="Status" actions={<NotifyToggle />}>
      <section aria-labelledby="status-list-title" className={cn(SURFACE, 'overflow-visible')}>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
          <div className="flex flex-col gap-1">
            <h2 id="status-list-title" className="text-base font-semibold">Inverter status</h2>
            <Legend />
          </div>
          {/* Compact layout: the pager spans the card (arrows at the edges, easy to hit). */}
          <div className={cn('flex flex-wrap items-center gap-2', compact && 'w-full')}>
            <Pager
              className={compact ? 'w-full rounded-md bg-control' : undefined}
              label={windowLabel(start, end)}
              size={range}
              canNext={page > 0}
              onPrev={() => setPage(p => p + 1)}
              onNext={() => setPage(p => Math.max(0, p - 1))}
            />
            {!compact && <RangeToggle range={range} onChange={changeRange} />}
          </div>
        </div>
        {error && <p className="border-b border-border px-4 py-2 text-sm text-destructive sm:px-5">{error}</p>}
        <ul className="divide-y divide-border">
          {rows.map(row => (
            <StatusRow
              key={row.id}
              row={row}
              now={now}
              today={today}
              interactive={!touch}
              fromLabel={fromLabel}
              toLabel={toLabel}
              onOpen={day => setSelected({ id: row.id, day })}
            />
          ))}
        </ul>
      </section>

      <Dialog open={selectedRow !== undefined} onOpenChange={open => { if (!open) setSelected(null); }}>
        <DialogContent className="max-w-3xl w-[calc(100vw-2rem)] max-h-[85dvh] overflow-hidden p-0 flex flex-col">
          {selectedRow && selectedSource && (
            <>
              <div className="flex items-start justify-between gap-3 p-4 border-b">
                <div className="min-w-0">
                  <DialogTitle>{selectedRow.name}</DialogTitle>
                  {selectedRow.description && (
                    <p className="mt-1 text-sm text-muted-foreground">{selectedRow.description}</p>
                  )}
                  <p className={cn('mt-1 flex items-center gap-2 text-sm font-medium', LEVEL_TEXT[selectedRow.level])}>
                    <span className={cn('size-2 rounded-full', LEVEL_BG[selectedRow.level])} aria-hidden="true" />
                    Now: {selectedRow.text}
                  </p>
                </div>
                <DialogClose asChild>
                  <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0">
                    <X className="h-4 w-4" />
                    <span className="sr-only">Close</span>
                  </Button>
                </DialogClose>
              </div>
              <DialogDescription>Change history of {selectedRow.name}</DialogDescription>
              <div className="flex min-h-0 flex-1 flex-col">
                <StatusTimeline
                  key={`${selectedRow.id}-${selected?.day?.getTime() ?? 'today'}`}
                  id={selectedRow.id}
                  source={selectedSource}
                  initialDay={selected?.day}
                />
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </Page>
  );
}
