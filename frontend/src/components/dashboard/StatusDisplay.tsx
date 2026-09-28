import React from 'react';
import { AlertTriangle, Info } from 'lucide-react';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { Badge } from '../ui/badge';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';

import { formatVoltage, formatCurrent, formatPower, formatEnergy, formatPercentage, formatValue } from '../../lib/utils/format';
import { isAlertStatus } from '../../lib/utils/status';
import type { SolisStatusDecoded } from '../../types';

interface StatusDisplayProps {
  dataId: string;
  showLabel?: boolean;
  className?: string;
  showTooltip?: boolean;
}

export function StatusDisplay({
  dataId,
  showLabel = true,
  className = '',
  showTooltip = true,
}: StatusDisplayProps): React.ReactElement {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const value = useRegisterStore(state => state.registerValues.get(dataId));
  
  const register = registerMetadata.get(dataId);

  if (!register) {
    return <span className={className}>-</span>;
  }

  const statusDecoded = value?.statusDecoded;
  const displayValue = value?.value;
  const numericValue = typeof displayValue === 'number' ? displayValue : undefined;

  if (!statusDecoded && numericValue === undefined && (displayValue === null || displayValue === undefined)) {
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
  } else if (numericValue !== undefined) {
    // For numeric values in status groups, use the formatter if available. The backend
    // already sends a scaled, rounded value, so no client-side scale multiplication.
    if (register.format) {
      const precision = register.precision ?? (register.unit === '%' ? 1 : 2);

      // Apply the appropriate formatter based on format type
      switch (register.format) {
        case 'percentage':
          statusText = formatPercentage(numericValue, precision);
          break;
        case 'power':
          statusText = formatPower(numericValue, precision);
          break;
        case 'energy':
          statusText = formatEnergy(numericValue, precision);
          break;
        case 'voltage':
          statusText = formatVoltage(numericValue, precision);
          break;
        case 'current':
          statusText = formatCurrent(numericValue, precision);
          break;
        default:
          statusText = formatValue(numericValue, register.unit || '', precision);
      }
    } else {
      // For fault registers without format, just show the value (don't show "Raw: 0")
      statusText = String(numericValue);
    }
  } else if (displayValue !== null && displayValue !== undefined) {
    statusText = String(displayValue);
  }

  return (
    <div className={`flex flex-wrap items-center gap-1.5 sm:gap-2 ${className}`}>
      {showLabel && (
        <span className="text-xs sm:text-sm font-medium truncate min-w-0">{register.name}:</span>
      )}
      <div className="flex-shrink-0" aria-live="polite" aria-atomic="true">
        {shouldAlert ? (
          <Badge variant="destructive" className="text-xs px-1.5 py-0.5 truncate max-w-[100px] sm:max-w-[130px] md:max-w-[160px] lg:max-w-[180px] xl:max-w-[200px]">
            {statusText}
          </Badge>
        ) : (
          <Badge variant="secondary" className="text-xs px-1.5 py-0.5 truncate max-w-[100px] sm:max-w-[130px] md:max-w-[160px] lg:max-w-[180px] xl:max-w-[200px]">
            {statusText}
          </Badge>
        )}
      </div>
      {shouldAlert && (
        <AlertTriangle className="h-3.5 w-3.5 text-destructive flex-shrink-0" />
      )}
      {showTooltip && register.description && (
        <div className="flex-shrink-0">
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
        </div>
      )}
    </div>
  );
}
