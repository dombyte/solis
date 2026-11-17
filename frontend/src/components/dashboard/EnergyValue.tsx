import React from 'react';
import { formatMeasurement } from '../../lib/utils/format';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { Skeleton } from '../ui/skeleton';

interface EnergyValueProps {
  /** Internal register id, e.g. `pv_energy_today`. */
  id: string;
  /** Show the unit after the number (the table names it once in its header instead). */
  showUnit?: boolean;
}

/**
 * One energy figure from the live store, formatted like everywhere else ("4,695.70").
 * Missing data shows a dash, never 0.
 */
export function EnergyValue({ id, showUnit = false }: EnergyValueProps): React.ReactElement {
  // Select only this value: the cell re-renders only when its own value changes.
  const value = useRegisterStore(state => state.registerValues.get(id));
  const register = useRegisterStore(state => state.registerMetadata.get(id));
  const isLoading = useRegisterStore(state => state.isLoading);

  if (isLoading && value?.value === undefined) {
    return <Skeleton className="ml-auto h-4 w-14" />;
  }
  if (typeof value?.value !== 'number') {
    return <span className="text-muted-foreground">–</span>;
  }

  const m = formatMeasurement(value.value, register?.unit ?? 'kWh', register?.precision);
  return (
    <span className="whitespace-nowrap">
      {m.value}
      {showUnit && <span className="ml-1 text-xs font-normal text-muted-foreground">{m.unit}</span>}
    </span>
  );
}
