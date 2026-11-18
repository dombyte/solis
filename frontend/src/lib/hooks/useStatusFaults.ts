import { useMemo } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { dashboardGroups } from '../config/groups';
import { apiDataObjects } from '../config/data';
import { useRegisterStore } from '../stores/useRegisterStore';
import { statusLevel, statusText } from '../utils/status';
import { useSubscription } from './useSubscription';

/** The status registers (system_status group of data.ts), in display order. */
export const STATUS_IDS: string[] = dashboardGroups.find(g => g.id === 'system_status')?.dataIds ?? [];

const STATUS_KEYS = STATUS_IDS
  .map(id => apiDataObjects.find(o => o.id === id)?.key)
  .filter((key): key is string => key !== undefined);

/** One status register currently reporting a fault. */
export interface StatusFault {
  id: string;
  name: string;
  /** The active fault(s) as shown on the Status page. */
  text: string;
}

export interface StatusFaults {
  faults: StatusFault[];
  /** Ids of the status registers that currently have a value. */
  present: string[];
}

/**
 * The status registers that report a fault right now (statusLevel "fault": waiting or
 * standby at night is not one). Subscribes to every status key, so the nav badge, the
 * dashboard banner and notifications work on every page.
 */
export function useStatusFaults(): StatusFaults {
  useSubscription(STATUS_KEYS);
  const metadata = useRegisterStore(state => state.registerMetadata);
  const values = useRegisterStore(useShallow(state => STATUS_IDS.map(id => state.registerValues.get(id))));

  return useMemo(() => {
    const faults: StatusFault[] = [];
    const present: string[] = [];
    STATUS_IDS.forEach((id, i) => {
      const reg = metadata.get(id);
      const value = values[i];
      if (!reg || !value || value.value === null) return;
      present.push(id);
      if (statusLevel(reg, value) === 'fault') {
        faults.push({ id, name: reg.name, text: statusText(value.statusDecoded, value.value) });
      }
    });
    return { faults, present };
  }, [metadata, values]);
}
