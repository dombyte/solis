import { useServiceWorkerUpdate } from '../../lib/hooks/useServiceWorkerUpdate';
import { LineAwesomeIcon } from '../ui/LineAwesomeIcon';

export function UpdateBanner() {
  const { hasUpdate, triggerUpdate } = useServiceWorkerUpdate();

  if (!hasUpdate) return null;

  return (
    <div className="fixed bottom-4 left-4 right-4 z-50 max-w-[calc(100vw-2rem)]">
      <div className="flex items-center justify-between gap-4 p-4 bg-card border border-border rounded-lg shadow-lg w-full">
        <div className="flex items-center gap-3">
          <LineAwesomeIcon icon="la-sync-alt" size="lg" className="text-primary" />
          <div>
            <h3 className="font-semibold text-foreground">New version available</h3>
            <p className="text-sm text-muted-foreground">
              A new version of Solis Monitor is ready. Click update to get the latest features.
            </p>
          </div>
        </div>
        <button
          onClick={triggerUpdate}
          className="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm font-medium hover:bg-primary/90 transition-colors whitespace-nowrap"
        >
          Update Now
        </button>
      </div>
    </div>
  );
}
