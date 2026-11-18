import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { CircleAlert, X } from 'lucide-react';
import { useStatusFaults } from '../../lib/hooks/useStatusFaults';
import { Button } from '../ui/button';

const DISMISSED_KEY = 'solis.faultBanner.dismissed';

// Dismissed for this tab session only; storage can throw (private mode).
function loadDismissed(): string | null {
  try {
    return window.sessionStorage.getItem(DISMISSED_KEY);
  } catch {
    return null;
  }
}

function saveDismissed(signature: string): void {
  try {
    window.sessionStorage.setItem(DISMISSED_KEY, signature);
  } catch {
    // dismissed until the page reloads
  }
}

/**
 * Active inverter faults above the dashboard, linking to the Status page. Dismissing it
 * hides this exact set of faults for the session; a new or changed fault shows it again.
 */
export function FaultBanner(): React.ReactElement | null {
  const { faults } = useStatusFaults();
  const [dismissed, setDismissed] = useState(loadDismissed);
  const signature = faults.map(f => `${f.id}:${f.text}`).join('|');

  if (faults.length === 0 || signature === dismissed) return null;

  const dismiss = () => {
    saveDismissed(signature);
    setDismissed(signature);
  };

  return (
    <div role="alert" className="flex items-start gap-3 rounded-xl border border-destructive/50 bg-destructive/10 p-3 sm:p-4">
      <CircleAlert className="mt-0.5 size-5 shrink-0 text-destructive" aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <p className="font-semibold text-foreground">
          {faults.length === 1 ? 'The inverter reports a fault' : `The inverter reports ${faults.length} faults`}
        </p>
        <ul className="mt-1 space-y-0.5 text-sm">
          {faults.map(f => (
            <li key={f.id} className="break-words">
              <span className="text-muted-foreground">{f.name}:</span> {f.text}
            </li>
          ))}
        </ul>
        <Link to="/status" className="mt-2 inline-block text-sm font-medium text-destructive hover:underline">
          Show status and history
        </Link>
      </div>
      <Button variant="ghost" size="icon" className="-m-1 size-8 shrink-0" onClick={dismiss} aria-label="Dismiss fault notice">
        <X className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}
