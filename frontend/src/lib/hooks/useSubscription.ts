import { useEffect, useMemo } from 'react';
import { websocketClient } from '../api/websocket';

/**
 * Declares this component's interest in a set of register keys. The underlying client
 * ref-counts keys across all callers, so overlapping subscriptions from multiple
 * components are safe, and re-subscribes the active set on reconnect.
 */
export function useSubscription(keys: string[]): void {
  // JSON-encode rather than comma-join so a register key could safely contain a comma.
  // Callers usually pass a new array every render, so the memo recomputes each time; the
  // effect still only re-runs when the resulting string (the key set) changes.
  const keysKey = useMemo(() => JSON.stringify(keys), [keys]);

  useEffect(() => {
    const list = JSON.parse(keysKey) as string[];
    if (list.length === 0) return;
    const unsubscribe = websocketClient.subscribe(list);
    return unsubscribe;
  }, [keysKey]);
}
