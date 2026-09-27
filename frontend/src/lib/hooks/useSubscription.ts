import { useEffect } from 'react';
import { websocketClient } from '../api/websocket';

/**
 * Declares this component's interest in a set of register keys. The underlying client
 * ref-counts keys across all callers, so overlapping subscriptions from multiple
 * components are safe, and re-subscribes the active set on reconnect.
 */
export function useSubscription(keys: string[]): void {
  const keysKey = keys.join(',');

  useEffect(() => {
    if (!keysKey) return;
    const list = keysKey.split(',');
    const unsubscribe = websocketClient.subscribe(list);
    return unsubscribe;
  }, [keysKey]);
}
