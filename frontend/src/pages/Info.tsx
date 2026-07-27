import { LineAwesomeIcon } from '../components/ui/LineAwesomeIcon';
import { useWebSocket } from '../lib/hooks/useWebSocket';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { useState, useEffect } from 'react';

export function Info() {
  // Get git commit hash from import.meta.env if available
  const commitHash = import.meta.env.VITE_GIT_COMMIT_HASH || import.meta.env.VITE_GIT_VERSION || 'dev';
  const shortHash = typeof commitHash === 'string' ? commitHash.substring(0, 7) : 'dev';

  // Get WebSocket connection status (initialized at app level)
  const { isConnected } = useWebSocket({ autoConnect: false, requestInitialData: false });
  const lastUpdated = useRegisterStore(state => state.lastUpdated);

  // State for licenses log
  const [licensesLog, setLicensesLog] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
        <h1 className="hidden md:block text-2xl sm:text-3xl font-bold mb-6">About Solis Monitor</h1>

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
          <h2 className="text-lg font-semibold mb-4">Version</h2>
          <p className="text-muted-foreground">
            <code className="bg-muted px-2 py-1 rounded">v{shortHash}</code>
          </p>
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
