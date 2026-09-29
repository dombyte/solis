import { useEffect, useCallback } from 'react';
import { create } from 'zustand';

const CURRENT = import.meta.env.VITE_GIT_COMMIT_HASH || 'dev';
const POLL_MS = 60_000;

interface VersionState {
  hasUpdate: boolean;
  /** Fetches version.json once and returns whether a newer build is deployed. */
  check: () => Promise<boolean>;
}

/**
 * One shared version state for every consumer (UpdateBanner, Info), so a manual check
 * and the banner always agree and only one poller runs.
 */
const useVersionStore = create<VersionState>((set, get) => ({
  hasUpdate: false,
  check: async () => {
    try {
      const baseUrl = import.meta.env.BASE_URL || '/';
      const res = await fetch(`${baseUrl}data/version.json`, { cache: 'no-store' });
      if (!res.ok) return get().hasUpdate;
      const { version } = (await res.json()) as { version?: unknown };
      const hasUpdate = typeof version === 'string' && version !== CURRENT && version !== 'dev';
      set({ hasUpdate });
      return hasUpdate;
    } catch {
      return get().hasUpdate; // offline - handled by OfflineBanner
    }
  },
}));

// Ref-counted poller: the first mounted consumer starts it, the last one stops it.
let consumers = 0;
let pollId: ReturnType<typeof setInterval> | null = null;

function acquirePoller(): () => void {
  consumers++;
  if (consumers === 1) {
    const { check } = useVersionStore.getState();
    pollId = setInterval(check, POLL_MS);
    setTimeout(check, 0); // initial check on the next tick, not during render/effect
  }
  return () => {
    consumers--;
    if (consumers === 0 && pollId !== null) {
      clearInterval(pollId);
      pollId = null;
    }
  };
}

export function useVersionCheck() {
  const hasUpdate = useVersionStore(s => s.hasUpdate);
  const check = useVersionStore(s => s.check);

  useEffect(() => acquirePoller(), []);

  const triggerUpdate = useCallback(() => window.location.reload(), []);

  return { hasUpdate, triggerUpdate, checkForUpdate: check };
}
