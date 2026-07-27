type FormatType = 'number' | 'percentage' | 'power' | 'energy' | 'voltage' | 'current' | 'temperature';

type TemplateString = string;

// API Data Object Interface
export interface ApiDataObject {
  key: string;        // External - matches API/WS
  id: string;         // Internal - used in frontend
  name: string | TemplateString;
  description?: string | TemplateString;
  unit?: string | TemplateString;
  source: string;    // Full API path (e.g., '/api/data/temperature')
  category?: string;
  group?: string;
  precision?: number;
  format?: FormatType;
  scale?: number;
  order?: number;
  visible?: boolean;
  value?: string | TemplateString; // Template for value display (e.g., '{DecodedValue}')
  externalApi?: {
    baseUrl?: string;
    path?: string;
    headers?: Record<string, string>;
    authTokenEnvVar?: string;
    pollInterval?: number;
    dataMapper?: (data: unknown) => number;
  };
}

// Group Configuration
export interface GroupConfig {
  id: string;
  title: string;
  description?: string;
  dataIds: string[];       // Internal IDs from data.ts
  category?: string;
  layout?: 'grid' | 'list' | 'compact';
  columns?: number;
  visible?: boolean;
  order?: number;
}

// Status decoded types for WebSocket messages
export type SolisStatusDecoded = {
  name: string;
  description: string;
};

export type FaultStatusDecoded = string[];

// WebSocket message types
interface WebSocketCacheUpdate {
  type: 'cache_update' | 'connected' | 'request_initial_data';
  data?: Record<string, {
    Key: string;
    Name: string;
    RawValue: number;
    DecodedValue: number;
    StringValue: string;
    Unit: string;
    Timestamp: string;
    status_decoded?: SolisStatusDecoded | FaultStatusDecoded;
  }>;
  timestamp?: string;
}

export type WebSocketMessage = WebSocketCacheUpdate;

// Raw data object from API or WebSocket
export interface RawRegisterData {
  Key: string;
  Name: string;
  RawValue: number;
  DecodedValue: number;
  StringValue: string;
  Unit: string;
  Timestamp: string;
  status_decoded?: SolisStatusDecoded | FaultStatusDecoded;
}

// Register metadata and values for store
export interface RegisterMetadata {
  id: string;
  key: string;
  name: string;
  description?: string;
  unit?: string;
  category?: string;
  group?: string;
  source: string;
  format?: FormatType;
  precision?: number;
  scale?: number;
  value?: string;
}

export interface RegisterValue {
  key: string;
  id: string;
  value: number | string | null;
  rawValue?: number;
  timestamp?: string;
  unit?: string;
  statusDecoded?: SolisStatusDecoded | FaultStatusDecoded;
}

// History data types
export interface HistoryDataPoint {
  date?: string;
  month?: string;
  year?: string;
  value: number;
  timestamp?: string;
}

// Chart data types
export interface ChartDataset {
  label: string;
  key: string;
  borderColor: string;
  backgroundColor: string;
  unit: string;
  data: (number | null)[];
}

export interface ChartData {
  labels: string[];
  datasets: ChartDataset[];
}

// Period types
export type Period = 'daily' | 'monthly' | 'yearly';
