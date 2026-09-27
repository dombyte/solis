import { useEffect, useState, useCallback } from 'react';

export function useVersionCheck() {
  const CURRENT = import.meta.env.VITE_GIT_COMMIT_HASH || 'dev';
  const [hasUpdate, setHasUpdate] = useState(false);

  const check = useCallback(async () => {
    try {
      const baseUrl = import.meta.env.BASE_URL || '/';
      const res = await fetch(`${baseUrl}data/version.json`, { cache: 'no-store' });
      if (!res.ok) return;
      const { version } = await res.json();
      setHasUpdate(version !== CURRENT && version !== 'dev');
    } catch {
      // Offline - handled by OfflineBanner
    }
  }, [CURRENT]);

  useEffect(() => {
    const intervalId = setInterval(check, 60_000);
    // Defer the initial check to a timer tick rather than calling it inline, so this
    // effect only ever sets up subscriptions instead of setting state synchronously.
    const initialCheckId = setTimeout(check, 0);
    return () => {
      clearInterval(intervalId);
      clearTimeout(initialCheckId);
    };
  }, [check]);

  const triggerUpdate = useCallback(() => window.location.reload(), []);
  const checkForUpdate = useCallback(() => check(), [check]);

  return { hasUpdate, triggerUpdate, checkForUpdate };
}
