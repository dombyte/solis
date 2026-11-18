import React from 'react';
import { Bell, BellOff } from 'lucide-react';
import { useNotifyStore } from '../../lib/stores/useNotifyStore';
import { Button } from '../ui/button';
import { InfoPopover } from '../ui/info-popover';

/** Opt-in for fault notifications in this browser, with the reason when it cannot work here. */
export function NotifyToggle(): React.ReactElement {
  const { support, permission, enabled, enable, disable } = useNotifyStore();
  const blocked = support !== 'ok' || permission === 'denied';
  const reason = support === 'insecure'
    ? 'Browsers only allow notifications on HTTPS pages (or localhost). Serve the monitor through an HTTPS reverse proxy to use them.'
    : support === 'unsupported'
      ? 'This browser does not offer notifications here. On an iPhone, add the app to the Home Screen and open it from there.'
      : 'Notifications are blocked for this site. Allow them in the browser’s site settings, then reload.';

  return (
    <div className="flex items-center gap-1">
      <Button
        variant={enabled ? 'default' : 'outline'}
        size="sm"
        className="gap-2"
        disabled={blocked}
        aria-pressed={enabled}
        onClick={() => (enabled ? disable() : void enable())}
      >
        {enabled ? <Bell className="size-4" aria-hidden="true" /> : <BellOff className="size-4" aria-hidden="true" />}
        {enabled ? 'Fault notifications on' : 'Notify me of faults'}
      </Button>
      <InfoPopover label="fault notifications" align="end">
        {blocked ? (
          <p>{reason}</p>
        ) : (
          <p>
            Shows a notification when the inverter reports a new fault while this app is open
            but not in front (another tab or window, minimized). A closed app, or a phone app in
            the background, gets none. Waiting or standby at night is not a fault.
          </p>
        )}
      </InfoPopover>
    </div>
  );
}

