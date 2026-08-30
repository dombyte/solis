import React, { useLayoutEffect, useState, useEffect, Suspense } from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import { ThemeProvider } from './components/theme-provider';
import { useRegisterStore } from './lib/stores/useRegisterStore';
import { OfflineBanner } from './components/layout/OfflineBanner';
import { LoadingScreen } from './components/layout/LoadingScreen';
import { websocketClient } from './lib/api/websocket';
import { initWebSocket } from './lib/api/initWebSocket';
import { App } from './App';

// Initialize WebSocket connection as early as possible
initWebSocket();

// Initialize service worker with workbox-window for better update control
// This must happen before the app renders so the SW can start checking for updates
const initializeServiceWorker = () => {
  if ('serviceWorker' in navigator) {
    // Import workbox-window dynamically
    import('workbox-window').then(({ Workbox }) => {
      const wb = new Workbox('/sw.js', { scope: '/' });
      
      // Store globally for use in hooks
      // Using any to avoid type mismatches between workbox-window versions
      (window as any).__workbox = wb;
      
      // Auto-register
      wb.register();
      
      console.log('Service Worker registered with workbox-window');
    }).catch((err: unknown) => {
      console.error('Failed to load workbox-window:', err);
    });
  }
};

// Initialize service worker
initializeServiceWorker();

function OnlineStatusProvider({ children }: { children: React.ReactNode }) {
  const [consecutiveFailures, setConsecutiveFailures] = useState(0);
  const MAX_FAILURES_BEFORE_SHOWING = 3;

  // Track WebSocket connection state for offline detection
  useEffect(() => {
    let isMounted = true;
    let failures = 0;

    const handleConnect = () => {
      if (isMounted) {
        failures = 0;
        setConsecutiveFailures(0);
      }
    };

    const handleDisconnect = () => {
      if (isMounted) {
        // Increment failure count on each disconnection
        failures++;
        setConsecutiveFailures(failures);
        
        // WebSocket will auto-reconnect internally, but we track failures here
        if (failures >= MAX_FAILURES_BEFORE_SHOWING) {
          console.warn(`Offline: WebSocket disconnected ${failures} times`);
        }
      }
    };

    const unsubscribeConnect = websocketClient.onConnect(handleConnect);
    const unsubscribeDisconnect = websocketClient.onDisconnect(handleDisconnect);

    // Check initial state
    if (websocketClient.isConnected()) {
      setConsecutiveFailures(0);
    }

    // Periodic check: if still disconnected after some time, count as additional failure
    const intervalId = setInterval(() => {
      if (!websocketClient.isConnected()) {
        // Only count if we haven't already exceeded the threshold
        if (failures < MAX_FAILURES_BEFORE_SHOWING) {
          failures++;
          setConsecutiveFailures(failures);
        }
      }
    }, 5000); // Check every 5 seconds

    return () => {
      isMounted = false;
      unsubscribeConnect();
      unsubscribeDisconnect();
      clearInterval(intervalId);
      // Don't disconnect WebSocket here - let it persist for the entire app lifecycle
      // It will be disconnected when the module is unloaded or the browser tab is closed
    };
  }, []);

  // Show offline banner if we've had 3 consecutive failures
  const isOffline = consecutiveFailures >= MAX_FAILURES_BEFORE_SHOWING;

  return (
    <>
      {children}
      <OfflineBanner isOnline={!isOffline} isChecking={false} />
    </>
  );
}

export function AppWithStore() {
  const initializedRef = React.useRef(false);
  const [isInitializing, setIsInitializing] = useState(true);
  const initialize = useRegisterStore(state => state.initialize);
  
  useLayoutEffect(() => {
    if (!initializedRef.current) {
      initializedRef.current = true;
      initialize().finally(() => {
        setIsInitializing(false);
      });
    }
  }, [initialize]);
  
  if (isInitializing) {
    return <LoadingScreen message="Initializing application..." />;
  }
  
  return (
    <Suspense fallback={<LoadingScreen message="Loading application..." />}>
      <App />
    </Suspense>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ThemeProvider defaultTheme="dark" storageKey="solis-theme">
      <OnlineStatusProvider>
        <AppWithStore />
      </OnlineStatusProvider>
    </ThemeProvider>
  </React.StrictMode>
);
