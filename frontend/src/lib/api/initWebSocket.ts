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
  }
}

export function initWebSocket(): void {
  if (initialized) return;
  initialized = true;

  websocketClient.onMessage(handleMessage);
  websocketClient.onConnect(() => useRegisterStore.getState().setConnected(true));
  websocketClient.onDisconnect(() => useRegisterStore.getState().setConnected(false));

  // Connect WebSocket immediately
  websocketClient.connect();
}
