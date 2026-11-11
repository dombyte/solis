import { useEffect, useState } from 'react';
import { addDays, isSameDay, startOfDay } from 'date-fns';

/**
 * The current local day. It changes at midnight and when a tab that was in the background
 * over midnight becomes visible again, so a page left open never works with yesterday.
 */
export function useToday(): Date {
  const [today, setToday] = useState(() => startOfDay(new Date()));

  useEffect(() => {
    const refresh = () => setToday(prev => (isSameDay(prev, new Date()) ? prev : startOfDay(new Date())));
    // One second past midnight: timers may fire slightly early.
    const untilMidnight = addDays(today, 1).getTime() - Date.now() + 1000;
    const timer = window.setTimeout(refresh, Math.max(untilMidnight, 1000));
    document.addEventListener('visibilitychange', refresh);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, [today]);

  return today;
}
