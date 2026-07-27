import { useEffect, useState, useCallback, useRef } from 'react';

interface UpdateState {
  hasUpdate: boolean;
}

export function useServiceWorkerUpdate() {
  const [updateState, setUpdateState] = useState<UpdateState>({
    hasUpdate: false,
  });

  const updateAvailableRef = useRef<ServiceWorkerRegistration | null>(null);
  const checkAttemptsRef = useRef(0);
  const MAX_CHECK_ATTEMPTS = 3;

  const checkForUpdate = useCallback(async () => {
    if (!('serviceWorker' in navigator)) return;

    try {
      const registration = await navigator.serviceWorker.getRegistration();
      
      if (registration?.waiting) {
        setUpdateState({ hasUpdate: true });
        updateAvailableRef.current = registration;
      } else if (checkAttemptsRef.current < MAX_CHECK_ATTEMPTS) {
        // If no registration yet, retry after a short delay
        // This handles the case when PWA is first launching
        checkAttemptsRef.current++;
        setTimeout(checkForUpdate, 500);
      }
    } catch (error) {
      console.error('Error checking for SW update:', error);
    }
  }, []);

  const triggerUpdate = useCallback(() => {
    const registration = updateAvailableRef.current;
    if (!registration?.waiting) return;

    registration.waiting.postMessage({ type: 'SKIP_WAITING' });

    const onControllerChange = () => {
      setTimeout(() => {
        window.location.reload();
      }, 100);
    };
    navigator.serviceWorker.addEventListener('controllerchange', onControllerChange, { once: true });
  }, []);

  useEffect(() => {
    if (!('serviceWorker' in navigator)) return;

    // Start checking after a small delay to let SW register
    const initialDelay = setTimeout(() => {
      checkAttemptsRef.current = 0;
      checkForUpdate();
    }, 1000);

    // Check every 30 seconds
    const intervalId = setInterval(checkForUpdate, 30_000);

    return () => {
      clearTimeout(initialDelay);
      clearInterval(intervalId);
    };
  }, [checkForUpdate]);

  return {
    hasUpdate: updateState.hasUpdate,
    triggerUpdate,
  };
}
