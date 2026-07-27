import { LineAwesomeIcon } from '../ui/LineAwesomeIcon';

interface OfflineBannerProps {
  isOnline: boolean;
  isChecking?: boolean;
}

export function OfflineBanner({ isOnline, isChecking = false }: OfflineBannerProps) {
  if (isOnline || isChecking) return null;

  return (
    <div className="fixed bottom-4 left-4 right-4 z-60 max-w-[calc(100vw-2rem)]">
      <div className="flex items-center justify-between gap-4 p-4 bg-warning/30 border border-warning rounded-lg shadow-lg backdrop-blur-sm w-full">
        <div className="flex items-center gap-3">
          <LineAwesomeIcon icon="la-wifi-slash" size="lg" className="text-warning" />
          <div>
            <h3 className="font-semibold text-foreground">You are offline</h3>
            <p className="text-sm text-muted-foreground">
              Solis Monitor requires an active internet connection. Please check your network.
            </p>
          </div>
        </div>
        <button
          onClick={() => window.location.reload()}
          className="px-4 py-2 bg-warning text-warning-foreground rounded-md text-sm font-medium hover:bg-warning/90 transition-colors whitespace-nowrap"
        >
          Retry
        </button>
      </div>
    </div>
  );
}
