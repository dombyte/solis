import { useEffect, useState, useCallback } from 'react';
import { websocketClient } from '../api/websocket';
import { useRegisterStore } from '../stores/useRegisterStore';
import type { WebSocketMessage } from '../../types';

export interface UseWebSocketOptions {
  autoConnect?: boolean;
  requestInitialData?: boolean;
}

export interface UseWebSocketResult {
  isConnected: boolean;
  error: string | null;
  connect: () => void;
  disconnect: () => void;
  send: (message: unknown) => void;
}

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketResult {
  const { autoConnect = true, requestInitialData = true } = options;
  const [error, setError] = useState<string | null>(null);
  const updateValues = useRegisterStore(state => state.updateValues);
  const setConnected = useRegisterStore(state => state.setConnected);
  const isConnected = useRegisterStore(state => state.isConnected);

  const handleMessage = useCallback((message: WebSocketMessage) => {
    switch (message.type) {
      case 'cache_update':
        updateValues(message.data || {}, message.timestamp);
        break;
      case 'connected':
        // Handle connection confirmation
        break;
      case 'request_initial_data':
        // Handle initial data request (usually sent by client)
        break;
      default:
        console.log('Received unknown WebSocket message type:', message.type);
    }
  }, [updateValues]);

  useEffect(() => {
    if (autoConnect) {
      websocketClient.connect();
    }
    const unsubscribe = websocketClient.onMessage(handleMessage);
    
    // Also update store connection state when websocket connects/disconnects
    const unsubscribeConnect = websocketClient.onConnect(() => {
      setConnected(true);
      setError(null);
      // Send initial request for data when connected (if enabled)
      if (requestInitialData) {
        websocketClient.send({ type: 'request_initial_data' });
      }
    });
    
    const unsubscribeDisconnect = websocketClient.onDisconnect(() => {
      setConnected(false);
    });

    // Sync store with current connection state on mount
    setConnected(websocketClient.isConnected());

    // If the socket is already connected and we want initial data, request it now
    if (requestInitialData && websocketClient.isConnected()) {
      try {
        websocketClient.send({ type: 'request_initial_data' });
      } catch (error) {
        console.log('Failed to send initial data request:', error);
      }
    }

    // Set up periodic connection check and keepalive
    // Note: The backend might close idle connections, so we need to send periodic messages
    // We use 'request_initial_data' as it's a known message type that the backend understands
    const heartbeatInterval = setInterval(() => {
      if (websocketClient.isConnected()) {
        try {
          // Send a known message type to keep the connection alive
          websocketClient.send({ type: 'request_initial_data' } as unknown);
        } catch {
          console.log('Keepalive failed, WebSocket might be disconnected');
        }
      } else if (autoConnect) {
        // Only attempt to reconnect if autoConnect is enabled
        console.log('WebSocket not connected, attempting to reconnect...');
        websocketClient.connect();
      }
    }, 25000); // Send keepalive every 25 seconds (less than backend's 5-minute timeout)

    return () => {
      clearInterval(heartbeatInterval);
      unsubscribe();
      unsubscribeConnect();
      unsubscribeDisconnect();
      if (autoConnect) websocketClient.disconnect();
    };
  }, [autoConnect, handleMessage, setConnected]);

  const connect = useCallback(() => {
    setError(null);
    websocketClient.connect();
  }, []);

  const disconnect = useCallback(() => {
    websocketClient.disconnect();
  }, []);

  const send = useCallback((message: unknown) => {
    websocketClient.send(message);
  }, []);

  return {
    isConnected,
    error,
    connect,
    disconnect,
    send,
  };
};
