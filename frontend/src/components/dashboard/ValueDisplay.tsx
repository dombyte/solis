import React from 'react';
import { formatValue } from '../../lib/utils/format';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { Skeleton } from '../ui/skeleton';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';
import { Info } from 'lucide-react';

interface ValueDisplayProps {
  dataId: string;
  showLabel?: boolean;
  showUnit?: boolean;
  className?: string;
  showTooltip?: boolean;
}

export function ValueDisplay({
  dataId,
  showLabel = true,
  showUnit = true,
  className = '',
  showTooltip = true,
}: ValueDisplayProps): React.ReactElement {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  // Select only this value: unchanged values keep their object across WS frames, so the
  // component re-renders only when its own value changes (review FE-M2).
  const value = useRegisterStore(state => state.registerValues.get(dataId));
  const getResolvedRegisterById = useRegisterStore(state => state.getResolvedRegisterById);
  const isLoading = useRegisterStore(state => state.isLoading);
  
  const resolvedRegister = getResolvedRegisterById(dataId);
  const register = resolvedRegister || registerMetadata.get(dataId);

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


  return (
    <div className={`flex flex-wrap items-center gap-1.5 sm:gap-2 ${className}`}>
      {showLabel && (
        <span className="text-xs sm:text-sm font-medium truncate min-w-0">{register.name}:</span>
      )}
      <div className="flex flex-wrap items-center gap-1 sm:gap-1.5">
        <span className="text-base sm:text-lg font-semibold break-all">{displayValue}{displayUnit && ' '}{displayUnit}</span>
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
    </div>
  );
}
