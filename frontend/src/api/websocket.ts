import type { WebSocketCacheUpdate } from '../types'

export type { WebSocketCacheUpdate }

export type WebSocketMessage = WebSocketCacheUpdate

class SolisWebSocket {
  private ws: WebSocket | null = null
  private url: string
  private reconnectInterval: number = 5000 // 5 seconds
  private reconnectAttempts: number = 0
  private maxReconnectAttempts: number = 10
  private listeners: Set<(message: WebSocketMessage) => void> = new Set()
  private connected: boolean = false
  private onConnectCallbacks: (() => void)[] = []
  private onDisconnectCallbacks: (() => void)[] = []

  constructor(url?: string) {
    // Allow overriding WebSocket URL via environment variable
    // This is useful for development when frontend runs on different port than backend
    if (url) {
      this.url = url
    } else if (import.meta.env.VITE_WS_URL) {
      this.url = import.meta.env.VITE_WS_URL
    } else {
      this.url = '/ws'
    }
  }

  connect(): void {
    // If url starts with ws:// or wss://, use it as-is
    let wsUrl: string
    if (this.url.startsWith('ws://') || this.url.startsWith('wss://')) {
      wsUrl = this.url
    } else {
      // Use same protocol as page (ws:// or wss://) with same host
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const host = window.location.host
      wsUrl = `${protocol}//${host}${this.url}`
    }
    this.ws = new WebSocket(wsUrl)

    this.ws.onopen = () => {
      this.connected = true
      this.reconnectAttempts = 0
      console.log('WebSocket connected')
      this.onConnectCallbacks.forEach(cb => cb())
      // Request initial data on connection
      this.send({ type: 'request_initial_data' })
    }

    this.ws.onclose = (event) => {
      this.connected = false
      console.log('WebSocket disconnected:', event.code, event.reason)
      this.onDisconnectCallbacks.forEach(cb => cb())
      this.scheduleReconnect()
    }

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error)
    }

    this.ws.onmessage = (event) => {
      try {
        const message: WebSocketMessage = JSON.parse(event.data)
        this.listeners.forEach(listener => listener(message))
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error)
      }
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.warn('Max reconnection attempts reached')
      return
    }
    
    this.reconnectAttempts++
    const delay = Math.min(this.reconnectInterval * this.reconnectAttempts, 30000)
    
    setTimeout(() => {
      if (!this.connected) {
        console.log(`WebSocket reconnect attempt ${this.reconnectAttempts}...`)
        this.connect()
      }
    }, delay)
  }

  disconnect(): void {
    if (this.ws) {
      this.ws.close()
      this.ws = null
      this.connected = false
    }
  }

  isConnected(): boolean {
    return this.connected
  }

  onMessage(callback: (message: WebSocketMessage) => void): () => void {
    this.listeners.add(callback)
    return () => {
      this.listeners.delete(callback)
    }
  }

  onConnect(callback: () => void): () => void {
    this.onConnectCallbacks.push(callback)
    return () => {
      this.onConnectCallbacks = this.onConnectCallbacks.filter(cb => cb !== callback)
    }
  }

  onDisconnect(callback: () => void): () => void {
    this.onDisconnectCallbacks.push(callback)
    return () => {
      this.onDisconnectCallbacks = this.onDisconnectCallbacks.filter(cb => cb !== callback)
    }
  }

  send(message: any): void {
    if (this.ws && this.connected) {
      this.ws.send(JSON.stringify(message))
    }
  }
}

export const websocketClient = new SolisWebSocket()
