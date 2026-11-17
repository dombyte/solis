import { CircleAlert, CircleCheck, ExternalLink, FolderGit2, LoaderCircle, RefreshCw, Rocket } from 'lucide-react';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { useSubscription } from '../lib/hooks/useSubscription';
import { POWER_FLOW_KEYS } from '../components/dashboard/flow/model';
import { APP_VERSION, useVersionCheck } from '../lib/hooks/useVersionCheck';
import { useState, useEffect, useCallback } from 'react';
import { format } from 'date-fns';
import { Page } from '../components/layout/Page';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';

/** One entry of the generated licenses.json. */
interface License {
  name: string;
  version: string;
  license: string;
  repository?: string;
}

const CHECK_TONE = {
  'update-available': 'text-warning',
  'up-to-date': 'text-success',
  checking: 'text-muted-foreground animate-pulse',
  idle: 'text-muted-foreground',
} as const;

export function Info() {
  // WebSocket connection is initialized at app level. The store only stamps lastUpdated
  // when a subscribed value changes, so Info subscribes to the live power keys (they
  // change on nearly every poll) to keep "Last Updated" current on this page too.
  useSubscription([...POWER_FLOW_KEYS]);
  const isConnected = useRegisterStore(state => state.isConnected);
  const lastUpdated = useRegisterStore(state => state.lastUpdated);

  // Version check
  const { hasUpdate, checkForUpdate, triggerUpdate } = useVersionCheck();
  // Built with the binary's version (VITE_APP_VERSION); release versions have no "v".
  const version = /^\d/.test(APP_VERSION) ? `v${APP_VERSION}` : APP_VERSION;
  const [checking, setChecking] = useState(false);
  const [lastCheckTime, setLastCheckTime] = useState<number | null>(null);
  const [checkStatus, setCheckStatus] = useState<'idle' | 'checking' | 'update-available' | 'up-to-date'>('idle');

  const [licenses, setLicenses] = useState<License[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Manual update check handler
  const handleCheckForUpdate = useCallback(async () => {
    setChecking(true);
    setCheckStatus('checking');
    try {
      // Use the fresh result: `hasUpdate` from this render is the value before the check.
      const updateAvailable = await checkForUpdate();
      setCheckStatus(updateAvailable ? 'update-available' : 'up-to-date');
    } finally {
      setChecking(false);
      setLastCheckTime(Date.now());
    }
  }, [checkForUpdate]);

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
          setError('Licenses file not found. Run build first.');
          return;
        }

        if (!response.ok) {
          throw new Error(`Failed to load licenses: ${response.status} ${response.statusText}`);
        }

        const body: unknown = await response.json();
        if (!Array.isArray(body)) throw new Error('Unexpected licenses file');
        setLicenses(body as License[]);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error');
      } finally {
        setLoading(false);
      }
    };

    fetchLicenses();
  }, []);

  return (
    <Page title="Info">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card>
          <CardHeader><CardTitle>About</CardTitle></CardHeader>
          <CardContent>
            <p className="text-muted-foreground text-sm sm:text-base">
              Solis Monitor is a web application for monitoring Solis solar inverters.
              It provides real-time data visualization and historical tracking of your
              solar energy production and consumption.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Connection</CardTitle></CardHeader>
          <CardContent>
            <div className="flex items-start gap-2 text-sm">
              <span className={`h-2 w-2 rounded-full ${isConnected ? 'bg-success' : 'bg-destructive'} mt-1.5`} />
              <div>
                <span>WebSocket {isConnected ? 'connected' : 'disconnected'}</span>
                {lastUpdated && (
                  <div className="mt-1.5">
                    Last updated: <span className="text-muted-foreground">{format(lastUpdated, 'HH:mm:ss')}</span>
                  </div>
                )}
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Links</CardTitle></CardHeader>
          <CardContent>
            <div className="flex flex-col sm:flex-row gap-4 text-sm sm:text-base">
              <a href="/docs" target="_blank" rel="noopener noreferrer" className="flex items-center gap-2 hover:underline">
                <ExternalLink className="size-4" aria-hidden="true" />
                <span>API Docs</span>
              </a>
              <a href="https://github.com/dombyte/solis" target="_blank" rel="noopener noreferrer" className="flex items-center gap-2 hover:underline">
                <FolderGit2 className="size-4" aria-hidden="true" />
                <span>GitHub</span>
              </a>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Version</CardTitle></CardHeader>
          <CardContent>
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
              <code className="bg-muted px-2 py-1 rounded w-fit">{version}</code>
              <div className="flex items-center gap-2 flex-wrap">
                <Button variant="outline" size="sm" onClick={handleCheckForUpdate} disabled={checking}>
                  {checking ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <RefreshCw aria-hidden="true" />}
                  {checking ? 'Checking...' : 'Check for update'}
                </Button>
                {hasUpdate && (
                  <Button size="sm" onClick={handleTriggerUpdate}>
                    <Rocket aria-hidden="true" />
                    Apply update
                  </Button>
                )}
              </div>
            </div>
            {lastCheckTime && (
              <p className={`mt-3 text-sm flex items-center gap-2 ${CHECK_TONE[checkStatus]}`}>
                {checkStatus === 'checking' && <><LoaderCircle className="size-4 animate-spin" aria-hidden="true" /><span>Checking for updates...</span></>}
                {checkStatus === 'update-available' && <><CircleAlert className="size-4" aria-hidden="true" /><span>Update available: click "Apply update" to install.</span></>}
                {checkStatus === 'up-to-date' && <><CircleCheck className="size-4" aria-hidden="true" /><span>Up to date · last checked {format(lastCheckTime, 'HH:mm:ss')}</span></>}
                {checkStatus === 'idle' && <span>Last checked {format(lastCheckTime, 'HH:mm:ss')}</span>}
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Licenses</CardTitle>
          <CardDescription>Third-party dependencies used in this application.</CardDescription>
        </CardHeader>
        <CardContent>
          {error && <p className="text-destructive text-sm">{error}</p>}
          {loading && <p className="text-muted-foreground text-sm">Loading licenses...</p>}
          {licenses && (
            <div className="max-h-96 overflow-auto rounded-md border border-border">
              <table className="w-full text-sm">
                <thead className="sticky top-0 bg-card text-left text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 font-medium">Package</th>
                    <th className="px-3 py-2 font-medium">Version</th>
                    <th className="px-3 py-2 font-medium">License</th>
                  </tr>
                </thead>
                <tbody>
                  {licenses.map(l => (
                    <tr key={`${l.name}@${l.version}`} className="border-t border-border">
                      <td className="px-3 py-1.5">
                        {l.repository
                          ? <a href={l.repository} target="_blank" rel="noopener noreferrer" className="hover:underline">{l.name}</a>
                          : l.name}
                      </td>
                      <td className="px-3 py-1.5 text-muted-foreground tabular-nums">{l.version}</td>
                      <td className="px-3 py-1.5 text-muted-foreground">{l.license}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </Page>
  );
}
