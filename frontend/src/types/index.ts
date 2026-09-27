type FormatType = 'number' | 'percentage' | 'power' | 'energy' | 'voltage' | 'current';

type TemplateString = string;

// API Data Object Interface
export interface ApiDataObject {
  key: string;        // External - matches API/WS
  id: string;         // Internal - used in frontend
  name: string | TemplateString;
  description?: string | TemplateString;
  unit?: string | TemplateString;
  source: string;    // Full API path (e.g., '/api/data/solis_status')
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

// WebSocket subscription protocol (v3): subscribe/unsubscribe/ping -> snapshot/update/error.
// One key's value on the wire, rounded to 2 decimals server-side.
export interface WsValueDTO {
  value: number;
  timestamp?: string;
  unit?: string;
  status_decoded?: SolisStatusDecoded | FaultStatusDecoded;
}

interface WsSnapshotMessage {
  type: 'snapshot';
  values: Record<string, WsValueDTO>;
}

interface WsUpdateMessage {
  type: 'update';
  ts: string;
  values: Record<string, WsValueDTO>;
}

interface WsErrorMessage {
  type: 'error';
  code: string;
  message: string;
  keys?: string[];
}

export type WebSocketMessage = WsSnapshotMessage | WsUpdateMessage | WsErrorMessage;

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
