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
    const id = setInterval(check, 60_000);
    check();
    return () => clearInterval(id);
  }, [check]);

  const triggerUpdate = useCallback(() => window.location.reload(), []);
  const checkForUpdate = useCallback(() => check(), [check]);

  return { hasUpdate, triggerUpdate, checkForUpdate };
}
