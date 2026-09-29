import type { RegisterMetadata, RegisterValue } from '../../../types';
import { isAlertStatus } from '../../../lib/utils/status';

// Flow register keys that are used for the power flow diagram
const FLOW_KEYS = [
  'pv_total_power',
  'battery_soc',
  'battery_power_signed',
  'household_load_power',
  'backup_load_power',
  'grid_power',
] as const;

// Status keys for the inverter status
const STATUS_KEYS = [
  'solis_status',
  'operating_status',
] as const;

// All keys needed for the power flow diagram
export const POWER_FLOW_KEYS = [...FLOW_KEYS, ...STATUS_KEYS] as const;

/**
 * Direction rules:
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
type FlowNode =
  | 'pv'
  | 'grid'
  | 'battery'
  | 'household'
  | 'backup'
  | 'inverter';

/**
 * Edge identifiers connecting nodes
 */
type FlowEdgeName =
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
  soc?: number;          // For battery only
  stale: boolean;        // This node has no data of its own (grayed independently of the rest)
}

/**
 * View model for a flow edge
 */
interface EdgeViewModel {
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
 * Format power value:
 * 2 decimals, W < 1000 ≤ kW; grid/battery show magnitude, sign conveyed by animation
 */
function formatPowerW(value: number | null): string {
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
      if (values.grid_power === null || values.grid_power === 0) return 'none';
      return values.grid_power > 0 ? 'out' : 'in';
    case 'battery':
      // battery_power_signed > 0 = charging (into battery = in), < 0 = discharging (out of battery)
      if (values.battery_power_signed === null || values.battery_power_signed === 0) return 'none';
      return values.battery_power_signed > 0 ? 'in' : 'out';
    case 'household':
      // Household always consumes (out from inverter)
      return values.household_load_power !== null && values.household_load_power > 0 ? 'out' : 'none';
    case 'backup':
      // Backup always consumes (out from inverter)
      return values.backup_load_power !== null && values.backup_load_power > 0 ? 'out' : 'none';
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
      // The path is drawn grid -> inverter.
      // grid_power > 0 = export: energy flows inverter -> grid, so the animation is reversed
      // grid_power < 0 = import: energy flows grid -> inverter, the path direction
      if (values.grid_power === null || values.grid_power === 0) {
        return { active: false, reverse: false };
      }
      return { 
        active: true,
        reverse: values.grid_power > 0 // export: inverter -> grid (against the drawn path)
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
      // The desktop path is drawn node -> inverter (same as pv/grid/battery), but household
      // only ever consumes (inverter -> household), so it always needs the reverse flag.
      return {
        active: values.household_load_power !== null && values.household_load_power !== 0,
        reverse: true
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
 * Build the complete flow view model from register values.
 *
 * `registerMetadataByKey` is the store's key-indexed metadata map (not the
 * id-indexed one), so each of the fixed FLOW_KEYS/STATUS_KEYS resolves in O(1).
 */
export function buildFlowViewModel(
  registerValues: Map<string, RegisterValue>,
  registerMetadataByKey: Map<string, RegisterMetadata>,
  stale: boolean = false
): FlowViewModel {
  const getRegisterAndValue = (key: string): { register: RegisterMetadata | undefined; value: RegisterValue | undefined } => {
    const register = registerMetadataByKey.get(key);
    const value = register ? registerValues.get(register.id) : undefined;
    return { register, value };
  };

  const getValue = (key: string): number | null => {
    const { value } = getRegisterAndValue(key);
    return value && typeof value.value === 'number' ? value.value : null;
  };

  const getStatusName = (value: RegisterValue | undefined): string | undefined => {
    if (!value) return undefined;
    if (value.statusDecoded && typeof value.statusDecoded === 'object' && 'name' in value.statusDecoded) {
      return (value.statusDecoded as { name: string }).name;
    }
    if (typeof value.value === 'string') {
      return value.value;
    }
    return undefined;
  };

  // Extract values from the store
  const values: Record<string, number | null> = {
    pv_total_power: getValue('pv_total_power'),
    battery_soc: getValue('battery_soc'),
    battery_power_signed: getValue('battery_power_signed'),
    household_load_power: getValue('household_load_power'),
    backup_load_power: getValue('backup_load_power'),
    grid_power: getValue('grid_power'),
  };

  // Status values + alert detection - reuses the same isAlertStatus logic as
  // the Status page (StatusDisplay.tsx) so the two surfaces never disagree.
  const solis = getRegisterAndValue('solis_status');
  const operating = getRegisterAndValue('operating_status');
  const solisStatus = getStatusName(solis.value);
  const operatingStatus = getStatusName(operating.value);
  const alert = isAlertStatus(solis.register, solis.value) || isAlertStatus(operating.register, operating.value);

  // Check if all nodes have data
  const allPresent = 
    values.pv_total_power !== null &&
    values.battery_soc !== null &&
    values.battery_power_signed !== null &&
    values.household_load_power !== null &&
    values.backup_load_power !== null &&
    values.grid_power !== null;

  // Per-node presence: a node grays out on its own when only its data is
  // missing, independent of the diagram-wide `stale` flag.
  const pvPresent = values.pv_total_power !== null;
  const gridPresent = values.grid_power !== null;
  const batteryPresent = values.battery_soc !== null && values.battery_power_signed !== null;
  const householdPresent = values.household_load_power !== null;
  const backupPresent = values.backup_load_power !== null;

  // Node view models
  const nodes: Record<FlowNode, NodeViewModel> = {
    pv: {
      id: 'pv',
      present: pvPresent,
      active: values.pv_total_power !== null && values.pv_total_power !== 0,
      value: values.pv_total_power,
      displayValue: formatPowerW(values.pv_total_power),
      direction: getNodeDirection('pv', values),
      color: '--flow-pv',
      label: 'PV',
      stale: !pvPresent,
    },
    grid: {
      id: 'grid',
      present: gridPresent,
      active: values.grid_power !== null && values.grid_power !== 0,
      value: values.grid_power,
      displayValue: formatPowerW(values.grid_power !== null ? Math.abs(values.grid_power) : null),
      direction: getNodeDirection('grid', values),
      color: '--flow-grid',
      label: 'Grid',
      stale: !gridPresent,
    },
    battery: {
      id: 'battery',
      present: batteryPresent,
      active: values.battery_power_signed !== null && values.battery_power_signed !== 0,
      value: values.battery_power_signed,
      displayValue: formatPowerW(values.battery_power_signed),
      direction: getNodeDirection('battery', values),
      color: '--flow-batt',
      label: 'Battery',
      soc: values.battery_soc ?? undefined, // missing SOC shows "–", not 0 % (FE-L4)
      stale: !batteryPresent,
    },
    household: {
      id: 'household',
      present: householdPresent,
      active: values.household_load_power !== null && values.household_load_power !== 0,
      value: values.household_load_power,
      displayValue: formatPowerW(values.household_load_power),
      direction: getNodeDirection('household', values),
      color: '--flow-hh',
      label: 'House',
      stale: !householdPresent,
    },
    backup: {
      id: 'backup',
      present: backupPresent,
      active: values.backup_load_power !== null && values.backup_load_power !== 0,
      value: values.backup_load_power,
      displayValue: formatPowerW(values.backup_load_power),
      direction: getNodeDirection('backup', values),
      color: '--flow-bk',
      label: 'Backup',
      stale: !backupPresent,
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
      stale: false,
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
      alert,
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
  return backupPower > 0 ? 'supplying loads' : 'standby · ready';
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

/**
 * Plain-text summary of the whole diagram for screen readers (read on demand, not a
 * live region: announcing every WebSocket tick would flood assistive tech). Only
 * status changes are live (StatusDisplay).
 */
export function buildFlowSummary(viewModel: FlowViewModel): string {
  const { nodes, inverter } = viewModel;

  const describe = (node: NodeViewModel, sub?: string): string =>
    node.stale ? `${node.label}: no data` : `${node.label} ${node.displayValue}${sub ? ` (${sub})` : ''}`;

  return [
    describe(nodes.pv),
    describe(nodes.grid, getGridSubStatus(nodes.grid.value)),
    nodes.battery.stale
      ? 'Battery: no data'
      : `Battery ${Math.round(nodes.battery.soc ?? 0)}% ${nodes.battery.displayValue} (${getBatterySubStatus(nodes.battery.value)})`,
    describe(nodes.household),
    describe(nodes.backup, getBackupSubStatus(nodes.backup.value)),
    `Inverter: ${inverter.status}, ${inverter.operatingStatus}`,
  ].join(', ');
}
