// API Response Types
export interface ErrorResponse {
  code: number
  error: string
  message: string
}

// Status decoded types for WebSocket messages
export type SolisStatusDecoded = {
  name: string
  description: string
}

export type FaultStatusDecoded = string[]

// WebSocket Message Types
export interface WebSocketCacheUpdate {
  type: 'cache_update'
  timestamp: string
  data: Record<string, {
    Key: string
    Name: string
    RawValue: number
    DecodedValue: number
    StringValue: string
    Unit: string
    Timestamp: string
    status_decoded?: SolisStatusDecoded | FaultStatusDecoded
  }>
}

export type WebSocketMessage = WebSocketCacheUpdate

export interface RegisterInfo {
  address: number
  data_type: string
  description: string
  key: string
  name: string
  stability: string
  unit: string
}

export interface DailyDataPoint {
  date: string
  raw_value: number
  value: number
}

export interface HistoryDataPoint {
  count?: number
  max?: number
  min?: number
  timestamp: string
  value: number
}

export type Interval = 'raw'

export interface HistoryResult {
  data: HistoryDataPoint[]
  interval: Interval
  key: string
  unit: string
}

export interface MonthlyDataPoint {
  month: string
  raw_value: number
  value: number
}

export interface TotalDataPoint {
  raw_value: number
  timestamp: string
  value: number
}

export interface YearlyDataPoint {
  raw_value: number
  value: number
  year: string
}

// Device info type (flexible for various stable registers)
export interface DeviceInfo {
  [key: string]: any
}

// Register value with metadata
export interface RegisterValue {
  key: string
  name: string
  value: number | string
  unit: string
  description?: string
  raw_value?: number
  timestamp?: string
}
