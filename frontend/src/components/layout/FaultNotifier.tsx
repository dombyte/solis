import { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useStatusFaults, type StatusFault } from '../../lib/hooks/useStatusFaults';
import { useNotifyStore } from '../../lib/stores/useNotifyStore';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';

const ICON = '/pwa-192x192.png';
const STATUS_PATH = '/status';

/**
 * Shows the fault through the service worker (required on Android, where
 * `new Notification` throws) and falls back to the page API. Clicking it opens the
 * Status page (sw.js handles clicks on worker notifications).
 */
async function show(fault: StatusFault, openStatus: () => void): Promise<void> {
  const title = `Inverter fault: ${fault.name}`;
  const options: NotificationOptions = {
    body: fault.text, tag: `solis-${fault.id}`, icon: ICON, data: { url: STATUS_PATH },
  };
  try {
    const reg = await navigator.serviceWorker?.getRegistration();
    if (reg) {
      await reg.showNotification(title, options);
      return;
    }
  } catch {
    // no worker (or it failed): try the page API
  }
  try {
    const n = new Notification(title, options);
    n.onclick = () => {
      window.focus();
      openStatus();
      n.close();
    };
  } catch {
    // not supported here; the banner and the nav badge still show the fault
  }
}

/** The user is not looking at the app (background tab, minimized, another window focused). */
function away(): boolean {
  return document.visibilityState === 'hidden' || !document.hasFocus();
}

/**
 * Notifies about new faults while the app is open but not in front (opt-in on the
 * Status page). Faults already present when the app connects are not notified (the
 * banner shows them), and neither is one that only came back after a reconnect: a
 * register's fault is forgotten only once it reports a non-fault value, never because
 * its value went missing.
 */
export function FaultNotifier(): null {
  const { faults, present } = useStatusFaults();
  const enabled = useNotifyStore(state => state.enabled);
  const ready = useRegisterStore(state => state.isConnected && !state.isLoading);
  const navigate = useNavigate();
  // id -> fault text already seen (notified or present at start)
  const seen = useRef(new Map<string, string>());
  const baselined = useRef(false);

  useEffect(() => {
    if (!ready || present.length === 0) return;
    const faulty = new Set(faults.map(f => f.id));
    for (const id of present) {
      if (!faulty.has(id)) seen.current.delete(id);
    }
    for (const f of faults) {
      if (seen.current.get(f.id) === f.text) continue;
      seen.current.set(f.id, f.text);
      if (baselined.current && enabled && away()) void show(f, () => navigate(STATUS_PATH));
    }
    baselined.current = true;
  }, [faults, present, ready, enabled, navigate]);

  return null;
}
