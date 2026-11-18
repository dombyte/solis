import { useCallback, useEffect, useMemo, useState } from 'react';
import { historyDataGroups } from '../config/groups';
import {
  DEFAULT_PRESET, isPresetOf, isRangeKey, type HistoryRangeChoice,
} from '../utils/historyRange';
import type { Period } from '../../types';

/**
 * Per-browser History settings. Bump the version when the shape changes; an old or broken
 * entry falls back to the defaults field by field, never breaks the page.
 */
const STORAGE_KEY = 'solis.history.v1';

const PERIODS: Period[] = ['daily', 'monthly', 'yearly'];

interface HistoryPrefs {
  period: Period;
  /** Selected series per period, so switching period and back keeps each selection. */
  series: Record<Period, string[]>;
  range: Record<Period, HistoryRangeChoice>;
}

function perPeriod<T>(fn: (p: Period) => T): Record<Period, T> {
  return { daily: fn('daily'), monthly: fn('monthly'), yearly: fn('yearly') };
}

function defaultPrefs(): HistoryPrefs {
  return {
    period: 'daily',
    series: perPeriod(p => historyDataGroups[p].slice(0, 1)),
    range: perPeriod(p => ({ preset: DEFAULT_PRESET[p] })),
  };
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null;
}

/** Stored ids that still exist (registers get renamed); an emptied list gets the default. */
function sanitizeSeries(raw: unknown, period: Period, fallback: string[]): string[] {
  if (!Array.isArray(raw)) return fallback;
  const valid = historyDataGroups[period];
  const ids = [...new Set(raw.filter((id): id is string => valid.includes(id)))];
  return ids.length === 0 && raw.length > 0 ? fallback : ids;
}

function sanitizeRange(raw: unknown, period: Period, fallback: HistoryRangeChoice): HistoryRangeChoice {
  if (!isRecord(raw)) return fallback;
  if (isPresetOf(raw.preset, period)) return { preset: raw.preset };
  const c = raw.custom;
  if (isRecord(c) && isRangeKey(c.start, period) && isRangeKey(c.end, period) && c.start <= c.end) {
    return { custom: { start: c.start, end: c.end } };
  }
  return fallback;
}

function sanitize(raw: unknown): HistoryPrefs {
  const d = defaultPrefs();
  if (!isRecord(raw)) return d;
  const series = isRecord(raw.series) ? raw.series : {};
  const range = isRecord(raw.range) ? raw.range : {};
  return {
    period: PERIODS.includes(raw.period as Period) ? (raw.period as Period) : d.period,
    series: perPeriod(p => sanitizeSeries(series[p], p, d.series[p])),
    range: perPeriod(p => sanitizeRange(range[p], p, d.range[p])),
  };
}

// Storage can throw (private mode, blocked site data); the page then works unsaved.
function load(): HistoryPrefs {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    return raw ? sanitize(JSON.parse(raw)) : defaultPrefs();
  } catch {
    return defaultPrefs();
  }
}

function save(prefs: HistoryPrefs, isDefault: boolean): void {
  try {
    // Defaults are not stored, so a later change of a default reaches this browser too.
    if (isDefault) window.localStorage.removeItem(STORAGE_KEY);
    else window.localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    // not persisted; the in-memory settings still apply
  }
}

export interface UseHistoryPrefsResult {
  prefs: HistoryPrefs;
  /** True while nothing differs from the defaults (the reset button is then disabled). */
  isDefault: boolean;
  setPeriod: (period: Period) => void;
  /** Toggles a series of the current period. */
  toggleSeries: (id: string) => void;
  /** Sets the range of the current period. */
  setRange: (choice: HistoryRangeChoice) => void;
  reset: () => void;
}

/** The History page's period, series and range, remembered in this browser. */
export function useHistoryPrefs(): UseHistoryPrefsResult {
  const [prefs, setPrefs] = useState<HistoryPrefs>(load);
  const isDefault = useMemo(
    () => JSON.stringify(prefs) === JSON.stringify(defaultPrefs()),
    [prefs],
  );

  useEffect(() => save(prefs, isDefault), [prefs, isDefault]);

  const setPeriod = useCallback((period: Period) => setPrefs(p => ({ ...p, period })), []);

  const toggleSeries = useCallback((id: string) => setPrefs(p => {
    const cur = p.series[p.period];
    const next = cur.includes(id) ? cur.filter(x => x !== id) : [...cur, id];
    return { ...p, series: { ...p.series, [p.period]: next } };
  }), []);

  const setRange = useCallback((choice: HistoryRangeChoice) => setPrefs(p => ({
    ...p, range: { ...p.range, [p.period]: choice },
  })), []);

  const reset = useCallback(() => setPrefs(defaultPrefs()), []);

  return { prefs, isDefault, setPeriod, toggleSeries, setRange, reset };
}
