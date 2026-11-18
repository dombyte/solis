import { useCallback, useRef, useState } from 'react';
import { format } from 'date-fns';
import { api } from '../api/client';
import { statusLevelOf, statusText, type StatusLevel } from '../utils/status';

/** The server returns at most this many changes per request (the newest). */
export const STATUS_ROW_CAP = 1000;
const DAY_KEY = 'yyyy-MM-dd';

/** One stored change of a status register. */
interface StatusEntry {
  at: Date;
  text: string;
  level: StatusLevel;
}

interface WireEntry {
  timestamp: string;
  status_decoded: unknown;
}

interface HistoryResponse {
  history?: WireEntry[];
  before?: WireEntry;
}

export interface RangeChanges {
  /** Oldest first. */
  entries: StatusEntry[];
  /** The change active at the range start, if one is stored. */
  before: StatusEntry | null;
}

/** The changes of [start, end] (local days, both inclusive) and the state at start. */
export async function fetchChanges(id: string, source: string, start: Date, end: Date, signal: AbortSignal): Promise<RangeChanges> {
  const body = await api.get(source, { start: format(start, DAY_KEY), end: format(end, DAY_KEY) }, { signal });
  const res = body as HistoryResponse | null;
  if (!Array.isArray(res?.history)) throw new Error('Unexpected status history response');
  const toEntry = (h: WireEntry): StatusEntry => ({
    at: new Date(h.timestamp), text: statusText(h.status_decoded), level: statusLevelOf(id, h.status_decoded),
  });
  return { entries: res.history.map(toEntry).reverse(), before: res.before ? toEntry(res.before) : null };
}

export interface UseStatusHistoryResult {
  /** Loaded changes, oldest first. */
  entries: StatusEntry[];
  /** The change active at the start of the oldest loaded day (stored before it). */
  before: StatusEntry | null;
  /** The oldest loaded day (null before the first load finished). */
  from: Date | null;
  loading: boolean;
  error: string | null;
  /** A request returned the row cap: older changes of that range are missing. */
  capped: boolean;
  /**
   * Loads the days [start, end] and puts them in front of what is loaded, so ranges must
   * be requested from the newest backwards. A newer call aborts a pending one.
   */
  load: (start: Date, end: Date) => Promise<void>;
  /** Aborts a pending load (on unmount). */
  abort: () => void;
}

/** The change history of one status register, loaded by day ranges (start/end). */
export function useStatusHistory(id: string, source: string): UseStatusHistoryResult {
  const [entries, setEntries] = useState<StatusEntry[]>([]);
  const [before, setBefore] = useState<StatusEntry | null>(null);
  const [from, setFrom] = useState<Date | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [capped, setCapped] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async (start: Date, end: Date) => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setError(null);
    try {
      const older = await fetchChanges(id, source, start, end, controller.signal);
      if (controller.signal.aborted) return;
      if (older.entries.length >= STATUS_ROW_CAP) setCapped(true);
      setEntries(prev => [...older.entries, ...prev]);
      setBefore(older.before);
      setFrom(start);
    } catch (err) {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Failed to load history');
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [id, source]);

  const abort = useCallback(() => abortRef.current?.abort(), []);

  return { entries, before, from, loading, error, capped, load, abort };
}
