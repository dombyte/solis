import type { WebSocketMessage } from '../../types';

type MessageListener = (message: WebSocketMessage) => void;

const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 30000;
const RECONNECT_JITTER = 0.2;

/**
 * Delay before reconnect attempt `attempt` (1-based): exponential from 1 s, capped at
 * 30 s, with ±20 % jitter so many open dashboards do not reconnect in lockstep after a
 * server restart. `random` is injectable for checks (defaults to Math.random).
 */
function reconnectDelay(attempt: number, random: () => number = Math.random): number {
  const exp = Math.min(RECONNECT_BASE_MS * 2 ** Math.max(0, attempt - 1), RECONNECT_MAX_MS);
  return Math.round(exp * (1 + (random() * 2 - 1) * RECONNECT_JITTER));
}

/**
 * Subscription client for the v3 WebSocket protocol (subscribe/unsubscribe/ping ->
 * snapshot/update/error). Keys are ref-counted across every caller of `subscribe()` so
 * multiple components can want the same key without double-subscribing or dropping it
 * early; the full active key set is resent on every (re)connect.
 */
class SolisWebSocket {
  private ws: WebSocket | null = null;
  private url: string;
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private connected = false;
  private shouldReconnect = true;
  private listeners: Set<MessageListener> = new Set();
  private onConnectCallbacks: (() => void)[] = [];
  private onDisconnectCallbacks: (() => void)[] = [];
  private subscriptions: Map<string, number> = new Map();

  constructor(url?: string) {
    // Allow overriding WebSocket URL via environment variable
    // This is useful for development when frontend runs on different port than backend
    if (url) {
      this.url = url;
    } else if (import.meta.env.VITE_WS_URL) {
      this.url = import.meta.env.VITE_WS_URL;
    } else if (import.meta.env.VITE_API_BASE_URL) {
      // If API base URL is configured, use it for WebSocket too
      const apiUrl = import.meta.env.VITE_API_BASE_URL;
      // Extract host and port from API URL
      try {
        const urlObj = new URL(apiUrl);
        const protocol = urlObj.protocol === 'https:' ? 'wss:' : 'ws:';
        this.url = `${protocol}//${urlObj.host}/ws`;
      } catch {
        this.url = '/ws';
      }
    } else {
      this.url = '/ws';
    }

    // Reconnecting never gives up (see scheduleReconnect); these events only skip the
    // current backoff wait when the network returns or the tab becomes visible again.
    if (typeof window !== 'undefined') {
      window.addEventListener('online', this.reconnectNow);
      window.addEventListener('focus', this.reconnectNow);
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') this.reconnectNow();
      });
    }
  }

  private reconnectNow = (): void => {
    this.reconnectAttempts = 0;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.shouldReconnect && !this.connected) {
      this.connect();
    }
  };

  connect(): void {
    // Don't create a new connection if we're already connected or connecting
    if (this.connected) {
      return;
    }

    if (this.ws) {
      // Check if the socket is in a non-closed state (CONNECTING or OPEN)
      const readyState = this.ws.readyState;
      if (readyState === WebSocket.CONNECTING || readyState === WebSocket.OPEN) {
        return;
      }
    }

    // If url starts with ws:// or wss://, use it as-is
    let wsUrl: string;
    if (this.url.startsWith('ws://') || this.url.startsWith('wss://')) {
      wsUrl = this.url;
    } else {
      // Use same protocol as page (ws:// or wss://) with same host
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = window.location.host;
      wsUrl = `${protocol}//${host}${this.url}`;
    }

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      this.connected = true;
      this.reconnectAttempts = 0;
      this.onConnectCallbacks.forEach(cb => cb());
      // Re-subscribe to the full active key set; the hub has no memory of this client.
      this.resubscribeAll();
    };

    this.ws.onclose = () => {
      this.connected = false;
      this.onDisconnectCallbacks.forEach(cb => cb());
      // Clear the reference to allow creating a new socket
      this.ws = null;
      if (this.shouldReconnect) {
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    this.ws.onmessage = (event) => {
      try {
        const message: WebSocketMessage = JSON.parse(event.data);
        this.listeners.forEach(listener => listener(message));
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error);
      }
    };
  }

  private resubscribeAll(): void {
    const keys = Array.from(this.subscriptions.keys());
    if (keys.length > 0) {
      this.send({ type: 'subscribe', keys });
    }
  }

  /**
   * Schedules the next attempt with capped exponential backoff. It never gives up: the
   * backend exits and is restarted by its container runtime when self-healing fails,
   * which can take longer than any fixed attempt budget.
   */
  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    this.reconnectAttempts++;
    const delay = reconnectDelay(this.reconnectAttempts);

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (!this.connected) {
        this.connect();
      }
    }, delay);
  }

  isConnected(): boolean {
    return this.connected;
  }

  onMessage(callback: MessageListener): () => void {
    this.listeners.add(callback);
    return () => { this.listeners.delete(callback); };
  }

  onConnect(callback: () => void): () => void {
    this.onConnectCallbacks.push(callback);
    return () => {
      this.onConnectCallbacks = this.onConnectCallbacks.filter(cb => cb !== callback);
    };
  }

  onDisconnect(callback: () => void): () => void {
    this.onDisconnectCallbacks.push(callback);
    return () => {
      this.onDisconnectCallbacks = this.onDisconnectCallbacks.filter(cb => cb !== callback);
    };
  }

  send(message: unknown): void {
    if (this.ws && this.connected && this.ws.readyState === WebSocket.OPEN) {
      try {
        this.ws.send(JSON.stringify(message));
      } catch (error) {
        console.warn('Failed to send WebSocket message:', error);
        this.connected = false;
      }
    }
  }

  /**
   * Ref-counted subscribe: sends a `subscribe` frame for keys new to the active set
   * (a no-op while disconnected; `resubscribeAll` covers them on the next connect).
   * The returned function releases this caller's interest, sending `unsubscribe` once
   * no other caller still needs a given key.
   */
  subscribe(keys: string[]): () => void {
    const newKeys: string[] = [];
    for (const key of keys) {
      const count = this.subscriptions.get(key) ?? 0;
      this.subscriptions.set(key, count + 1);
      if (count === 0) newKeys.push(key);
    }
    if (newKeys.length > 0) {
      this.send({ type: 'subscribe', keys: newKeys });
    }

    let released = false;
    return () => {
      if (released) return;
      released = true;
      const removedKeys: string[] = [];
      for (const key of keys) {
        const count = this.subscriptions.get(key) ?? 0;
        if (count <= 1) {
          this.subscriptions.delete(key);
          removedKeys.push(key);
        } else {
          this.subscriptions.set(key, count - 1);
        }
      }
      if (removedKeys.length > 0) {
        this.send({ type: 'unsubscribe', keys: removedKeys });
      }
    };
  }
}

// Singleton instance
export const websocketClient = new SolisWebSocket();
