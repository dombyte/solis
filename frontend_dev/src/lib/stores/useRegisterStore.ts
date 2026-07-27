import { create } from 'zustand';
import type { RegisterMetadata, RegisterValue, SolisStatusDecoded, FaultStatusDecoded, ApiDataObject } from '../../types';
import { apiDataObjects } from '../config/data';
import { resolveTemplate, isTemplate, type TemplateContext } from '../utils/template';

interface RegisterStoreState {
  // Maps for register metadata
  registerMetadata: Map<string, RegisterMetadata>;
  registerMetadataByKey: Map<string, RegisterMetadata>;
  
  // Map for register values
  registerValues: Map<string, RegisterValue>;
  
  // Connection and loading state
  isConnected: boolean;
  lastUpdated: Date | null;
  isLoading: boolean;
  error: string | null;
  
  // Actions
  initialize: () => Promise<void>;
  updateValue: (key: string, value: unknown, timestamp?: string) => void;
  updateValues: (data: Record<string, unknown>, timestamp?: string) => void;
  setConnected: (connected: boolean) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  
  // Getters
  getRegisterById: (id: string) => RegisterMetadata | undefined;
  getRegisterByKey: (key: string) => RegisterMetadata | undefined;
  getValueById: (id: string) => RegisterValue | undefined;
  getValueByKey: (key: string) => RegisterValue | undefined;
  getValuesByGroup: (groupId: string) => RegisterValue[];
  getAllValues: () => RegisterValue[];

  // Template resolution getter
  getResolvedRegisterById: (id: string) => (RegisterMetadata & { displayValue: number | string | null | undefined }) | undefined;
  
  // Helper methods
  hasDataForId: (id: string) => boolean;
  hasDataForKey: (key: string) => boolean;
}

// Convert ApiDataObject to RegisterMetadata
function toRegisterMetadata(obj: ApiDataObject): RegisterMetadata {
  return {
    id: obj.id,
    key: obj.key,
    name: obj.name,
    description: obj.description,
    unit: obj.unit,
    category: obj.category,
    group: obj.group,
    source: obj.source,
    format: obj.format,
    precision: obj.precision,
    scale: obj.scale,
  };
}

// Helper to resolve register metadata templates with current value
function resolveRegisterMetadata(
  metadata: RegisterMetadata,
  regValue: RegisterValue | undefined
): RegisterMetadata & { displayValue: number | string | null | undefined } {
  // Skip if no templates and no value field
  if (!regValue && !metadata.value && 
      !isTemplate(metadata.name) && 
      !isTemplate(metadata.description ?? '') &&
      !isTemplate(metadata.unit ?? '')) {
    const displayValue = regValue ? (regValue as RegisterValue).value : undefined;
    return { ...metadata, displayValue };
  }
  
  const val = regValue || { value: null, rawValue: undefined, timestamp: undefined, statusDecoded: undefined } as RegisterValue;
  
  const context: TemplateContext = {
    // From metadata
    key: metadata.key,
    id: metadata.id,
    name: metadata.name,
    description: metadata.description || '',
    unit: metadata.unit || '',
    source: metadata.source,
    precision: metadata.precision,
    format: metadata.format,
    category: metadata.category,
    group: metadata.group,
    
    // From API/WS data (via store value)
    Key: metadata.key,
    Name: val.statusDecoded && typeof val.statusDecoded === 'object' && 'name' in val.statusDecoded
      ? (val.statusDecoded as SolisStatusDecoded).name
      : metadata.name,
    Description: metadata.description || '',
    Unit: metadata.unit || '',
    
    // From RegisterValue
    value: val.value,
    rawValue: val.rawValue,
    timestamp: val.timestamp,
    statusDecoded: val.statusDecoded,
    
    // Direct API data properties (for template resolution)
    DecodedValue: val.rawValue !== undefined ? val.rawValue : (typeof val.value === 'number' ? val.value : undefined),
    RawValue: val.rawValue,
    StringValue: val.value !== null && val.value !== undefined 
      ? String(val.value) : '',
  };
  
  // Resolve the display value based on metadata.value field
  let displayValue: number | string | null | undefined = val.value;
  
  if (metadata.value && isTemplate(metadata.value)) {
    const placeholder = metadata.value.match(/\{(\w+)\}/)?.[1];
    if (placeholder) {
      const templateValue = context[placeholder as keyof TemplateContext];
      displayValue = templateValue !== null && templateValue !== undefined 
        ? String(templateValue) 
        : undefined;
    }
  } else if (metadata.value) {
    displayValue = metadata.value;
  } else {
    displayValue = val.value ?? val.rawValue;
  }
  
  return {
    ...metadata,
    name: isTemplate(metadata.name) ? resolveTemplate(metadata.name, context) : metadata.name,
    description: metadata.description && isTemplate(metadata.description) 
      ? resolveTemplate(metadata.description, context) 
      : metadata.description,
    unit: metadata.unit && isTemplate(metadata.unit) 
      ? resolveTemplate(metadata.unit, context) 
      : metadata.unit,
    displayValue,
  };
}

export const useRegisterStore = create<RegisterStoreState>((set, get) => ({
  // Initialize maps
  registerMetadata: new Map(),
  registerMetadataByKey: new Map(),
  registerValues: new Map(),
  
  // Initial state
  isConnected: false,
  lastUpdated: null,
  isLoading: true,
  error: null,

  initialize: () => {
    return new Promise<void>((resolve) => {
      const metadata = new Map<string, RegisterMetadata>();
      const metadataByKey = new Map<string, RegisterMetadata>();
      const values = new Map<string, RegisterValue>();
      
      apiDataObjects.forEach(obj => {
        const regMeta = toRegisterMetadata(obj);
        metadata.set(regMeta.id, regMeta);
        metadataByKey.set(regMeta.key, regMeta);
        
        // Initialize values with null
        values.set(regMeta.id, {
          key: regMeta.key,
          id: regMeta.id,
          value: null,
          unit: regMeta.unit,
        });
      });
      
      set({
        registerMetadata: metadata,
        registerMetadataByKey: metadataByKey,
        registerValues: values,
        isLoading: false,
      });
      
      resolve();
    });
  },

  updateValue: (key, value, timestamp) => {
    const regByKey = get().registerMetadataByKey.get(key);
    if (regByKey) {
      const newValues = new Map(get().registerValues);
      
      // Handle status_decoded if present in the value object
      let statusDecoded: SolisStatusDecoded | FaultStatusDecoded | undefined;
      let displayValue: number | string | null = typeof value === 'number' ? value : typeof value === 'string' ? value : null;
      let rawValue: number | undefined;
      
      if (typeof value === 'object' && value !== null) {
        const val = value as Record<string, unknown>;
        // Extract status_decoded first as it takes priority for display
        if (val.status_decoded !== undefined && val.status_decoded !== null) {
          statusDecoded = val.status_decoded as SolisStatusDecoded | FaultStatusDecoded;
          // For status registers, use the decoded status as the display value
          if (Array.isArray(statusDecoded)) {
            displayValue = statusDecoded.join(', ');
          } else if (typeof statusDecoded === 'object') {
            const statusObj = statusDecoded as { name?: string; description?: string };
            displayValue = statusObj.name || JSON.stringify(statusDecoded);
          } else {
            displayValue = String(statusDecoded);
          }
          // For rawValue, try to extract the numeric raw value
          rawValue = (val.RawValue as number | undefined) ?? (val.DecodedValue as number | undefined);
        } else {
          // For numeric registers without status_decoded, prefer DecodedValue then RawValue
          // This avoids using StringValue which might contain debug info like "Raw: 498"
          const decodedValue = val.DecodedValue as number | string | undefined;
          const rawValueTemp = val.RawValue as number | string | undefined;
          const stringValue = val.StringValue as string | undefined;
          displayValue = decodedValue ?? rawValueTemp ?? stringValue ?? (value !== null && value !== undefined ? String(value) : null);
          rawValue = (val.RawValue as number | undefined) ?? (val.DecodedValue as number | undefined);
        }
      } else if (typeof value === 'number') {
        // Simple numeric value
        rawValue = value;
        displayValue = value;
      }
      
      newValues.set(regByKey.id, {
        key,
        id: regByKey.id,
        value: displayValue,
        rawValue: typeof rawValue === 'number' ? rawValue : undefined,
        timestamp,
        unit: regByKey.unit,
        statusDecoded,
      });
      set({
        registerValues: newValues,
        lastUpdated: timestamp ? new Date(timestamp) : new Date(),
      });
    }
  },

  updateValues: (data, timestamp) => {
    const updates = new Map(get().registerValues);
    const metadataByKey = get().registerMetadataByKey;
    
    Object.entries(data).forEach(([key, value]) => {
      const regByKey = metadataByKey.get(key);
      if (regByKey) {
        // Handle case where value is an object with DecodedValue (from WebSocket)
        let actualValue: number | string | null = typeof value === 'number' ? value : typeof value === 'string' ? value : null;
        let rawValue: number | string | undefined = undefined;
        let statusDecoded: SolisStatusDecoded | FaultStatusDecoded | undefined;
        
        if (typeof value === 'object' && value !== null) {
          const val = value as Record<string, unknown>;
          // Extract status_decoded first as it takes priority for display
          if (val.status_decoded !== undefined && val.status_decoded !== null) {
            statusDecoded = val.status_decoded as SolisStatusDecoded | FaultStatusDecoded;
            // For status registers, use the decoded status as the display value
            if (Array.isArray(statusDecoded)) {
              actualValue = statusDecoded.join(', ');
            } else if (typeof statusDecoded === 'object') {
              const statusObj = statusDecoded as { name?: string; description?: string };
              actualValue = statusObj.name || JSON.stringify(statusDecoded);
            } else {
              actualValue = String(statusDecoded);
            }
            // For rawValue, try to extract the numeric raw value
            rawValue = (val.RawValue as number | string | undefined) ?? (val.DecodedValue as number | string | undefined);
          } else {
            // For numeric registers without status_decoded, prefer DecodedValue then RawValue
            // This avoids using StringValue which might contain debug info like "Raw: 498"
            const decodedValue = val.DecodedValue as number | string | undefined;
            const rawValueTemp = val.RawValue as number | string | undefined;
            const stringValue = val.StringValue as string | undefined;
            actualValue = decodedValue ?? rawValueTemp ?? stringValue ?? (value !== null && value !== undefined ? String(value) : null);
            rawValue = (val.RawValue as number | string | undefined) ?? (val.DecodedValue as number | string | undefined);
          }
        } else if (typeof value === 'number') {
          // Simple numeric value
          actualValue = value;
          rawValue = value;
        }
        
        updates.set(regByKey.id, {
          key,
          id: regByKey.id,
          value: actualValue,
          rawValue: typeof rawValue === 'number' ? rawValue : undefined,
          timestamp,
          unit: regByKey.unit,
          statusDecoded,
        });
      }
    });
    
    set({
      registerValues: updates,
      lastUpdated: timestamp ? new Date(timestamp) : new Date(),
    });
  },

  setConnected: (connected) => set({ isConnected: connected }),
  setLoading: (loading) => set({ isLoading: loading }),
  setError: (error) => set({ error }),

  getRegisterById: (id) => get().registerMetadata.get(id),
  getRegisterByKey: (key) => get().registerMetadataByKey.get(key),
  
  getValueById: (id) => get().registerValues.get(id),
  
  getValueByKey: (key) => {
    const reg = get().registerMetadataByKey.get(key);
    return reg ? get().registerValues.get(reg.id) : undefined;
  },
  
  getResolvedRegisterById: (id) => {
    const metadata = get().registerMetadata.get(id);
    if (!metadata) return undefined;
    const value = get().registerValues.get(id);
    return resolveRegisterMetadata(metadata, value);
  },
  
  getValuesByGroup: (groupId) => {
    const allValues = Array.from(get().registerValues.values());
    const metadata = get().registerMetadata;
    
    return allValues.filter(v => {
      const reg = metadata.get(v.id);
      return reg?.group === groupId;
    });
  },
  
  getAllValues: () => Array.from(get().registerValues.values()),
  
  hasDataForId: (id) => {
    const value = get().registerValues.get(id);
    return value !== undefined && value.value !== null;
  },
  
  hasDataForKey: (key) => {
    const reg = get().registerMetadataByKey.get(key);
    if (!reg) return false;
    const value = get().registerValues.get(reg.id);
    return value !== undefined && value.value !== null;
  },
}));
