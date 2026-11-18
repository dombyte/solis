import { useEffect, useState } from 'react';

/** The current time, refreshed every `intervalMs` (for relative times like "12 s ago"). */
export function useNow(intervalMs: number): Date {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);

  return now;
}
