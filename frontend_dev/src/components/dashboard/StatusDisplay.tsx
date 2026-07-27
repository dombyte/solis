import React from 'react';
import { AlertTriangle, Info } from 'lucide-react';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { Badge } from '../ui/badge';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';

import { formatTemperature, formatVoltage, formatCurrent, formatPower, formatEnergy, formatPercentage, formatValue } from '../../lib/utils/format';
import type { SolisStatusDecoded, RegisterMetadata, RegisterValue } from '../../types';

interface StatusDisplayProps {
  dataId: string;
  showLabel?: boolean;
  className?: string;
  showTooltip?: boolean;
}

// Check if a status should show an alert (non-standard status)
function isAlertStatus(register: RegisterMetadata | undefined, value: RegisterValue | undefined): boolean {
  if (!register || !value) return false;

  // Numeric values in status groups (like temperature) - never alert
  if (value.rawValue !== undefined && register.format) {
    return false;
  }

  // If it has statusDecoded, check the specific status
  if (value.statusDecoded !== undefined && value.statusDecoded !== null) {
    if (Array.isArray(value.statusDecoded)) {
      // For arrays, check if this is a status register that should alert on non-empty
      if (register.id === 'operating_status') {
        // For operating status, check if any element is NOT "Normal operation"
        const hasOnlyNormal = value.statusDecoded.every((s: string) => s.toLowerCase().trim() === 'normal operation');
        return !hasOnlyNormal;
      }
      // For fault status registers, alert if array is not empty OR if it contains non-zero/fault items
      // Check if all items are "0" or "No faults" or "Normal operation" - if so, don't alert
      const hasRealFaults = value.statusDecoded.some((s: string) => {
        const item = String(s).toLowerCase().trim();
        return item !== '0' && item !== 'no faults' && item !== '' && item !== 'normal operation' && !item.includes('normal');
      });
      return hasRealFaults;
    } else if (typeof value.statusDecoded === 'object') {
      const statusObj = value.statusDecoded as SolisStatusDecoded;
      const statusName = (statusObj.name?.toLowerCase() || '').trim();
      
      // Solis Status - alert if NOT "Generating"
      if (register.id === 'solis_status') {
        return statusName !== 'generating';
      }
      
      // Operating Status - alert if NOT "Normal operation"
      if (register.id === 'operating_status') {
        return statusName !== 'normal operation';
      }
      
      // Grid Fault Status - alert if it contains fault/error
      if (register.id === 'grid_fault_status') {
        return statusName !== '' && statusName !== '0' && statusName !== 'no faults';
      }
      
      // Any other status with alert keywords
      const alertKeywords = ['fault', 'error', 'alarm', 'warning', 'fail', 'off', 'overvoltage', 'undervoltage'];
      return alertKeywords.some(kw => statusName.includes(kw));
    }
  }
  
  // For raw values without decoded status (like fault registers showing 0)
  if (value.rawValue !== undefined && !value.statusDecoded) {
    // If raw value is 0, it means no fault - don't alert
    if (value.rawValue === 0) {
      return false;
    }
    // If it's a fault register and raw value is non-zero, alert
    if (register.category === 'status' && value.rawValue !== 0) {
      return true;
    }
  }
  
  // Default: don't alert
  return false;
}

export function StatusDisplay({
  dataId,
  showLabel = true,
  className = '',
  showTooltip = true,
}: StatusDisplayProps): React.ReactElement {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const registerValues = useRegisterStore(state => state.registerValues);
  
  const register = registerMetadata.get(dataId);
  const value = registerValues.get(dataId);

  if (!register) {
    return <span className={className}>-</span>;
  }

  const statusDecoded = value?.statusDecoded;
  const rawValue = value?.rawValue;
  const displayValue = value?.value;

  if (!statusDecoded && rawValue === undefined && displayValue === null) {
    return <span className={className}>-</span>;
  }

  // Format the status display
  let statusText = '-';

  // Check if this should show an alert icon
  const shouldAlert = isAlertStatus(register, value);

  if (statusDecoded !== undefined && statusDecoded !== null) {
    if (Array.isArray(statusDecoded)) {
      if (statusDecoded.length > 0) {
        statusText = statusDecoded.join(', ');
      } else {
        statusText = 'No faults';
      }
    } else if (typeof statusDecoded === 'object') {
      const statusObj = statusDecoded as SolisStatusDecoded;
      statusText = statusObj.name || JSON.stringify(statusDecoded);
    } else {
      statusText = String(statusDecoded);
    }
  } else if (rawValue !== undefined) {
    // For numeric values in status groups (like temperature), use the formatter if available
    if (register.format) {
      // Apply scale factor if present
      const scale = register.scale ?? 1;
      const scaledValue = rawValue * scale;
      // Use the displayValue if it's a number, otherwise use scaled rawValue
      const numericValue = typeof displayValue === 'number' ? displayValue : scaledValue;
      const precision = register.precision ?? (register.unit === '%' || register.unit === '°C' ? 1 : 2);
      
      // Apply the appropriate formatter based on format type
      switch (register.format) {
        case 'temperature':
          statusText = formatTemperature(numericValue ?? null, precision);
          break;
        case 'percentage':
          statusText = formatPercentage(numericValue ?? null, precision);
          break;
        case 'power':
          statusText = formatPower(numericValue ?? null, precision);
          break;
        case 'energy':
          statusText = formatEnergy(numericValue ?? null, precision);
          break;
        case 'voltage':
          statusText = formatVoltage(numericValue ?? null, precision);
          break;
        case 'current':
          statusText = formatCurrent(numericValue ?? null, precision);
          break;
        default:
          statusText = formatValue(numericValue ?? null, register.unit || '', precision);
      }
    } else {
      // For fault registers without format, just show the raw value (don't show "Raw: 0")
      statusText = String(rawValue);
    }
  } else if (displayValue !== null && displayValue !== undefined) {
    statusText = String(displayValue);
  }

  return (
    <div className={`flex items-center gap-2 ${className}`}>
      {showLabel && (
        <span className="text-sm font-medium">{register.name}:</span>
      )}
      {shouldAlert ? (
        <Badge variant="destructive" className="text-xs px-2 py-0.5">
          {statusText}
        </Badge>
      ) : (
        <Badge variant="secondary" className="text-xs px-2 py-0.5">
          {statusText}
        </Badge>
      )}
      {shouldAlert && (
        <AlertTriangle className="h-4 w-4 text-destructive" />
      )}
      {showTooltip && register.description && (
        <Popover>
          <PopoverTrigger asChild>
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors rounded-sm p-1"
              aria-label={`Info about ${register.name}`}
            >
              <Info className="h-3.5 w-3.5" />
            </button>
          </PopoverTrigger>
          <PopoverContent className="w-72 max-w-[300px]" align="center" sideOffset={8}>
            <p className="text-sm text-popover-foreground whitespace-normal break-words">{register.description}</p>
          </PopoverContent>
        </Popover>
      )}
    </div>
  );
}
