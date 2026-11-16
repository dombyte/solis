import React from 'react';
import { formatMeasurement } from '../../lib/utils/format';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { Skeleton } from '../ui/skeleton';
import { InfoPopover } from '../ui/info-popover';

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

  // The resolved value (templates applied) wins over the raw store value.
  const raw = resolvedRegister?.displayValue !== undefined ? resolvedRegister.displayValue : value?.value;
  const registerUnit = register.unit || '';
  let displayValue = '-';
  let displayUnit = '';
  if (raw !== null && raw !== undefined) {
    if (value?.statusDecoded !== undefined && value?.statusDecoded !== null) {
      displayValue = String(raw); // a decoded status, already text
    } else if (typeof raw === 'number') {
      const m = formatMeasurement(raw, registerUnit, register.precision);
      displayValue = m.value;
      displayUnit = showUnit ? m.unit : '';
    } else {
      displayValue = raw;
      displayUnit = showUnit ? registerUnit : '';
    }
  }

  // Label and value share one line when the row is wide enough for the longest label,
  // "Energy Consumption", with a 5-digit kWh value: 16rem with the small phone text,
  // 18rem from `sm` up (text-sm/text-lg); below that they stack. The switch is a
  // container query on the row's own width, and all rows of a card are equally wide, so
  // a card never mixes one-line and two-line rows (a per-row flex-wrap did). A value
  // with a unit never breaks; a label that still does not fit is truncated.
  return (
    <div className={`@container ${className}`}>
      <div className="flex items-center gap-1.5 sm:gap-2 @max-[16rem]:flex-col @max-[16rem]:items-start @max-[16rem]:gap-0.5 sm:@max-[18rem]:flex-col sm:@max-[18rem]:items-start sm:@max-[18rem]:gap-0.5">
        {showLabel && (
          <span className="text-xs sm:text-sm font-medium text-muted-foreground truncate min-w-0 max-w-full">{register.name}:</span>
        )}
        {/* Only a number with a unit is kept whole; free text (a decoded status) may wrap. */}
        <div className={`flex items-center gap-1 sm:gap-1.5 ${displayUnit ? 'shrink-0' : 'min-w-0'}`}>
          <span className={`text-base sm:text-lg font-semibold ${displayUnit ? 'whitespace-nowrap' : 'break-words min-w-0'}`}>{displayValue}{displayUnit && ' '}{displayUnit}</span>
          {showTooltip && register.description && (
            <InfoPopover label={register.name}>
              <p>{register.description}</p>
            </InfoPopover>
          )}
        </div>
      </div>
    </div>
  );
}
