import { create } from 'zustand';
import type { RegisterMetadata, RegisterValue, SolisStatusDecoded, ApiDataObject, WsValueDTO } from '../../types';
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
  
  // Actions
  initialize: () => Promise<void>;
  applyWsValues: (values: Record<string, WsValueDTO>, ts?: string, removed?: string[]) => void;
  setConnected: (connected: boolean) => void;
  clearValues: () => void;

  // Getters
  getRegisterById: (id: string) => RegisterMetadata | undefined;

  // Template resolution getter
  getResolvedRegisterById: (id: string) => (RegisterMetadata & { displayValue: number | string | null | undefined }) | undefined;
  
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
  
  const val = regValue || { value: null, timestamp: undefined, statusDecoded: undefined } as RegisterValue;
  
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
    timestamp: val.timestamp,
    statusDecoded: val.statusDecoded,

    // Direct API data properties (for template resolution); the backend already sends a
    // single scaled/rounded value, so DecodedValue and RawValue alias the same number.
    DecodedValue: typeof val.value === 'number' ? val.value : undefined,
    RawValue: typeof val.value === 'number' ? val.value : undefined,
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
    displayValue = val.value;
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

  applyWsValues: (values, ts, removed) => {
    const metadataByKey = get().registerMetadataByKey;
    const updates = new Map(get().registerValues);
    let changed = false;

    // Keys the server no longer has a value for: drop them so the UI shows "no data"
    // instead of a stale value.
    removed?.forEach((key) => {
      const reg = metadataByKey.get(key);
      if (reg && updates.delete(reg.id)) changed = true;
    });

    Object.entries(values).forEach(([key, dto]) => {
      const reg = metadataByKey.get(key);
      if (!reg) return;
      changed = true;

      const statusDecoded = dto.status_decoded;
      let displayValue: number | string | null;
      if (statusDecoded !== undefined && statusDecoded !== null) {
        displayValue = Array.isArray(statusDecoded)
          ? statusDecoded.join(', ')
          : (statusDecoded as SolisStatusDecoded).name ?? JSON.stringify(statusDecoded);
      } else {
        displayValue = dto.value;
      }

      const timestamp = dto.timestamp ?? ts;

      updates.set(reg.id, {
        key,
        id: reg.id,
        value: displayValue,
        timestamp,
        unit: dto.unit ?? reg.unit,
        statusDecoded,
      });
    });

    if (!changed) return;

    set({
      registerValues: updates,
      lastUpdated: ts ? new Date(ts) : new Date(),
    });
  },

  setConnected: (connected) => set({ isConnected: connected }),

  clearValues: () => {
    const { registerMetadata, lastUpdated, registerValues } = get();
    const empty = Array.from(registerValues.values()).every(v => v.value === null);
    if (empty && lastUpdated === null) return; // repeated reconnect failures: no re-render
    const values = new Map<string, RegisterValue>();
    registerMetadata.forEach(reg => {
      values.set(reg.id, { key: reg.key, id: reg.id, value: null, unit: reg.unit });
    });
    set({ registerValues: values, lastUpdated: null });
  },

  getRegisterById: (id) => get().registerMetadata.get(id),
  
  getResolvedRegisterById: (id) => {
    const metadata = get().registerMetadata.get(id);
    if (!metadata) return undefined;
    const value = get().registerValues.get(id);
    return resolveRegisterMetadata(metadata, value);
  },
}));
