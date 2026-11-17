import {
  endOfMonth, endOfYear, format, startOfMonth, startOfYear, subDays, subMonths, subYears,
} from 'date-fns';
import type { Period } from '../../types';

/** A history range as the keys the API takes (yyyy-MM-dd, yyyy-MM or yyyy, inclusive). */
export interface HistoryRange {
  start: string;
  end: string;
}

export type HistoryPreset =
  | 'last_7d' | 'last_30d' | 'this_month' | 'last_month'
  | 'this_year' | 'last_12m' | 'last_year' | 'last_3y'
  | 'last_5y' | 'last_10y';

/**
 * What the user picked: a preset (resolved against today on every load, so "last 30 days"
 * moves with the calendar) or a fixed range from the calendar.
 */
export type HistoryRangeChoice = { preset: HistoryPreset } | { custom: HistoryRange };

/** The presets that make sense for each period, in display order. */
export const HISTORY_PRESETS: Record<Period, { id: HistoryPreset; label: string }[]> = {
  daily: [
    { id: 'last_7d', label: 'Last 7 days' },
    { id: 'last_30d', label: 'Last 30 days' },
    { id: 'this_month', label: 'This month' },
    { id: 'last_month', label: 'Last month' },
    { id: 'this_year', label: 'This year' },
  ],
  monthly: [
    { id: 'last_12m', label: 'Last 12 months' },
    { id: 'this_year', label: 'This year' },
    { id: 'last_year', label: 'Last year' },
    { id: 'last_3y', label: 'Last 3 years' },
  ],
  yearly: [
    { id: 'last_5y', label: 'Last 5 years' },
    { id: 'last_10y', label: 'Last 10 years' },
  ],
};

/** The range each period opens with: this month by day, this year by month, five years. */
export const DEFAULT_PRESET: Record<Period, HistoryPreset> = {
  daily: 'this_month',
  monthly: 'this_year',
  yearly: 'last_5y',
};

const KEY_FORMAT: Record<Period, string> = {
  daily: 'yyyy-MM-dd',
  monthly: 'yyyy-MM',
  yearly: 'yyyy',
};

const KEY_PATTERN: Record<Period, RegExp> = {
  daily: /^\d{4}-\d{2}-\d{2}$/,
  monthly: /^\d{4}-\d{2}$/,
  yearly: /^\d{4}$/,
};

/** Whether key is a range key of period's format (the API rejects any other). */
export function isRangeKey(key: unknown, period: Period): key is string {
  return typeof key === 'string' && KEY_PATTERN[period].test(key);
}

/** Whether preset is offered for period. */
export function isPresetOf(preset: unknown, period: Period): preset is HistoryPreset {
  return HISTORY_PRESETS[period].some(p => p.id === preset);
}

/** First and last day of a preset; ranges that include today end today. */
function presetDays(preset: HistoryPreset, today: Date): [Date, Date] {
  switch (preset) {
    case 'last_7d': return [subDays(today, 6), today];
    case 'last_30d': return [subDays(today, 29), today];
    case 'this_month': return [startOfMonth(today), today];
    case 'last_month': {
      const m = subMonths(today, 1);
      return [startOfMonth(m), endOfMonth(m)];
    }
    case 'this_year': return [startOfYear(today), today];
    case 'last_12m': return [startOfMonth(subMonths(today, 11)), today];
    case 'last_year': {
      const y = subYears(today, 1);
      return [startOfYear(y), endOfYear(y)];
    }
    case 'last_3y': return [startOfYear(subYears(today, 2)), today];
    case 'last_5y': return [startOfYear(subYears(today, 4)), today];
    case 'last_10y': return [startOfYear(subYears(today, 9)), today];
  }
}

/** The API range of a choice for period, presets resolved against today. */
export function resolveRange(choice: HistoryRangeChoice, period: Period, today: Date): HistoryRange {
  if ('custom' in choice) return choice.custom;
  const [from, to] = presetDays(choice.preset, today);
  return { start: format(from, KEY_FORMAT[period]), end: format(to, KEY_FORMAT[period]) };
}

/** A calendar selection as range keys of period's format (local dates, never UTC). */
export function rangeFromDates(from: Date, to: Date, period: Period): HistoryRange {
  return { start: format(from, KEY_FORMAT[period]), end: format(to, KEY_FORMAT[period]) };
}
