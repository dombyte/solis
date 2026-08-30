import { LineAwesomeIcon } from '../components/ui/LineAwesomeIcon';
import { useWebSocket } from '../lib/hooks/useWebSocket';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { useServiceWorkerUpdate } from '../lib/hooks/useServiceWorkerUpdate';
import { useMobile } from '../hooks/useMobile';
import { useState, useEffect, useCallback } from 'react';

export function Info() {
  const isMobile = useMobile();

  // Get git commit hash from import.meta.env if available
  const commitHash = import.meta.env.VITE_GIT_COMMIT_HASH || import.meta.env.VITE_GIT_VERSION || 'dev';
  const shortHash = typeof commitHash === 'string' ? commitHash.substring(0, 7) : 'dev';

  // Get WebSocket connection status (initialized at app level)
  const { isConnected } = useWebSocket({ autoConnect: false, requestInitialData: false });
  const lastUpdated = useRegisterStore(state => state.lastUpdated);

  // Service worker update check
  const { hasUpdate, checkForUpdate, triggerUpdate } = useServiceWorkerUpdate();
  const [checking, setChecking] = useState(false);
  const [lastCheckTime, setLastCheckTime] = useState<number | null>(null);
  const [checkStatus, setCheckStatus] = useState<'idle' | 'checking' | 'update-available' | 'up-to-date'>('idle');

  // State for licenses log
  const [licensesLog, setLicensesLog] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Manual update check handler
  const handleCheckForUpdate = useCallback(async () => {
    setChecking(true);
    setCheckStatus('checking');
    try {
      await checkForUpdate();
      // Small delay to allow state to propagate
      await new Promise(resolve => setTimeout(resolve, 100));
      setCheckStatus(hasUpdate ? 'update-available' : 'up-to-date');
    } finally {
      setChecking(false);
      setLastCheckTime(Date.now());
    }
  }, [checkForUpdate, hasUpdate]);

  // Trigger update and reload
  const handleTriggerUpdate = useCallback(() => {
    setCheckStatus('checking');
    triggerUpdate();
  }, [triggerUpdate]);

  // Fetch licenses.json as raw text
  useEffect(() => {
    const fetchLicenses = async () => {
      try {
        const baseUrl = import.meta.env.BASE_URL || '/';
        const response = await fetch(`${baseUrl}data/licenses.json`);

        if (response.status === 404) {
          setLicensesLog('Licenses file not found. Run build first.');
          setLoading(false);
          return;
        }

        if (!response.ok) {
          throw new Error(`Failed to load licenses: ${response.status} ${response.statusText}`);
        }

        const text = await response.text();
        setLicensesLog(text);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error');
      } finally {
        setLoading(false);
      }
    };

    fetchLicenses();
  }, []);

  return (
    <div className="p-4 sm:p-6 md:p-8 w-full overflow-x-hidden">
      <div className="max-w-2xl mx-auto w-full">
        {!isMobile ? <h1 className="text-2xl sm:text-3xl font-bold mb-6">About Solis Monitor</h1> : null}

        <div className="glassy-card rounded-xl p-4 sm:p-6 mb-6">
          <h2 className="text-lg font-semibold mb-4">About</h2>
          <p className="text-muted-foreground text-sm sm:text-base">
            Solis Monitor is a web application for monitoring Solis solar inverters.
            It provides real-time data visualization and historical tracking of your
            solar energy production and consumption.
          </p>
        </div>



        <div className="glassy-card rounded-xl p-4 sm:p-6 mb-6">
          <h2 className="text-lg font-semibold mb-4">Connection</h2>
          <div className="flex items-start gap-2">
            <span className={`h-2 w-2 rounded-full ${isConnected ? 'bg-success' : 'bg-destructive'} mt-1`} />
            <div className="text-sm">
              <span>Websockets</span>
              {lastUpdated && (
                <div className="mt-1.5">
                  Last Updated: <span className="text-muted-foreground">{lastUpdated.toLocaleTimeString()}</span>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="glassy-card rounded-xl p-4 sm:p-6 mb-6">
          <h2 className="text-lg font-semibold mb-4">Links</h2>
          <div className="flex flex-col sm:flex-row gap-4">
            <a
              href="/docs"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 hover:text-foreground transition-colors text-foreground"
            >
              <LineAwesomeIcon icon="la-external-link-alt" size="sm" />
              <span>API Docs</span>
            </a>
            <a
              href="https://github.com/dombyte/solis"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 hover:text-foreground transition-colors text-foreground"
            >
              <LineAwesomeIcon icon="la-github" style="brand" size="sm" />
              <span>GitHub</span>
            </a>
          </div>
        </div>

        <div className="glassy-card rounded-xl p-4 sm:p-6 mb-6">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold mb-1">Version</h2>
              <p className="text-muted-foreground">
                <code className="bg-muted px-2 py-1 rounded">v{shortHash}</code>
              </p>
            </div>
            {'serviceWorker' in navigator && (
              <div className="flex items-center gap-2 flex-wrap">
                <button
                  onClick={handleCheckForUpdate}
                  disabled={checking}
                  className={`flex items-center gap-1.5 px-3 py-1.5 bg-muted hover:bg-accent text-sm rounded-md transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed ${checking ? 'animate-pulse' : ''}`}
                  title="Check for updates"
                >
                  <LineAwesomeIcon icon={checking ? 'la-spinner fa-spin' : 'la-sync-alt'} size="sm" />
                  {checking ? 'Checking...' : 'Check Update'}
                </button>
                {hasUpdate && (
                  <button
                    onClick={handleTriggerUpdate}
                    className="flex items-center gap-1.5 px-3 py-1.5 bg-primary hover:bg-primary/90 text-primary-foreground text-sm rounded-md transition-all duration-200"
                    title="Apply update"
                  >
                    <LineAwesomeIcon icon="la-rocket" size="sm" />
                    Apply Update
                  </button>
                )}
              </div>
            )}
          </div>
          {lastCheckTime && (
            <div className={`mt-3 p-3 rounded-lg transition-all duration-300 ${
              checkStatus === 'update-available' ? 'bg-amber-500/10 border border-amber-500/20' :
              checkStatus === 'up-to-date' ? 'bg-emerald-500/10 border border-emerald-500/20' :
              'bg-muted/50'
            }`}>
              <p className={`text-sm flex items-center gap-2 ${
                checkStatus === 'update-available' ? 'text-amber-500' :
                checkStatus === 'up-to-date' ? 'text-emerald-500' :
                'text-muted-foreground'
              } ${checkStatus === 'checking' ? 'animate-pulse' : ''}`}>
                {checkStatus === 'checking' && (
                  <>
                    <LineAwesomeIcon icon="la-spinner fa-spin" size="sm" />
                    <span>Checking for updates...</span>
                  </>
                )}
                {checkStatus === 'update-available' && (
                  <>
                    <LineAwesomeIcon icon="la-exclamation-circle" size="sm" />
                    <span>Update available! Click "Apply Update" to install.</span>
                  </>
                )}
                {checkStatus === 'up-to-date' && (
                  <>
                    <LineAwesomeIcon icon="la-check-circle" size="sm" />
                    <span>Up to date - Last checked: {new Date(lastCheckTime).toLocaleTimeString()}</span>
                  </>
                )}
                {checkStatus === 'idle' && lastCheckTime && (
                  <span>Last checked: {new Date(lastCheckTime).toLocaleTimeString()}</span>
                )}
              </p>
            </div>
          )}
        </div>

        {/* Licenses Section - Log-style display */}
        <div className="glassy-card rounded-xl p-4 sm:p-6 mt-6 mb-6 sm:mb-8 lg:mb-10">
          <h2 className="text-lg font-semibold mb-4">Licenses</h2>
          <p className="text-muted-foreground text-sm mb-4">
            Third-party dependencies used in this application.
          </p>

          {error && (
            <p className="text-destructive text-sm">{error}</p>
          )}

          {loading && (
            <p className="text-muted-foreground text-sm">Loading licenses...</p>
          )}

          {!loading && !error && licensesLog && (
            <div className="max-h-96 overflow-y-auto">
              <pre className="font-mono text-xs text-muted-foreground whitespace-pre-wrap">
                {licensesLog}
              </pre>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
