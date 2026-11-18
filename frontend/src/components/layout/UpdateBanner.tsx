import { useVersionCheck } from '../../lib/hooks/useVersionCheck';
import { RefreshCw, X } from 'lucide-react';
import { useMobile } from '../../hooks/useMobile';

export function UpdateBanner() {
  const { hasUpdate, dismissed, triggerUpdate, dismissUpdate } = useVersionCheck();
  const isMobile = useMobile();

  if (!hasUpdate || dismissed) return null;

  return (
    // Above the fixed mobile nav on mobile (App reserves its height below the content).
    <div className={`fixed ${isMobile ? 'above-mobile-nav' : 'bottom-4'} left-4 right-4 z-50 max-w-[calc(100vw-2rem)]`}>
      <div className="flex items-center justify-between gap-4 p-4 bg-card border border-border rounded-lg shadow-lg w-full" role="status">
        <div className="flex items-center gap-3">
          <RefreshCw className="size-5 text-primary" aria-hidden="true" />
          <div>
            <h3 className="font-semibold text-foreground">New version available</h3>
            <p className="text-sm text-muted-foreground">
              A new version of Solis Monitor is ready. Click update to get the latest features.
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={triggerUpdate}
            className="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm font-medium hover:bg-primary/90 transition-colors whitespace-nowrap"
          >
            Update Now
          </button>
          <button
            onClick={dismissUpdate}
            aria-label="Dismiss update notice"
            title="Dismiss"
            className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
          >
            <X className="size-5" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  );
}
