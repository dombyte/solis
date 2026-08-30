/**
 * Format values based on unit and precision
 */
export function formatValue(
  value: number | string | boolean | null | undefined | Record<string, unknown>,
  unit: string = '',
  precision?: number
): string {
  if (value === null || value === undefined) return '-';
  if (typeof value === 'string') return value;
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  
  // Handle objects - try to extract DecodedValue, RawValue, or StringValue
  if (typeof value === 'object' && value !== null) {
    const obj = value as Record<string, unknown>;
    const decoded = obj.DecodedValue ?? obj.RawValue ?? obj.StringValue ?? obj.value ?? obj.key;
    if (typeof decoded === 'number' || typeof decoded === 'string') {
      return formatValue(decoded, unit, precision);
    }
    console.warn('formatValue received unprocessable object:', value);
    return '-';
  }
  
  const numValue = Number(value);
  if (isNaN(numValue)) {
    console.warn('formatValue received non-numeric value:', value);
    return '-';
  }
  const prec = precision ?? (unit === '%' ? 1 : unit === 'kWh' || unit === 'Wh' ? 2 : 2);

  // Handle percentage
  if (unit === '%') {
    return numValue.toFixed(prec);
  }

  // Handle power (W/kW)
  if (unit === 'W') {
    if (Math.abs(numValue) >= 1000) {
      return (numValue / 1000).toFixed(prec);
    }
    return numValue.toFixed(prec);
  }
  
  // Handle power already in kW
  if (unit === 'kW') {
    return numValue.toFixed(prec);
  }

  // Handle energy (Wh/kWh)
  if (unit === 'Wh') {
    if (Math.abs(numValue) >= 1000) {
      return (numValue / 1000).toFixed(prec);
    }
    return numValue.toFixed(prec);
  }
  
  // Handle energy already in kWh
  if (unit === 'kWh') {
    return numValue.toFixed(prec);
  }

  // Handle voltage
  if (unit === 'V') {
    return numValue.toFixed(prec);
  }

  // Handle current
  if (unit === 'A') {
    return numValue.toFixed(prec);
  }

  // Default: just format the number
  return numValue.toFixed(prec);
}

/**
 * Format a percentage value
 */
export function formatPercentage(value: number | null | undefined, precision: number = 1): string {
  if (value === null || value === undefined) return '-';
  return `${value.toFixed(precision)}%`;
}

/**
 * Format power value (auto-convert between W and kW)
 */
export function formatPower(value: number | null | undefined, precision: number = 2): string {
  if (value === null || value === undefined) return '-';
  if (Math.abs(value) >= 1000) {
    return `${(value / 1000).toFixed(precision)} kW`;
  }
  return `${value.toFixed(precision)} W`;
}

/**
 * Format energy value
 */
export function formatEnergy(value: number | null | undefined, precision: number = 2): string {
  if (value === null || value === undefined) return '-';
  return `${value.toFixed(precision)} kWh`;
}

/**
 * Format voltage value
 */
export function formatVoltage(value: number | null | undefined, precision: number = 1): string {
  if (value === null || value === undefined) return '-';
  return `${value.toFixed(precision)} V`;
}

/**
 * Format current value
 */
export function formatCurrent(value: number | null | undefined, precision: number = 2): string {
  if (value === null || value === undefined) return '-';
  return `${value.toFixed(precision)} A`;
}


