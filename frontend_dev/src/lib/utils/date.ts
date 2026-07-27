import { format, startOfDay, startOfMonth, startOfYear } from 'date-fns';
import type { Period } from '../../types';

/**
 * Get date range with proper formatting for each period
 */
export function getDateRangeForPeriod(period: Period): { start: string; end: string } {
  const end = new Date();
  const start = new Date();
  
  switch (period) {
    case 'daily':
      // Current month: start from first day of current month
      start.setDate(1);
      return {
        start: format(startOfDay(start), 'yyyy-MM-dd'),
        end: format(startOfDay(end), 'yyyy-MM-dd'),
      };
    case 'monthly':
      // Current year: start from January 1st of current year
      start.setMonth(0);
      start.setDate(1);
      return {
        start: format(startOfMonth(start), 'yyyy-MM'),
        end: format(startOfMonth(end), 'yyyy-MM'),
      };
    case 'yearly':
      // Last 5 years: start from January 1st, 5 years ago
      start.setFullYear(end.getFullYear() - 5);
      start.setMonth(0);
      start.setDate(1);
      return {
        start: format(startOfYear(start), 'yyyy'),
        end: format(startOfYear(end), 'yyyy'),
      };
    default:
      start.setDate(1);
      return {
        start: format(startOfDay(start), 'yyyy-MM-dd'),
        end: format(startOfDay(end), 'yyyy-MM-dd'),
      };
  }
}


