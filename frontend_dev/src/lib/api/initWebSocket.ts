/**
 * Initialize WebSocket connection early
 * This module should be imported as early as possible (e.g., in main.tsx before rendering)
 * to ensure the WebSocket connection is established before the app needs data.
 * 
 * Note: The initial data request is handled by the pages when they mount,
 * not here, to ensure the request is sent when pages are ready to receive it.
 */
import { websocketClient } from './websocket';

// Flag to ensure we only initialize once
let initialized = false;

export function initWebSocket(): void {
  if (initialized) return;
  initialized = true;
  
  // Connect WebSocket immediately
  websocketClient.connect();
}
