import { useEffect, useCallback } from 'react';
import { create } from 'zustand';
import { api } from '../api/client';

/** The version this frontend was built with (VITE_APP_VERSION, same as the binary's). */
export const APP_VERSION: string = import.meta.env.VITE_APP_VERSION || 'dev';
const POLL_MS = 60_000;
const DISMISSED_KEY = 'solis-update-dismissed';

// Storage can be unavailable (private mode, blocked site data): dismissal then lasts
// only for this page load.
function readDismissed(): string | null {
  try {
    return window.localStorage.getItem(DISMISSED_KEY);
  } catch {
    return null;
  }
}

function writeDismissed(version: string): void {
  try {
    window.localStorage.setItem(DISMISSED_KEY, version);
  } catch {
    // ignore, see readDismissed
  }
}

interface VersionState {
  hasUpdate: boolean;
  /** The version the server reported last (null until the first successful check). */
  serverVersion: string | null;
  /** The server version whose update banner the user closed. */
  dismissedVersion: string | null;
  dismiss: () => void;
  /** Fetches GET /api/version once and returns whether the server runs a newer build. */
  check: () => Promise<boolean>;
}

/**
 * One shared version state for every consumer (UpdateBanner, Info), so a manual check
 * and the banner always agree and only one poller runs. The frontend ships inside the
 * binary: a server version other than the one this page was built with means the
 * server was upgraded and a reload loads the new frontend. Unversioned ("dev") builds
 * on either side never report an update. Closing the banner hides it for that server
 * version only; a later release shows it again, and Info always offers the update.
 */
const useVersionStore = create<VersionState>((set, get) => ({
  hasUpdate: false,
  serverVersion: null,
  dismissedVersion: readDismissed(),
  dismiss: () => {
    const { serverVersion } = get();
    if (serverVersion === null) return;
    writeDismissed(serverVersion);
    set({ dismissedVersion: serverVersion });
  },
  check: async () => {
    try {
      const { version } = (await api.get('/api/version')) as { version?: unknown };
      const hasUpdate =
        typeof version === 'string' && version !== 'dev' && APP_VERSION !== 'dev' &&
        version !== APP_VERSION;
      set({ hasUpdate, serverVersion: typeof version === 'string' ? version : null });
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
  const dismissed = useVersionStore(
    s => s.serverVersion !== null && s.serverVersion === s.dismissedVersion,
  );
  const dismissUpdate = useVersionStore(s => s.dismiss);

  useEffect(() => acquirePoller(), []);

  const triggerUpdate = useCallback(() => window.location.reload(), []);

  return { hasUpdate, dismissed, triggerUpdate, dismissUpdate, checkForUpdate: check };
}
