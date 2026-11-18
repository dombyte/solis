import { create } from 'zustand';

/**
 * Whether this browser can show notifications here:
 * - ok: supported in a secure context
 * - insecure: the page is served over plain HTTP (browsers require HTTPS or localhost)
 * - unsupported: no Notification API (e.g. iPhone Safari outside an installed web app)
 */
export type NotifySupport = 'ok' | 'insecure' | 'unsupported';

const STORAGE_KEY = 'solis.notify.v1';

function support(): NotifySupport {
  if (!window.isSecureContext) return 'insecure';
  return 'Notification' in window ? 'ok' : 'unsupported';
}

function permission(): NotificationPermission {
  return support() === 'ok' ? Notification.permission : 'denied';
}

// Per-browser opt-in; storage can throw (private mode), then the choice lasts the session.
function loadEnabled(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

function saveEnabled(enabled: boolean): void {
  try {
    if (enabled) window.localStorage.setItem(STORAGE_KEY, '1');
    else window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // not persisted
  }
}

interface NotifyState {
  support: NotifySupport;
  permission: NotificationPermission;
  /** The user opted in and the browser granted permission. */
  enabled: boolean;
  /** Asks for permission (from a click: browsers require a user gesture) and opts in. */
  enable: () => Promise<void>;
  disable: () => void;
}

/** Opt-in for fault notifications in this browser. */
export const useNotifyStore = create<NotifyState>(set => ({
  support: support(),
  permission: permission(),
  enabled: loadEnabled() && permission() === 'granted',
  enable: async () => {
    if (support() !== 'ok') return;
    const result = await Notification.requestPermission();
    const enabled = result === 'granted';
    saveEnabled(enabled);
    set({ permission: result, enabled });
  },
  disable: () => {
    saveEnabled(false);
    set({ enabled: false });
  },
}));
