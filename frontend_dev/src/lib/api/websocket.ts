import type { WebSocketMessage } from '../../types';

class SolisWebSocket {
  private ws: WebSocket | null = null;
  private url: string;
  private reconnectInterval = 5000;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 10;
  private connected = false;
  private shouldReconnect = true;
  private listeners: Set<(message: WebSocketMessage) => void> = new Set();
  private onConnectCallbacks: (() => void)[] = [];
  private onDisconnectCallbacks: (() => void)[] = [];

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
  }

  connect(): void {
    // Don't create a new connection if we're already connected or connecting
    if (this.connected) {
      console.log('WebSocket already connected, skipping new connection');
      return;
    }
    
    if (this.ws) {
      // Check if the socket is in a non-closed state (CONNECTING or OPEN)
      const readyState = this.ws.readyState;
      if (readyState === WebSocket.CONNECTING || readyState === WebSocket.OPEN) {
        console.log('WebSocket already connecting or open, skipping new connection');
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
    
    console.log('Connecting to WebSocket:', wsUrl);
    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      this.connected = true;
      this.reconnectAttempts = 0;
      console.log('WebSocket connected');
      this.onConnectCallbacks.forEach(cb => cb());
      this.listeners.forEach(cb => cb({ type: 'connected' } as WebSocketMessage));
    };

    this.ws.onclose = (event) => {
      this.connected = false;
      console.log('WebSocket disconnected:', event.code, event.reason);
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

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.warn('Max reconnection attempts reached');
      return;
    }
    
    this.reconnectAttempts++;
    const delay = Math.min(this.reconnectInterval * this.reconnectAttempts, 30000);
    
    setTimeout(() => {
      if (!this.connected) {
        console.log(`WebSocket reconnect attempt ${this.reconnectAttempts}...`);
        this.connect();
      }
    }, delay);
  }

  disconnect(): void {
    this.shouldReconnect = false;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
      this.connected = false;
    }
    this.shouldReconnect = true;
  }

  isConnected(): boolean {
    return this.connected;
  }

  onMessage(callback: (message: WebSocketMessage) => void): () => void {
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
    } else {
      console.log('WebSocket not ready, message not sent:', this.ws?.readyState);
    }
  }
}

// Singleton instance
export const websocketClient = new SolisWebSocket();
