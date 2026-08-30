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
      // Get workbox instance from window
      const wb = window.__workbox;
      
      if (wb) {
        // Use workbox-window's update method to force check for updates
        // update() returns void, updates are communicated via events
        await wb.update();
        
        // After update(), check if there's a waiting service worker
        // We need to check this after a small delay to allow the SW to process
        setTimeout(async () => {
          const registration = await navigator.serviceWorker.getRegistration();
          const hasWaiting = !!registration?.waiting;
          const hasActive = !!registration?.active;
          
          // Only consider it an update if there's an active SW being replaced
          const isUpdate = hasWaiting && hasActive;
          
          setUpdateState({ hasUpdate: isUpdate });
          if (isUpdate) {
            updateAvailableRef.current = wb;
          } else {
            updateAvailableRef.current = null;
          }
        }, 100);
        
        // Reset retry counter
        checkAttemptsRef.current = 0;
        return;
      }

      // Fallback to native API if workbox-window is not available
      const registration = await navigator.serviceWorker.getRegistration();
      const hasWaiting = !!registration?.waiting;
      const hasActive = !!registration?.active;
      
      if (hasWaiting) {
        // Only show update banner if we have an active service worker
        // This prevents false positives on initial registration
        const isUpdate = hasActive;
        
        if (isUpdate) {
          setUpdateState({ hasUpdate: true });
          updateAvailableRef.current = null;
        } else {
          // This is initial registration, not an update
          setUpdateState({ hasUpdate: false });
          updateAvailableRef.current = null;
        }
        
        // Reset retry counter since we found a registration
        checkAttemptsRef.current = 0;
      } else {
        // Reset state when no update is waiting
        setUpdateState({ hasUpdate: false });
        updateAvailableRef.current = null;
        
        // If no registration yet, retry after a short delay
        // This handles the case when PWA is first launching
        if (!registration && checkAttemptsRef.current < MAX_CHECK_ATTEMPTS) {
          checkAttemptsRef.current++;
          setTimeout(() => checkForUpdateRef.current?.(), 500);
        }
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
    
    const handleUpdateFound = () => {
      // When an update is found, check immediately and then continue periodic checks
      // Add a small delay to avoid race conditions during initial registration
      setTimeout(() => checkForUpdate(), 100);
    };

    const handleWorkboxWaiting = () => {
      // workbox-window emits 'waiting' event when update is available
      console.log('Workbox waiting event fired');
      // Use a timeout to avoid synchronous setState in useEffect
      setTimeout(() => {
        setUpdateState({ hasUpdate: true });
        updateAvailableRef.current = wb;
      }, 0);
    };

    const handleControlling = () => {
      // New SW is now controlling, reload the page
      setTimeout(() => window.location.reload(), 100);
    };

    // Start checking after a small delay to let SW register
    // This does an initial check to see if there's an update available on app load
    const initialDelay = setTimeout(() => {
      checkAttemptsRef.current = 0;
      checkForUpdate();
    }, 1000);

    // Setup listeners
    const setupListeners = async () => {
      // Get workbox instance from window
      wb = window.__workbox;
      
      if (wb) {
        // Listen to workbox-window events
        wb.addEventListener('waiting', handleWorkboxWaiting);
        wb.addEventListener('externalwaiting', handleWorkboxWaiting);
        wb.addEventListener('controlling', handleControlling);
      }

      // Also listen for native updatefound events on the registration
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
      if (wb) {
        wb.removeEventListener('waiting', handleWorkboxWaiting);
        wb.removeEventListener('externalwaiting', handleWorkboxWaiting);
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
