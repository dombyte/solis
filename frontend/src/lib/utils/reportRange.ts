import {
  addDays, differenceInCalendarDays, endOfMonth, endOfYear, format, isAfter, parseISO,
  startOfMonth, startOfYear, subDays, subMonths, subYears,
} from 'date-fns';
import type { CompareMode } from '../api/report';

export type ReportPreset = 'this_month' | 'last_month' | 'this_year' | 'last_year' | 'custom';

export const REPORT_PRESETS: { id: Exclude<ReportPreset, 'custom'>; label: string }[] = [
  { id: 'this_month', label: 'This month' },
  { id: 'last_month', label: 'Last month' },
  { id: 'this_year', label: 'This year' },
  { id: 'last_year', label: 'Last year' },
];

export const PRESET_LABEL: Record<ReportPreset, string> = {
  this_month: 'This month', last_month: 'Last month', this_year: 'This year',
  last_year: 'Last year', custom: 'Custom range',
};

/** Same limit as the server (report.MaxRangeDays). */
const MAX_REPORT_DAYS = 3660;

export interface DayRange {
  from: string; // YYYY-MM-DD
  to: string;   // YYYY-MM-DD, inclusive
}

const day = (d: Date): string => format(d, 'yyyy-MM-dd');

/** The day range of a preset; ranges that include today end today (local time). */
export function presetRange(preset: Exclude<ReportPreset, 'custom'>, today: Date): DayRange {
  switch (preset) {
    case 'this_month':
      return { from: day(startOfMonth(today)), to: day(today) };
    case 'last_month': {
      const m = subMonths(today, 1);
      return { from: day(startOfMonth(m)), to: day(endOfMonth(m)) };
    }
    case 'this_year':
      return { from: day(startOfYear(today)), to: day(today) };
    case 'last_year': {
      const y = subYears(today, 1);
      return { from: day(startOfYear(y)), to: day(endOfYear(y)) };
    }
  }
}

/**
 * r without today, like the server reports it (report.Range.Finished): today is still in
 * progress, so a range ending today is evaluated up to yesterday.
 */
export function finishedRange(r: DayRange, today: Date): DayRange {
  if (r.to !== day(today) || r.from === r.to) return r;
  return { from: r.from, to: day(subDays(today, 1)) };
}

/** Number of calendar days in r. */
export function rangeDays(r: DayRange): number {
  return differenceInCalendarDays(parseISO(r.to), parseISO(r.from)) + 1;
}

/** A user-facing problem with r, or null when the server will accept it. */
export function rangeProblem(r: DayRange, today: Date): string | null {
  if (r.from > r.to) return 'The start date is after the end date.';
  if (isAfter(parseISO(r.to), today)) return 'The end date is in the future.';
  if (r.from === day(today)) return 'Today is still in progress; a report covers finished days only.';
  if (rangeDays(r) > MAX_REPORT_DAYS) return 'A report covers at most ten years.';
  return null;
}

/** Granularity the server picks automatically (report.AutoGranularity). */
export function autoGranularity(r: DayRange): 'day' | 'month' | 'year' {
  const n = rangeDays(r);
  if (n <= 62) return 'day';
  if (n <= 731) return 'month';
  return 'year';
}

/** "1 – 31 Oct 2026", "1 Sep – 31 Oct 2026", "1 Nov 2025 – 10 Jan 2026"; one day: "29 Oct 2026". */
export function formatRange(r: DayRange): string {
  const from = parseISO(r.from);
  const to = parseISO(r.to);
  if (r.from === r.to) return format(to, 'd MMM yyyy');
  if (from.getFullYear() !== to.getFullYear()) return `${format(from, 'd MMM yyyy')} – ${format(to, 'd MMM yyyy')}`;
  if (from.getMonth() !== to.getMonth()) return `${format(from, 'd MMM')} – ${format(to, 'd MMM yyyy')}`;
  return `${format(from, 'd')} – ${format(to, 'd MMM yyyy')}`;
}

/**
 * The comparison range of r, or null for 'none'. Mirrors report.CompareMode.Of on the
 * server (internal/report/range.go); the preview shows the server's range, this one only
 * labels the options before they are chosen.
 */
export function compareRange(mode: CompareMode, r: DayRange): DayRange | null {
  const from = parseISO(r.from);
  const to = parseISO(r.to);
  if (mode === 'none') return null;
  if (mode === 'last_year') return { from: day(subYears(from, 1)), to: day(subYears(to, 1)) };
  const span = (to.getFullYear() - from.getFullYear()) * 12 + to.getMonth() - from.getMonth() + 1;
  const wholeMonths = from.getDate() === 1 && addDays(to, 1).getDate() === 1;
  if (wholeMonths) return { from: day(subMonths(from, span)), to: day(subDays(from, 1)) };
  if (from.getDate() === 1) {
    // Period to date: the same days of the period before (subMonths clamps 31 Mar to 28 Feb).
    const n = from.getMonth() === 0 && from.getFullYear() === to.getFullYear() && span > 1 ? 12 : span;
    return { from: day(subMonths(from, n)), to: day(subMonths(to, n)) };
  }
  const days = rangeDays(r);
  return { from: day(subDays(from, days)), to: day(subDays(from, 1)) };
}

/** The preset whose range equals r, else 'custom'. */
export function presetOf(r: DayRange, today: Date): ReportPreset {
  return REPORT_PRESETS.find(p => {
    const pr = presetRange(p.id, today);
    return pr.from === r.from && pr.to === r.to;
  })?.id ?? 'custom';
}

/** Short name of a period: "Oct 2026" for a whole month, "2026" for a whole year. */
export function periodName(r: DayRange): string {
  const from = parseISO(r.from);
  const to = parseISO(r.to);
  const wholeMonth = day(startOfMonth(from)) === r.from && day(endOfMonth(to)) === r.to;
  if (wholeMonth && from.getFullYear() === to.getFullYear()) {
    if (from.getMonth() === 0 && to.getMonth() === 11) return String(from.getFullYear());
    if (from.getMonth() === to.getMonth()) return format(from, 'MMM yyyy');
  }
  return formatRange(r);
}
