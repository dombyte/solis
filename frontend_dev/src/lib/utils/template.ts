export interface TemplateContext {
  // From metadata
  key: string;
  id: string;
  name: string;
  description: string;
  unit: string;
  source: string;
  precision?: number;
  format?: string;
  category?: string;
  group?: string;
  
  // From API/WS data
  Key?: string;
  Name?: string;
  Description?: string;
  Unit?: string;
  RawValue?: number;
  DecodedValue?: number;
  StringValue?: string;
  Timestamp?: string;
  value?: unknown;           // Processed value from store
  rawValue?: number;     // From store
  timestamp?: string;    // From store
  statusDecoded?: unknown;   // From store
}

export function resolveTemplate(template: string, context: TemplateContext): string {
  return template.replace(/\{(\w+)\}/g, (match, placeholder) => {
    const value = context[placeholder as keyof TemplateContext];
    return value !== undefined ? String(value) : match;
  });
}

export function isTemplate(str: string): boolean {
  return /\{(\w+)\}/.test(str);
}


