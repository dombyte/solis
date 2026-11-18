import { useCallback, useRef, useState } from 'react';
import { format } from 'date-fns';
import { fetchChanges, type RangeChanges } from './useStatusHistory';

/** One status register to load. */
export interface OverviewSource {
  id: string;
  source: string;
}

export interface UseStatusOverviewResult {
  /** Changes per register id (missing when its request failed). */
  data: Map<string, RangeChanges>;
  /** "start/end" (yyyy-MM-dd) of the range `data` belongs to; null before the first load. */
  loadedKey: string | null;
  loading: boolean;
  /** Names the registers whose history failed to load. */
  error: string | null;
  /** Loads [start, end] for every register (one request each, in parallel). */
  load: (start: Date, end: Date) => Promise<void>;
  abort: () => void;
}

/** Identifies a loaded range ("2026-10-19/2026-11-17"). */
export function rangeKey(start: Date, end: Date): string {
  return `${format(start, 'yyyy-MM-dd')}/${format(end, 'yyyy-MM-dd')}`;
}

/** The change history of several status registers for the overview bars. */
export function useStatusOverview(sources: OverviewSource[]): UseStatusOverviewResult {
  const [data, setData] = useState<Map<string, RangeChanges>>(new Map());
  const [loadedKey, setLoadedKey] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async (start: Date, end: Date) => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    const results = await Promise.allSettled(
      sources.map(s => fetchChanges(s.id, s.source, start, end, controller.signal)),
    );
    if (controller.signal.aborted) return;
    const next = new Map<string, RangeChanges>();
    const failed: string[] = [];
    results.forEach((r, i) => {
      if (r.status === 'fulfilled') next.set(sources[i].id, r.value);
      else failed.push(sources[i].id);
    });
    setData(next);
    setLoadedKey(rangeKey(start, end));
    setError(failed.length > 0 ? `History unavailable for: ${failed.join(', ')}` : null);
    setLoading(false);
  }, [sources]);

  const abort = useCallback(() => abortRef.current?.abort(), []);

  return { data, loadedKey, loading, error, load, abort };
}
