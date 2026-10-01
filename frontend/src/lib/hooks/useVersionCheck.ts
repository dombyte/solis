import { useEffect, useCallback } from 'react';
import { create } from 'zustand';
import { api } from '../api/client';

/** The version this frontend was built with (VITE_APP_VERSION, same as the binary's). */
export const APP_VERSION: string = import.meta.env.VITE_APP_VERSION || 'dev';
const POLL_MS = 60_000;

interface VersionState {
  hasUpdate: boolean;
  /** Fetches GET /api/version once and returns whether the server runs a newer build. */
  check: () => Promise<boolean>;
}

/**
 * One shared version state for every consumer (UpdateBanner, Info), so a manual check
 * and the banner always agree and only one poller runs. The frontend ships inside the
 * binary: a server version other than the one this page was built with means the
 * server was upgraded and a reload loads the new frontend. Unversioned ("dev") builds
 * on either side never report an update.
 */
const useVersionStore = create<VersionState>((set, get) => ({
  hasUpdate: false,
  check: async () => {
    try {
      const { version } = (await api.get('/api/version')) as { version?: unknown };
      const hasUpdate =
        typeof version === 'string' && version !== 'dev' && APP_VERSION !== 'dev' &&
        version !== APP_VERSION;
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
