import type { RegisterInfo, RegisterValue, DeviceInfo } from '../types'
import { websocketClient, type WebSocketCacheUpdate } from '../api/websocket'
import { 
  getAllConfiguredKeys, 
  getRegisterConfig,
  type RegisterConfig 
} from '../config/registers'

// Remove: import { api } from '../api/client'

interface AppState {
  // Register information - loaded from config, not backend
  allRegisters: RegisterInfo[]
  registerMap: Map<string, RegisterInfo>
  
  // Current values for configured registers
  currentValues: Map<string, RegisterValue>
  
  // Device information - will be loaded via WebSocket
  deviceInfo: DeviceInfo | null
  
  // Loading state
  isLoading: boolean
  isOnline: boolean
  lastUpdated: Date | null
  
  // Error state
  error: string | null
}

const state: AppState = {
  allRegisters: [],
  registerMap: new Map(),
  currentValues: new Map(),
  deviceInfo: null,
  isLoading: true,
  isOnline: false,
  lastUpdated: null,
  error: null,
}

// initialized tracks if the store has been initialized
let initialized = false

export const store = {
  getState: () => state,

  async initialize() {
    if (initialized) return
    initialized = true

    state.isLoading = true
    state.error = null

    // Initialize WebSocket connection
    websocketClient.connect()
    
    // Set up WebSocket message handler
    const unsubscribe = websocketClient.onMessage((message) => {
      if (message.type === 'cache_update') {
        this.handleCacheUpdate(message as WebSocketCacheUpdate)
      }
    })

    // Set up connect/disconnect handlers
    const onConnect = websocketClient.onConnect(() => {
      state.isOnline = true
      state.error = null
    })
    
    const onDisconnect = websocketClient.onDisconnect(() => {
      state.isOnline = false
      state.error = 'Disconnected from server'
    })

    state.isLoading = false
    state.lastUpdated = new Date()

    // Cleanup function (optional, for unmounting)
    return () => {
      unsubscribe()
      onConnect()
      onDisconnect()
    }
  },

  handleCacheUpdate(message: WebSocketCacheUpdate): void {
    const updateData = message.data
    
    for (const [key, value] of Object.entries(updateData)) {
      const config = getRegisterConfig(key)
      if (config) {
        // For status registers with decoded status information
        if (value.status_decoded !== undefined && value.status_decoded !== null) {
          // Format status_decoded to a string
          let displayValue: string
          if (Array.isArray(value.status_decoded)) {
            // For fault status registers (array of strings)
            displayValue = value.status_decoded.join(', ')
          } else if (typeof value.status_decoded === 'object' && value.status_decoded !== null) {
            // For solis_status (object with name and description)
            const statusObj = value.status_decoded as { name?: string; description?: string }
            displayValue = statusObj.name || JSON.stringify(value.status_decoded)
          } else {
            displayValue = String(value.status_decoded)
          }
          
          state.currentValues.set(key, {
            key: key,
            name: config.name,
            value: displayValue,
            unit: config.unit || value.Unit || '',
            description: config.description,
            raw_value: value.RawValue,
            timestamp: value.Timestamp,
          })
        } else {
          // Use DecodedValue if available, otherwise RawValue
          const displayValue = value.DecodedValue !== undefined ? value.DecodedValue : value.RawValue
          state.currentValues.set(key, {
            key: key,
            name: config.name,
            value: displayValue,
            unit: config.unit || value.Unit || '',
            description: config.description,
            raw_value: value.RawValue,
            timestamp: value.Timestamp,
          })
        }
      }
    }
    
    state.lastUpdated = new Date(message.timestamp)
    state.isOnline = true
    state.error = null
  },

  getRegisterValue(key: string): RegisterValue | null {
    return state.currentValues.get(key) || null
  },

  getRegisterConfig(key: string): RegisterConfig | null {
    return getRegisterConfig(key)
  },

  getAllConfiguredValues(): Map<string, RegisterValue> {
    return state.currentValues
  },

  async refresh() {
    // With WebSocket-only, no action needed
    // The cache updates are pushed automatically
    // This method is kept for compatibility but does nothing
  },

  // Get all configured keys (from config, not from backend)
  getConfiguredKeys(): string[] {
    return getAllConfiguredKeys()
  },
}
