import { useEffect, useMemo } from 'react';
import { websocketClient } from '../api/websocket';

/**
 * Declares this component's interest in a set of register keys. The underlying client
 * ref-counts keys across all callers, so overlapping subscriptions from multiple
 * components are safe, and re-subscribes the active set on reconnect.
 */
export function useSubscription(keys: string[]): void {
  // JSON-encode rather than comma-join so a register key could safely contain a comma,
  // and stringify inside useMemo so effect deps stay stable across re-renders with the
  // same keys but a new array reference.
  const keysKey = useMemo(() => JSON.stringify(keys), [keys]);

  useEffect(() => {
    const list = JSON.parse(keysKey) as string[];
    if (list.length === 0) return;
    const unsubscribe = websocketClient.subscribe(list);
    return unsubscribe;
  }, [keysKey]);
}
