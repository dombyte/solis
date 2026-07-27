import React from 'react';
import { formatValue } from '../../lib/utils/format';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { Badge } from '../ui/badge';
import { Skeleton } from '../ui/skeleton';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';
import { Info } from 'lucide-react';

interface ValueDisplayProps {
  dataId: string;
  showLabel?: boolean;
  showUnit?: boolean;
  className?: string;
  showStatusIndicator?: boolean;
  showTooltip?: boolean;
}

export function ValueDisplay({
  dataId,
  showLabel = true,
  showUnit = true,
  className = '',
  showStatusIndicator = false,
  showTooltip = true,
}: ValueDisplayProps): React.ReactElement {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const registerValues = useRegisterStore(state => state.registerValues);
  const getResolvedRegisterById = useRegisterStore(state => state.getResolvedRegisterById);
  const isLoading = useRegisterStore(state => state.isLoading);
  
  const resolvedRegister = getResolvedRegisterById(dataId);
  const register = resolvedRegister || registerMetadata.get(dataId);
  const value = registerValues.get(dataId);

  if (!register) {
    return <span className={className}>-</span>;
  }

  // Show loading skeleton
  if (isLoading && !value?.value) {
    return (
      <div className={`flex items-center gap-2 ${className}`}>
        {showLabel && (
          <Skeleton className="h-4 w-24" />
        )}
        <Skeleton className="h-6 w-16" />
      </div>
    );
  }

  let displayValue: string = '-';
  let displayUnit = '';
  const statusDecoded = value?.statusDecoded;
  const rawValue = value?.rawValue;
  
  // Use resolved display value if available from template resolution
  if (resolvedRegister?.displayValue !== undefined) {
    const resolvedValue = resolvedRegister.displayValue;
    const resolvedUnit = resolvedRegister.unit || '';
    const resolvedPrecision = resolvedRegister.precision;
    
    // For power values that should be displayed in kW
    if (resolvedUnit === 'W' && typeof resolvedValue === 'number') {
      const numValue = Number(resolvedValue);
      if (Math.abs(numValue) >= 1000) {
        displayValue = (numValue / 1000).toFixed(resolvedPrecision ?? 2);
        displayUnit = showUnit ? 'kW' : '';
      } else {
        displayValue = formatValue(resolvedValue, resolvedUnit, resolvedPrecision);
        displayUnit = showUnit ? resolvedUnit : '';
      }
    }
    // For power values already in kW
    else if (resolvedUnit === 'kW' && typeof resolvedValue === 'number') {
      displayValue = formatValue(resolvedValue, resolvedUnit, resolvedPrecision);
      displayUnit = showUnit ? resolvedUnit : '';
    }
    // For energy values that should be displayed in kWh
    else if (resolvedUnit === 'Wh' && typeof resolvedValue === 'number') {
      const numValue = Number(resolvedValue);
      if (Math.abs(numValue) >= 1000) {
        displayValue = (numValue / 1000).toFixed(resolvedPrecision ?? 2);
        displayUnit = showUnit ? 'kWh' : '';
      } else {
        displayValue = formatValue(resolvedValue, resolvedUnit, resolvedPrecision);
        displayUnit = showUnit ? resolvedUnit : '';
      }
    }
    // For energy values already in kWh
    else if (resolvedUnit === 'kWh' && typeof resolvedValue === 'number') {
      displayValue = formatValue(resolvedValue, resolvedUnit, resolvedPrecision);
      displayUnit = showUnit ? resolvedUnit : '';
    } else {
      displayValue = formatValue(resolvedValue, resolvedUnit, resolvedPrecision);
      displayUnit = showUnit ? resolvedUnit : '';
    }
  }
  else if (value?.value !== null && value?.value !== undefined) {
    // For status values, use the pre-decoded display value
    if (statusDecoded !== undefined && statusDecoded !== null) {
      displayValue = String(value.value);
      displayUnit = '';
    }
    // For power values that should be displayed in kW
    else if (register.unit === 'W' && typeof value.value === 'number') {
      const numValue = Number(value.value);
      if (Math.abs(numValue) >= 1000) {
        displayValue = (numValue / 1000).toFixed(register.precision ?? 2);
        displayUnit = showUnit ? 'kW' : '';
      } else {
        displayValue = formatValue(value.value, register.unit || '', register.precision);
        displayUnit = showUnit ? register.unit : '';
      }
    }
    // For power values already in kW
    else if (register.unit === 'kW' && typeof value.value === 'number') {
      displayValue = formatValue(value.value, register.unit || '', register.precision);
      displayUnit = showUnit ? register.unit : '';
    }
    // For energy values that should be displayed in kWh
    else if (register.unit === 'Wh' && typeof value.value === 'number') {
      const numValue = Number(value.value);
      if (Math.abs(numValue) >= 1000) {
        displayValue = (numValue / 1000).toFixed(register.precision ?? 2);
        displayUnit = showUnit ? 'kWh' : '';
      } else {
        displayValue = formatValue(value.value, register.unit || '', register.precision);
        displayUnit = showUnit ? register.unit : '';
      }
    }
    // For energy values already in kWh
    else if (register.unit === 'kWh' && typeof value.value === 'number') {
      displayValue = formatValue(value.value, register.unit || '', register.precision);
      displayUnit = showUnit ? register.unit : '';
    } else {
      displayValue = formatValue(value.value, register.unit || '', register.precision);
      displayUnit = showUnit ? (register.unit || '') : '';
    }
  }

  // Check if this is a status register with active faults/statuses
  const hasStatusIssues = (statusDecoded: unknown): boolean => {
    if (!statusDecoded) return false;
    if (Array.isArray(statusDecoded)) {
      return statusDecoded.length > 0;
    }
    if (typeof statusDecoded === 'object' && statusDecoded !== null) {
      const obj = statusDecoded as { name?: string };
      return obj.name !== 'Normal' && obj.name !== 'OK' && obj.name !== 'No fault';
    }
    return false;
  };

  const hasIssues = hasStatusIssues(statusDecoded);

  return (
    <div className={`flex items-center gap-2 ${className}`}>
      {showLabel && (
        <span className="text-sm font-medium">{register.name}:</span>
      )}
      <div className="flex items-center gap-2">
        <span className="text-lg font-semibold">{displayValue}{displayUnit && ' '}{displayUnit}</span>
        {showStatusIndicator && hasIssues && rawValue !== undefined && (
          <Badge variant="destructive" className="text-xs">
            {Array.isArray(statusDecoded) ? statusDecoded.length : '!'}
          </Badge>
        )}
        {showStatusIndicator && !hasIssues && rawValue !== undefined && statusDecoded !== undefined && (
          <Badge variant="outline" className="text-xs">
            OK
          </Badge>
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
    </div>
  );
}
