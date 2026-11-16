import { format, startOfMonth, startOfYear, subYears } from 'date-fns';
import type { Period } from '../../types';

/** Years the yearly history shows by default, the current one included. */
const DEFAULT_YEARS = 5;

/**
 * The default history range of a period, as the keys the API takes: the current month by
 * day, the current year by month, the last five years (this one included) by year.
 */
export function getDateRangeForPeriod(period: Period): { start: string; end: string } {
  const today = new Date();
  switch (period) {
    case 'monthly':
      return { start: format(startOfYear(today), 'yyyy-MM'), end: format(today, 'yyyy-MM') };
    case 'yearly':
      return { start: format(subYears(today, DEFAULT_YEARS - 1), 'yyyy'), end: format(today, 'yyyy') };
    default:
      return { start: format(startOfMonth(today), 'yyyy-MM-dd'), end: format(today, 'yyyy-MM-dd') };
  }
}
