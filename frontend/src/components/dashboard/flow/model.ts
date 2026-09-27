import type { RegisterValue } from '../../../types';

// Flow register keys that are used for the power flow diagram
export const FLOW_KEYS = [
  'pv_total_power',
  'battery_soc',
  'battery_power_signed',
  'household_load_power',
  'backup_load_power',
  'grid_power',
] as const;

export type FlowKey = typeof FLOW_KEYS[number];

// Status keys for the inverter status
export const STATUS_KEYS = [
  'solis_status',
  'operating_status',
] as const;

export type StatusKey = typeof STATUS_KEYS[number];

// All keys needed for the power flow diagram
export const POWER_FLOW_KEYS = [...FLOW_KEYS, ...STATUS_KEYS] as const;

/**
 * Direction rules (from spec §14.2):
 * - PV → inverter (always positive from PV)
 * - inverter → household/backup (power flowing out)
 * - grid: export = out (positive), import = in (negative)
 * - battery: battery_power_signed > 0 = charging (into battery)
 * - Zero = shows 0, edge stops
 * - Missing data = node grayed out
 */

/**
 * Node identifiers in the flow diagram
 */
export type FlowNode = 
  | 'pv'
  | 'grid'
  | 'battery'
  | 'household'
  | 'backup'
  | 'inverter';

/**
 * Edge identifiers connecting nodes
 */
export type FlowEdgeName = 
  | 'pv_to_inverter'
  | 'grid_to_inverter'
  | 'battery_to_inverter'
  | 'inverter_to_household'
  | 'inverter_to_backup';

/**
 * View model for a single node in the flow diagram
 */
export interface NodeViewModel {
  id: FlowNode;
  present: boolean;       // Has data (not null/undefined)
  active: boolean;       // Has non-zero power value
  value: number | null;  // Current power value in W
  displayValue: string;  // Formatted display string
  direction: 'in' | 'out' | 'none';
  color: string;         // CSS token color
  label: string;
  icon: React.ReactNode; // Icon component (lucide-react)
  soc?: number;          // For battery only
  stale: boolean;        // Data is stale/unavailable
}

/**
 * View model for a flow edge
 */
export interface EdgeViewModel {
  id: FlowEdgeName;
  path: string;          // SVG path definition
  active: boolean;       // Edge is active (non-zero flow)
  reverse: boolean;      // Flow direction is reversed
  color: string;         // CSS token color
}

/**
 * Complete view model for the power flow diagram
 */
export interface FlowViewModel {
  nodes: Record<FlowNode, NodeViewModel>;
  edges: Record<FlowEdgeName, EdgeViewModel>;
  inverter: {
    status: string;
    operatingStatus: string;
    alert: boolean;
  };
  allPresent: boolean;    // All nodes have data
  stale: boolean;        // Any data is stale
}

/**
 * Format power value according to spec §14.2:
 * 2 decimals, W < 1000 ≤ kW; grid/battery show magnitude, sign conveyed by animation
 */
export function formatPowerW(value: number | null): string {
  if (value === null || value === 0) return '0.00 W';
  const abs = Math.abs(value);
  const formatted = abs >= 1000 
    ? (abs / 1000).toFixed(2) + ' kW'
    : abs.toFixed(2) + ' W';
  // Sign is conveyed by animation direction, not the text
  return formatted;
}

/**
 * Determine node direction based on flow rules
 * Returns 'in' (flow into inverter), 'out' (flow out of inverter), or 'none'
 */
function getNodeDirection(node: FlowNode, values: Record<string, number | null>): 'in' | 'out' | 'none' {
  switch (node) {
    case 'pv':
      // PV always flows TO inverter (out from PV perspective = into inverter)
      return values.pv_total_power !== null && values.pv_total_power > 0 ? 'out' : 'none';
    case 'grid':
      // Grid: positive = export (out from grid), negative = import (into grid)
      if (values.grid_power === null) return 'none';
      return values.grid_power > 0 ? 'out' : 'in';
    case 'battery':
      // battery_power_signed > 0 = charging (into battery = in), < 0 = discharging (out of battery)
      if (values.battery_power_signed === null) return 'none';
      return values.battery_power_signed > 0 ? 'in' : 'out';
    case 'household':
      // Household always consumes (out from inverter)
      return values.household_load_power !== null && values.household_load_power > 0 ? 'in' : 'none';
    case 'backup':
      // Backup always consumes (out from inverter)
      return values.backup_load_power !== null && values.backup_load_power > 0 ? 'in' : 'none';
    case 'inverter':
      // Inverter is the center - no direction
      return 'none';
    default:
      return 'none';
  }
}

/**
 * Determine edge activity and direction
 * Returns { active, reverse } where:
 * - active: whether the edge has non-zero flow
 * - reverse: whether the flow is in the reverse direction of the path
 */
function getEdgeState(edge: FlowEdgeName, values: Record<string, number | null>): { active: boolean; reverse: boolean } {
  switch (edge) {
    case 'pv_to_inverter':
      return { 
        active: values.pv_total_power !== null && values.pv_total_power !== 0,
        reverse: false // Always PV -> inverter
      };
    case 'grid_to_inverter':
      // grid_power > 0 = export (grid -> inverter is reverse of path direction)
      // grid_power < 0 = import (inverter -> grid, path direction)
      if (values.grid_power === null || values.grid_power === 0) {
        return { active: false, reverse: false };
      }
      return { 
        active: true,
        reverse: values.grid_power > 0 // export: grid -> inverter (reverse of drawn path)
      };
    case 'battery_to_inverter':
      // battery_power_signed > 0 = charging (battery <- inverter, so reverse)
      // battery_power_signed < 0 = discharging (battery -> inverter, path direction)
      if (values.battery_power_signed === null || values.battery_power_signed === 0) {
        return { active: false, reverse: false };
      }
      return {
        active: true,
        reverse: values.battery_power_signed > 0 // charging: inverter -> battery
      };
    case 'inverter_to_household':
      return {
        active: values.household_load_power !== null && values.household_load_power !== 0,
        reverse: false // Always inverter -> household
      };
    case 'inverter_to_backup':
      return {
        active: values.backup_load_power !== null && values.backup_load_power !== 0,
        reverse: false // Always inverter -> backup
      };
    default:
      return { active: false, reverse: false };
  }
}

/**
 * Check if a status indicates an alert condition
 */
export function isAlertStatus(status: string | undefined): boolean {
  if (!status) return false;
  const alertStatuses = [
    'Fault', 'Error', 'Warning', 'Alarm', 'Abnormal', 'Failure',
    'Overload', 'Overvoltage', 'Undervoltage', 'Overcurrent',
    'Island', 'Communication Error', 'Grid Fault',
  ];
  return alertStatuses.some(s => status.toLowerCase().includes(s.toLowerCase()));
}

/**
 * Build the complete flow view model from register values
 */
export function buildFlowViewModel(
  registerValues: Map<string, RegisterValue>,
  registerMetadata: Map<string, { key: string; id: string }>,
  stale: boolean = false
): FlowViewModel {
  // Extract values from the store
  const values: Record<string, number | null> = {
    pv_total_power: null,
    battery_soc: null,
    battery_power_signed: null,
    household_load_power: null,
    backup_load_power: null,
    grid_power: null,
  };

  // Get the metadata keys we need
  const getValue = (key: string): number | null => {
    // Try to find by key first
    for (const [id, meta] of registerMetadata) {
      if (meta.key === key) {
        const val = registerValues.get(id);
        if (val && typeof val.value === 'number') {
          return val.value;
        }
        break;
      }
    }
    return null;
  };

  values.pv_total_power = getValue('pv_total_power');
  values.battery_soc = getValue('battery_soc');
  values.battery_power_signed = getValue('battery_power_signed');
  values.household_load_power = getValue('household_load_power');
  values.backup_load_power = getValue('backup_load_power');
  values.grid_power = getValue('grid_power');

  // Get status values
  let solisStatus: string | undefined;
  let operatingStatus: string | undefined;

  for (const [id, meta] of registerMetadata) {
    if (meta.key === 'solis_status') {
      const val = registerValues.get(id);
      if (val && val.statusDecoded && typeof val.statusDecoded === 'object' && 'name' in val.statusDecoded) {
        solisStatus = (val.statusDecoded as { name: string }).name;
      } else if (val && typeof val.value === 'string') {
        solisStatus = val.value;
      }
    } else if (meta.key === 'operating_status') {
      const val = registerValues.get(id);
      if (val && val.statusDecoded && typeof val.statusDecoded === 'object' && 'name' in val.statusDecoded) {
        operatingStatus = (val.statusDecoded as { name: string }).name;
      } else if (val && typeof val.value === 'string') {
        operatingStatus = val.value;
      }
    }
  }

  // Check if all nodes have data
  const allPresent = 
    values.pv_total_power !== null &&
    values.battery_soc !== null &&
    values.battery_power_signed !== null &&
    values.household_load_power !== null &&
    values.backup_load_power !== null &&
    values.grid_power !== null;

  // Node view models
  const nodes: Record<FlowNode, NodeViewModel> = {
    pv: {
      id: 'pv',
      present: values.pv_total_power !== null,
      active: values.pv_total_power !== null && values.pv_total_power !== 0,
      value: values.pv_total_power,
      displayValue: formatPowerW(values.pv_total_power),
      direction: getNodeDirection('pv', values),
      color: '--flow-pv',
      label: 'PV',
      icon: null, // Will be set in component
      stale,
    },
    grid: {
      id: 'grid',
      present: values.grid_power !== null,
      active: values.grid_power !== null && values.grid_power !== 0,
      value: values.grid_power,
      displayValue: formatPowerW(values.grid_power !== null ? Math.abs(values.grid_power) : null),
      direction: getNodeDirection('grid', values),
      color: '--flow-grid',
      label: 'Grid',
      icon: null,
      stale,
    },
    battery: {
      id: 'battery',
      present: values.battery_soc !== null && values.battery_power_signed !== null,
      active: values.battery_power_signed !== null && values.battery_power_signed !== 0,
      value: values.battery_power_signed,
      displayValue: formatPowerW(values.battery_power_signed),
      direction: getNodeDirection('battery', values),
      color: '--flow-batt',
      label: 'Battery',
      icon: null,
      soc: values.battery_soc ?? 0,
      stale,
    },
    household: {
      id: 'household',
      present: values.household_load_power !== null,
      active: values.household_load_power !== null && values.household_load_power !== 0,
      value: values.household_load_power,
      displayValue: formatPowerW(values.household_load_power),
      direction: getNodeDirection('household', values),
      color: '--flow-hh',
      label: 'House',
      icon: null,
      stale,
    },
    backup: {
      id: 'backup',
      present: values.backup_load_power !== null,
      active: values.backup_load_power !== null && values.backup_load_power !== 0,
      value: values.backup_load_power,
      displayValue: formatPowerW(values.backup_load_power),
      direction: getNodeDirection('backup', values),
      color: '--flow-bk',
      label: 'Backup',
      icon: null,
      stale,
    },
    inverter: {
      id: 'inverter',
      present: true, // Always present
      active: true,
      value: null,
      displayValue: '',
      direction: 'none',
      color: '--flow-inv-bg',
      label: 'Inverter',
      icon: null,
      stale,
    },
  };

  // Edge view models
  const edges: Record<FlowEdgeName, EdgeViewModel> = {
    pv_to_inverter: {
      id: 'pv_to_inverter',
      path: '', // Will be set in component
      ...getEdgeState('pv_to_inverter', values),
      color: '--flow-pv',
    },
    grid_to_inverter: {
      id: 'grid_to_inverter',
      path: '',
      ...getEdgeState('grid_to_inverter', values),
      color: '--flow-grid',
    },
    battery_to_inverter: {
      id: 'battery_to_inverter',
      path: '',
      ...getEdgeState('battery_to_inverter', values),
      color: '--flow-batt',
    },
    inverter_to_household: {
      id: 'inverter_to_household',
      path: '',
      ...getEdgeState('inverter_to_household', values),
      color: '--flow-hh',
    },
    inverter_to_backup: {
      id: 'inverter_to_backup',
      path: '',
      ...getEdgeState('inverter_to_backup', values),
      color: '--flow-bk',
    },
  };

  return {
    nodes,
    edges,
    inverter: {
      status: solisStatus ?? 'Unknown',
      operatingStatus: operatingStatus ?? 'Unknown',
      alert: isAlertStatus(solisStatus) || isAlertStatus(operatingStatus),
    },
    allPresent,
    stale,
  };
}

/**
 * Get the sub-status text for backup
 */
export function getBackupSubStatus(backupPower: number | null): string {
  if (backupPower === null) return 'standby · ready';
  return backupPower > 0 ? 'supplying essential loads' : 'standby · ready';
}

/**
 * Get the sub-status text for grid
 */
export function getGridSubStatus(gridPower: number | null): string {
  if (gridPower === null) return '';
  if (gridPower > 0) return 'exporting';
  if (gridPower < 0) return 'importing';
  return 'idle';
}

/**
 * Get the sub-status text for battery
 */
export function getBatterySubStatus(batteryPower: number | null): string {
  if (batteryPower === null) return '';
  if (batteryPower > 0) return 'charging';
  if (batteryPower < 0) return 'discharging';
  return 'idle';
}
