import { useEffect, useState, useCallback, useRef } from 'react';

// Type matching workbox-window's Workbox class
interface WorkboxType {
  register: (options?: { immediate?: boolean }) => Promise<ServiceWorkerRegistration | undefined>;
  update: () => Promise<void>;
  messageSW: (data: object) => Promise<any>;
  messageSkipWaiting: () => void;
  addEventListener: (event: string, callback: (event: any) => void) => void;
  removeEventListener: (event: string, callback: (event: any) => void) => void;
}

declare global {
  interface Window {
    __workbox: any; // Using any to avoid type mismatches with workbox-window versions
  }
}

interface UpdateState {
  hasUpdate: boolean;
}

// Check if this is a real update (not first install or re-registration)
// Based on the approach from https://github.com/deanhume/pwa-update-available
// The updatefound event only fires when the service worker file actually changes
// We just need to check if there's an active controller to distinguish first install
const isRealUpdate = (): boolean => {
  // If there's a controller, it means there's an existing service worker
  // So if updatefound fires AND there's a controller, it's a real update
  return !!navigator.serviceWorker.controller;
};

// Type guard for WorkboxType
function isWorkboxType(obj: unknown): obj is WorkboxType {
  return (
    typeof obj === 'object' &&
    obj !== null &&
    'messageSkipWaiting' in obj &&
    typeof (obj as any).messageSkipWaiting === 'function' &&
    'update' in obj &&
    typeof (obj as any).update === 'function'
  );
}

export function useServiceWorkerUpdate() {
  const [updateState, setUpdateState] = useState<UpdateState>({
    hasUpdate: false,
  });

  const updateAvailableRef = useRef<any>(null);
  const checkAttemptsRef = useRef(0);
  const MAX_CHECK_ATTEMPTS = 3;
  const checkForUpdateRef = useRef<() => Promise<void>>(null);

  const checkForUpdate = useCallback(async () => {
    if (!('serviceWorker' in navigator)) return;

    try {
      const wb = window.__workbox;
      
      if (wb) {
        // Use workbox-window's update method to force check for updates
        // This will trigger the updatefound event if there's an update
        await wb.update();
        
        // Store the workbox instance for later use in triggerUpdate
        updateAvailableRef.current = wb;
        
        // Reset retry counter
        checkAttemptsRef.current = 0;
        return;
      }

      // Fallback to native API if workbox-window is not available
      const registration = await navigator.serviceWorker.getRegistration();
      
      if (registration) {
        // Force an update check
        await registration.update();
        updateAvailableRef.current = null;
        checkAttemptsRef.current = 0;
      } else if (checkAttemptsRef.current < MAX_CHECK_ATTEMPTS) {
        // If no registration yet, retry after a short delay
        // This handles the case when PWA is first launching
        checkAttemptsRef.current++;
        setTimeout(() => checkForUpdateRef.current?.(), 500);
      }
    } catch (error) {
      console.error('Error checking for SW update:', error);
    }
  }, []);
  
  useEffect(() => {
    checkForUpdateRef.current = checkForUpdate;
  }, [checkForUpdate]);

  const triggerUpdate = useCallback(() => {
    const wb = updateAvailableRef.current;
    
    const triggerReload = () => {
      const onControllerChange = () => {
        setTimeout(() => {
          window.location.reload();
        }, 100);
      };
      navigator.serviceWorker.addEventListener('controllerchange', onControllerChange, { once: true });
    };

    // Try workbox-window first
    if (wb && isWorkboxType(wb)) {
      // Use the built-in messageSkipWaiting method
      wb.messageSkipWaiting();
      triggerReload();
      return;
    }

    // Fallback to native API
    navigator.serviceWorker.getRegistration().then(registration => {
      if (!registration?.waiting) return;
      registration.waiting.postMessage({ type: 'SKIP_WAITING' });
      triggerReload();
    });
  }, []);

  useEffect(() => {
    if (!('serviceWorker' in navigator)) return;

    let registration: ServiceWorkerRegistration | null = null;
    let wb: any = null;
    let newWorker: ServiceWorker | null = null;
    
    const handleStateChange = () => {
      // Only show update when the new worker is installed
      if (newWorker?.state === 'installed' && isRealUpdate()) {
        setUpdateState({ hasUpdate: true });
      }
    };
    
    const handleUpdateFound = () => {
      // When an update is found (native SW API event)
      // This only fires when the service worker file actually changes
      // Get the installing worker and listen for state changes
      if (registration?.installing) {
        newWorker = registration.installing;
        newWorker.addEventListener('statechange', handleStateChange);
      }
    };

    const handleControlling = () => {
      // New SW is now controlling, reload the page
      setTimeout(() => window.location.reload(), 100);
    };

    // Start checking after a small delay to let SW register
    // The browser automatically checks for SW updates on page load
    // We just need to listen for the updatefound event
    const initialDelay = setTimeout(() => {
      checkAttemptsRef.current = 0;
    }, 1000);

    // Setup listeners
    const setupListeners = async () => {
      // Get workbox instance from window
      wb = window.__workbox;
      
      if (wb) {
        // Listen to workbox-window controlling event
        wb.addEventListener('controlling', handleControlling);
      }

      // Listen for native updatefound events on the registration
      const reg = await navigator.serviceWorker.getRegistration();
      if (reg) {
        registration = reg;
        registration.addEventListener('updatefound', handleUpdateFound);
      }
    };
    setupListeners();

    return () => {
      clearTimeout(initialDelay);
      if (registration) {
        registration.removeEventListener('updatefound', handleUpdateFound);
      }
      if (newWorker) {
        newWorker.removeEventListener('statechange', handleStateChange);
      }
      if (wb) {
        wb.removeEventListener('controlling', handleControlling);
      }
    };
  }, [checkForUpdate]);

  return {
    hasUpdate: updateState.hasUpdate,
    triggerUpdate,
    checkForUpdate,
  };
}
