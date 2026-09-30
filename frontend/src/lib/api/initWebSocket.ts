/**
 * Initialize WebSocket connection and message routing early.
 * This module should be imported as early as possible (e.g., in main.tsx before rendering)
 * to ensure the WebSocket connection is established before the app needs data.
 */
import { websocketClient } from './websocket';
import { useRegisterStore } from '../stores/useRegisterStore';
import type { WebSocketMessage } from '../../types';

// Flag to ensure we only initialize once
let initialized = false;

function handleMessage(message: WebSocketMessage): void {
  const { applyWsValues } = useRegisterStore.getState();
  switch (message.type) {
    case 'snapshot':
      applyWsValues(message.values);
      break;
    case 'update':
      applyWsValues(message.values, message.ts, message.removed);
      break;
    case 'error':
      console.warn(`WebSocket error [${message.code}]: ${message.message}`, message.keys ?? []);
      break;
    case 'pong':
      break;
  }
}

export function initWebSocket(): void {
  if (initialized) return;
  initialized = true;

  websocketClient.onMessage(handleMessage);
  websocketClient.onConnect(() => useRegisterStore.getState().setConnected(true));
  // Live values are only trustworthy while connected: clear them on disconnect so the
  // dashboard shows "no data" (grayed) instead of frozen numbers, and so a reconnect
  // shows exactly what the server's snapshot contains (keys the server no longer has
  // stay empty instead of keeping their pre-disconnect value).
  websocketClient.onDisconnect(() => {
    const store = useRegisterStore.getState();
    store.setConnected(false);
    store.clearValues();
  });

  // Connect WebSocket immediately
  websocketClient.connect();
}
